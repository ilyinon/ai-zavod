package lifecycle

import (
	"sort"
	"strings"

	"zavod_ai/internal/agentgroups"
)

type Executor struct {
	definition agentgroups.LifecycleDefinition
	steps      []agentgroups.LifecycleStep
	byKey      map[string]agentgroups.LifecycleStep
}

func NewExecutor(definition agentgroups.LifecycleDefinition, steps []agentgroups.LifecycleStep) Executor {
	ordered := append([]agentgroups.LifecycleStep(nil), steps...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].SortOrder == ordered[j].SortOrder {
			return ordered[i].StepKey < ordered[j].StepKey
		}
		return ordered[i].SortOrder < ordered[j].SortOrder
	})
	byKey := make(map[string]agentgroups.LifecycleStep, len(ordered))
	for _, step := range ordered {
		if key := strings.TrimSpace(step.StepKey); key != "" {
			byKey[key] = step
		}
	}
	return Executor{definition: definition, steps: ordered, byKey: byKey}
}

func (e Executor) Definition() agentgroups.LifecycleDefinition {
	return e.definition
}

func (e Executor) Steps() []agentgroups.LifecycleStep {
	return append([]agentgroups.LifecycleStep(nil), e.steps...)
}

func (e Executor) StepKeys() []string {
	keys := make([]string, 0, len(e.steps))
	for _, step := range e.steps {
		if key := strings.TrimSpace(step.StepKey); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func (e Executor) First() (agentgroups.LifecycleStep, bool) {
	if len(e.steps) == 0 {
		return agentgroups.LifecycleStep{}, false
	}
	return e.steps[0], true
}

func (e Executor) Step(stepKey string) (agentgroups.LifecycleStep, bool) {
	step, ok := e.byKey[strings.TrimSpace(stepKey)]
	return step, ok
}

func (e Executor) Next(currentStepKey string, success bool) (agentgroups.LifecycleStep, bool) {
	currentStepKey = strings.TrimSpace(currentStepKey)
	if currentStepKey == "" {
		return e.First()
	}
	current, ok := e.byKey[currentStepKey]
	if ok {
		target := current.OnFailureStepKey
		if success {
			target = current.OnSuccessStepKey
		}
		if next, found := e.Step(target); found {
			return next, true
		}
	}
	for index, step := range e.steps {
		if step.StepKey == currentStepKey && index+1 < len(e.steps) {
			return e.steps[index+1], true
		}
	}
	return agentgroups.LifecycleStep{}, false
}

func (e Executor) CanRetry(stepKey string, attempt int) bool {
	step, ok := e.Step(stepKey)
	if !ok || !step.CanRetry {
		return false
	}
	limit := step.MaxRetries
	if limit <= 0 {
		limit = e.definition.MaxRepairIterations
	}
	return attempt <= limit
}

func (e Executor) RetryLimit(stepKey string) int {
	step, ok := e.Step(stepKey)
	if ok && step.MaxRetries > 0 {
		return step.MaxRetries
	}
	if e.definition.MaxRepairIterations > 0 {
		return e.definition.MaxRepairIterations
	}
	return 0
}
