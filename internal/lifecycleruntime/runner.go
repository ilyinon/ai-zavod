package lifecycleruntime

import (
	"context"
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
	TerminalError  error
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
	checkpoint     func(lifecycler.RuntimeState) error
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

func (r Runner) WithCheckpoint(save func(lifecycler.RuntimeState) error) Runner {
	r.checkpoint = save
	return r
}

type StopError struct {
	Kind  string
	Cause string
}

func (e *StopError) Error() string {
	labels := map[string]string{"step_budget": "Исчерпан лимит выполнений шага", "repair_budget": "Исчерпан лимит доработок", "transition_budget": "Обнаружен цикл переходов: исчерпан общий лимит"}
	text := labels[e.Kind]
	if text == "" {
		text = "Lifecycle остановлен"
	}
	if e.Cause != "" {
		text += ". Причина: " + e.Cause
	}
	return text
}

func (r Runner) Run(ctx context.Context, state lifecycler.RuntimeState) (Result, error) {
	state = normalizeState(state)
	finish := func(status, reason string, err error) (Result, error) {
		if r.checkpoint != nil {
			if saveErr := r.checkpoint(cloneState(state)); saveErr != nil {
				return resultFromState(state, zw.StatusFailed, saveErr.Error()), saveErr
			}
		}
		return resultFromState(state, status, reason), err
	}
	maxTurns := r.maxTurns
	if maxTurns <= 0 {
		maxTurns = defaultMaxTurns(r.executor)
	}
	forceNextRun := map[string]bool{}
	for {
		if err := ctx.Err(); err != nil {
			return finish(zw.StatusFailed, err.Error(), err)
		}
		decision := r.executor.NextAction(state)
		// Completion consumes no execution budget, including at the exact limit.
		if decision.Action == lifecycler.ActionComplete {
			return finish(zw.StatusDone, decision.Reason, nil)
		}
		if decision.Action == lifecycler.ActionWaitHuman {
			return finish(zw.StatusWaitingUser, decision.Reason, nil)
		}
		if decision.Action == lifecycler.ActionBlocked {
			return finish(zw.StatusBlocked, firstNonEmpty(state.LastFailure, decision.Reason), nil)
		}
		if state.TransitionCount >= maxTurns {
			err := &StopError{Kind: "transition_budget", Cause: state.LastFailure}
			return finish(zw.StatusBlocked, err.Error(), err)
		}
		state.TransitionCount++
		failed := state.Results[decision.Step.StepKey].Status
		if decision.Action == lifecycler.ActionRetry || decision.Action == lifecycler.ActionJump && (failed == zw.StepStatusFailed || failed == "needs_work") {
			limit := r.executor.Definition().MaxRepairIterations
			if limit > 0 && state.RepairCount >= limit {
				err := &StopError{Kind: "repair_budget", Cause: state.LastFailure}
				return finish(zw.StatusBlocked, err.Error(), err)
			}
			state.RepairCount++
		}
		if r.checkpoint != nil {
			if err := r.checkpoint(cloneState(state)); err != nil {
				return finish(zw.StatusFailed, err.Error(), err)
			}
		}
		switch decision.Action {
		case lifecycler.ActionSkip:
			state.Results[decision.Step.StepKey] = lifecycler.StepResult{StepKey: decision.Step.StepKey, Status: zw.StepStatusSkipped, Error: decision.Reason}
			state.CurrentStepKey = decision.NextStepKey
		case lifecycler.ActionJump:
			mode := normalizeMode(decision.Step.Mode)
			if mode == lifecycler.ModeBranch || mode == lifecycler.ModeHumanGate {
				state.Results[decision.Step.StepKey] = lifecycler.StepResult{StepKey: decision.Step.StepKey, Status: zw.StepStatusDone}
			}
			target, exists := r.executor.Step(decision.NextStepKey)
			ordered := r.executor.Steps()
			from, to := -1, -1
			for index, step := range ordered {
				if step.StepKey == decision.Step.StepKey {
					from = index
				}
				if step.StepKey == decision.NextStepKey {
					to = index
				}
			}
			if exists && to >= 0 && to <= from {
				// Repair invalidates dependent outputs, but never their execution budgets.
				for _, affected := range ordered[to : from+1] {
					delete(state.Results, affected.StepKey)
					forceNextRun[affected.StepKey] = true
				}
			} else if exists && state.Results[target.StepKey].Status == zw.StepStatusFailed {
				delete(state.Results, decision.NextStepKey)
				forceNextRun[decision.NextStepKey] = true
			}
			state.CurrentStepKey = decision.NextStepKey
		case lifecycler.ActionRun, lifecycler.ActionRetry, lifecycler.ActionRunParallel:
			steps := []agentgroups.LifecycleStep{decision.Step}
			parallel := decision.Action == lifecycler.ActionRunParallel
			if parallel {
				steps = decision.Steps
			}
			for _, step := range steps {
				if err := ctx.Err(); err != nil {
					return finish(zw.StatusFailed, err.Error(), err)
				}
				force := forceNextRun[step.StepKey] || state.ExecutionCounts[step.StepKey] > 0
				delete(forceNextRun, step.StepKey)
				res := r.runStep(ctx, step, state, decision, decision.Action == lifecycler.ActionRetry, force || decision.Action == lifecycler.ActionRetry, parallel)
				state = applyStepResult(state, step.StepKey, res)
				if res.TerminalError != nil {
					return finish(zw.StatusFailed, res.Error, res.TerminalError)
				}
				if res.WaitHuman {
					return finish(zw.StatusWaitingUser, res.Error, nil)
				}
			}
			if parallel {
				state.CurrentStepKey = decision.Step.StepKey
			}
		default:
			err := fmt.Errorf("unknown lifecycle runtime action: %s", decision.Action)
			return finish(zw.StatusFailed, err.Error(), err)
		}
	}
}

func (r Runner) runStep(ctx context.Context, step agentgroups.LifecycleStep, state lifecycler.RuntimeState, decision lifecycler.RuntimeDecision, retry bool, force bool, parallel bool) StepResult {
	handler := r.handlerFor(step.Mode)
	if handler == nil {
		return StepResult{Status: zw.StepStatusFailed, Error: "handler not found for lifecycle mode: " + firstNonEmpty(step.Mode, lifecycler.ModeLLM)}
	}
	count := state.ExecutionCounts[step.StepKey]
	limit := 1
	if step.CanRetry {
		limit += step.MaxRetries
	}
	if count >= limit {
		err := &StopError{Kind: "step_budget", Cause: firstNonEmpty(state.LastFailure, step.Title)}
		return StepResult{Status: zw.StepStatusFailed, Error: err.Error(), TerminalError: err}
	}
	state.ExecutionCounts[step.StepKey] = count + 1
	state.InFlight[step.StepKey] = count + 1
	state.Attempts[step.StepKey] = count
	if r.checkpoint != nil {
		if err := r.checkpoint(cloneState(state)); err != nil {
			return StepResult{Status: zw.StepStatusFailed, Error: err.Error(), TerminalError: err}
		}
	}
	attempt := count
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
	delete(state.InFlight, stepKey)
	for key, value := range result.Variables {
		state.Variables[key] = value
	}
	state.Results[stepKey] = lifecycler.StepResult{
		Terminal: result.TerminalError != nil,
		StepKey:  stepKey,
		Status:   firstNonEmpty(result.Status, zw.StepStatusDone),
		Output:   result.Output,
		Error:    result.Error,
	}
	state.CurrentStepKey = stepKey
	if result.Status == zw.StepStatusFailed {
		state.LastFailure = result.Error
		state.LastFailureStep = stepKey
	} else if result.Status == zw.StepStatusDone && state.LastFailureStep == stepKey {
		state.LastFailure = ""
		state.LastFailureStep = ""
	}
	return state
}

func normalizeState(state lifecycler.RuntimeState) lifecycler.RuntimeState {
	if state.InFlight == nil {
		state.InFlight = map[string]int{}
	}
	if state.ExecutionCounts == nil {
		state.ExecutionCounts = map[string]int{}
		for key := range state.Results {
			state.ExecutionCounts[key] = state.Attempts[key] + 1
		}
	}
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
		InFlight:        cloneIntMap(state.InFlight),
		ExecutionCounts: cloneIntMap(state.ExecutionCounts), TransitionCount: state.TransitionCount, RepairCount: state.RepairCount, LastFailure: state.LastFailure, LastFailureStep: state.LastFailureStep,
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
