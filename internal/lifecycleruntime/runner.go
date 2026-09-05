package lifecycleruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"zavod_ai/internal/agentgroups"
	lifecycler "zavod_ai/internal/lifecycle"
	zw "zavod_ai/internal/workflow"
)

type StepHandler func(context.Context, StepContext) StepResult

type StepContext struct {
	Step        agentgroups.LifecycleStep
	Attempt     int
	Retry       bool
	Force       bool
	Parallel    bool
	State       lifecycler.RuntimeState
	Decision    lifecycler.RuntimeDecision
	Variables   map[string]string
	LastOutputs map[string]string
}

type StepResult struct {
	Status         string
	Output         string
	Error          string
	Variables      map[string]string
	WaitHuman      bool
	RequiredInputs []string
}

type Result struct {
	Status         string
	Reason         string
	CurrentStepKey string
	Results        map[string]lifecycler.StepResult
	Attempts       map[string]int
	Variables      map[string]string
}

type Runner struct {
	executor       lifecycler.Executor
	handlers       map[string]StepHandler
	defaultHandler StepHandler
	maxTurns       int
}

func NewRunner(executor lifecycler.Executor, handlers map[string]StepHandler, defaultHandler StepHandler) Runner {
	copied := map[string]StepHandler{}
	for mode, handler := range handlers {
		mode = normalizeMode(mode)
		if mode == "" || handler == nil {
			continue
		}
		copied[mode] = handler
	}
	return Runner{
		executor:       executor,
		handlers:       copied,
		defaultHandler: defaultHandler,
	}
}

func (r Runner) WithMaxTurns(maxTurns int) Runner {
	r.maxTurns = maxTurns
	return r
}

func (r Runner) Run(ctx context.Context, state lifecycler.RuntimeState) (Result, error) {
	state = normalizeState(state)
	maxTurns := r.maxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns(r.executor)
	}
	forceNextRun := map[string]bool{}
	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return resultFromState(state, zw.StatusFailed, err.Error()), err
		}
		decision := r.executor.NextAction(state)
		switch decision.Action {
		case lifecycler.ActionComplete:
			return resultFromState(state, zw.StatusDone, decision.Reason), nil
		case lifecycler.ActionBlocked:
			return resultFromState(state, zw.StatusBlocked, firstNonEmpty(decision.Reason, "lifecycle runtime blocked")), nil
		case lifecycler.ActionWaitHuman:
			return resultFromState(state, zw.StatusWaitingUser, firstNonEmpty(decision.Reason, "workflow waits for user input")), nil
		case lifecycler.ActionSkip:
			state.Results[decision.Step.StepKey] = lifecycler.StepResult{
				StepKey: decision.Step.StepKey,
				Status:  zw.StepStatusSkipped,
				Error:   decision.Reason,
			}
			state.CurrentStepKey = decision.NextStepKey
		case lifecycler.ActionJump:
			if strings.TrimSpace(decision.NextStepKey) != "" {
				delete(state.Results, decision.NextStepKey)
				forceNextRun[decision.NextStepKey] = true
			}
			state.CurrentStepKey = decision.NextStepKey
		case lifecycler.ActionRetry:
			stepResult := r.runStep(ctx, decision.Step, state, decision, true, true, false)
			state = applyStepResult(state, decision.Step.StepKey, stepResult)
			if stepResult.WaitHuman {
				return resultFromState(state, zw.StatusWaitingUser, firstNonEmpty(stepResult.Error, decision.Reason)), nil
			}
		case lifecycler.ActionRun:
			force := forceNextRun[decision.Step.StepKey]
			delete(forceNextRun, decision.Step.StepKey)
			stepResult := r.runStep(ctx, decision.Step, state, decision, false, force, false)
			state = applyStepResult(state, decision.Step.StepKey, stepResult)
			if stepResult.WaitHuman {
				return resultFromState(state, zw.StatusWaitingUser, firstNonEmpty(stepResult.Error, decision.Reason)), nil
			}
		case lifecycler.ActionRunParallel:
			for _, step := range decision.Steps {
				stepResult := r.runStep(ctx, step, state, decision, false, false, true)
				state = applyStepResult(state, step.StepKey, stepResult)
				if stepResult.WaitHuman {
					return resultFromState(state, zw.StatusWaitingUser, firstNonEmpty(stepResult.Error, decision.Reason)), nil
				}
			}
			state.CurrentStepKey = decision.Step.StepKey
		default:
			err := fmt.Errorf("unknown lifecycle runtime action: %s", decision.Action)
			return resultFromState(state, zw.StatusFailed, err.Error()), err
		}
	}
	err := errors.New("lifecycle runtime reached max turns")
	return resultFromState(state, zw.StatusBlocked, err.Error()), err
}

func (r Runner) runStep(ctx context.Context, step agentgroups.LifecycleStep, state lifecycler.RuntimeState, decision lifecycler.RuntimeDecision, retry bool, force bool, parallel bool) StepResult {
	handler := r.handlerFor(step.Mode)
	if handler == nil {
		return StepResult{Status: zw.StepStatusFailed, Error: "handler not found for lifecycle mode: " + firstNonEmpty(step.Mode, lifecycler.ModeLLM)}
	}
	attempt := state.Attempts[step.StepKey]
	if retry {
		attempt++
		state.Attempts[step.StepKey] = attempt
	}
	result := handler(ctx, StepContext{
		Step:        step,
		Attempt:     attempt,
		Retry:       retry,
		Force:       force,
		Parallel:    parallel,
		State:       cloneState(state),
		Decision:    decision,
		Variables:   cloneStringMap(state.Variables),
		LastOutputs: outputsFromState(state),
	})
	result.Status = firstNonEmpty(result.Status, zw.StepStatusDone)
	return result
}

func (r Runner) handlerFor(mode string) StepHandler {
	mode = normalizeMode(mode)
	if handler := r.handlers[mode]; handler != nil {
		return handler
	}
	return r.defaultHandler
}

func applyStepResult(state lifecycler.RuntimeState, stepKey string, result StepResult) lifecycler.RuntimeState {
	state = normalizeState(state)
	for key, value := range result.Variables {
		state.Variables[key] = value
	}
	state.Results[stepKey] = lifecycler.StepResult{
		StepKey: stepKey,
		Status:  firstNonEmpty(result.Status, zw.StepStatusDone),
		Output:  result.Output,
		Error:   result.Error,
	}
	state.CurrentStepKey = stepKey
	return state
}

func normalizeState(state lifecycler.RuntimeState) lifecycler.RuntimeState {
	if state.Results == nil {
		state.Results = map[string]lifecycler.StepResult{}
	}
	if state.Attempts == nil {
		state.Attempts = map[string]int{}
	}
	if state.Variables == nil {
		state.Variables = map[string]string{}
	}
	if state.HumanGates == nil {
		state.HumanGates = map[string]bool{}
	}
	return state
}

func resultFromState(state lifecycler.RuntimeState, status string, reason string) Result {
	state = normalizeState(state)
	return Result{
		Status:         status,
		Reason:         reason,
		CurrentStepKey: state.CurrentStepKey,
		Results:        cloneResults(state.Results),
		Attempts:       cloneIntMap(state.Attempts),
		Variables:      cloneStringMap(state.Variables),
	}
}

func defaultMaxTurns(executor lifecycler.Executor) int {
	definition := executor.Definition()
	if definition.MaxTotalIterations > 0 {
		return definition.MaxTotalIterations
	}
	stepCount := len(executor.StepKeys())
	if stepCount == 0 {
		return 1
	}
	retries := definition.MaxRepairIterations
	if retries <= 0 {
		retries = 1
	}
	maxTurns := stepCount * (retries + 3)
	if maxTurns < 20 {
		return 20
	}
	return maxTurns
}

func normalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return lifecycler.ModeLLM
	}
	return mode
}

func outputsFromState(state lifecycler.RuntimeState) map[string]string {
	out := map[string]string{}
	for key, result := range state.Results {
		if strings.TrimSpace(result.Output) != "" {
			out[key] = result.Output
		}
	}
	return out
}

func cloneState(state lifecycler.RuntimeState) lifecycler.RuntimeState {
	return lifecycler.RuntimeState{
		CurrentStepKey: state.CurrentStepKey,
		Results:        cloneResults(state.Results),
		Attempts:       cloneIntMap(state.Attempts),
		Variables:      cloneStringMap(state.Variables),
		HumanGates:     cloneBoolMap(state.HumanGates),
	}
}

func cloneResults(values map[string]lifecycler.StepResult) map[string]lifecycler.StepResult {
	out := map[string]lifecycler.StepResult{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneIntMap(values map[string]int) map[string]int {
	out := map[string]int{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	out := map[string]bool{}
	for key, value := range values {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
