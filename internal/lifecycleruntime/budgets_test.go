package lifecycleruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/lifecycle"
	zw "zavod_ai/internal/workflow"
)

func TestSelfReturnCannotResetExecutionBudget(t *testing.T) {
	for _, retries := range []int{0, 1, 2} {
		for _, enabled := range []bool{false, true} {
			executor := lifecycle.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{{StepKey: "a", CanRetry: enabled, MaxRetries: retries, OnFailureStepKey: "a", Required: true}})
			calls := 0
			var snapshot lifecycle.RuntimeState
			runner := NewRunner(executor, nil, func(context.Context, StepContext) StepResult {
				calls++
				return StepResult{Status: zw.StepStatusFailed, Error: "compiler error"}
			}).WithCheckpoint(func(s lifecycle.RuntimeState) error { snapshot = s; return nil })
			_, err := runner.Run(context.Background(), lifecycle.RuntimeState{})
			limit := 1
			if enabled {
				limit += retries
			}
			var stop *StopError
			if calls != limit || !errors.As(err, &stop) || stop.Kind != "step_budget" {
				t.Fatalf("retries=%d enabled=%v calls=%d err=%v", retries, enabled, calls, err)
			}
			raw, _ := json.Marshal(snapshot)
			json.Unmarshal(raw, &snapshot)
			runner.Run(context.Background(), snapshot)
			if calls != limit {
				t.Fatal("restore renewed execution budget")
			}
		}
	}
}

func TestSuccessAtExactBudgetClearsFailure(t *testing.T) {
	executor := lifecycle.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{{StepKey: "a", CanRetry: true, MaxRetries: 2, Required: true}})
	calls := 0
	var state lifecycle.RuntimeState
	runner := NewRunner(executor, nil, func(context.Context, StepContext) StepResult {
		calls++
		if calls < 3 {
			return StepResult{Status: zw.StepStatusFailed, Error: "bad code"}
		}
		return StepResult{Status: zw.StepStatusDone}
	}).WithMaxTurns(3).WithCheckpoint(func(s lifecycle.RuntimeState) error { state = s; return nil })
	result, err := runner.Run(context.Background(), lifecycle.RuntimeState{})
	if err != nil || result.Status != zw.StatusDone || state.LastFailure != "" || state.ExecutionCounts["a"] != 3 {
		t.Fatalf("last allowed success failed: %+v %v", result, err)
	}
}

func TestTransitionLoopAndCheckpoint(t *testing.T) {
	executor := lifecycle.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{{StepKey: "a", Mode: lifecycle.ModeBranch, OutputSchema: `{"branches":[{"default":true,"nextStepKey":"b"}]}`, SortOrder: 1}, {StepKey: "b", Mode: lifecycle.ModeBranch, OutputSchema: `{"branches":[{"default":true,"nextStepKey":"a"}]}`, SortOrder: 2}})
	var saved lifecycle.RuntimeState
	runner := NewRunner(executor, nil, func(context.Context, StepContext) StepResult { t.Fatal("branch executed model"); return StepResult{} }).WithMaxTurns(4).WithCheckpoint(func(s lifecycle.RuntimeState) error { saved = s; return nil })
	_, err := runner.Run(context.Background(), lifecycle.RuntimeState{})
	var stop *StopError
	if !errors.As(err, &stop) || stop.Kind != "transition_budget" || saved.TransitionCount != 4 {
		t.Fatalf("unbounded transitions: %+v %v", saved, err)
	}
	_, err = runner.Run(context.Background(), saved)
	if !errors.As(err, &stop) || saved.TransitionCount != 4 {
		t.Fatalf("restore reset transitions: %v", err)
	}
}

func TestCheckpointFailureDoesNotExecuteHandler(t *testing.T) {
	sentinel := errors.New("disk full")
	executor := lifecycle.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{{StepKey: "a"}})
	runner := NewRunner(executor, nil, func(context.Context, StepContext) StepResult {
		t.Fatal("executed without durable budget")
		return StepResult{}
	}).WithCheckpoint(func(lifecycle.RuntimeState) error { return sentinel })
	_, err := runner.Run(context.Background(), lifecycle.RuntimeState{})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

func TestForwardTransitionPreservesCompletedOutputsWithEqualOrder(t *testing.T) {
	executor := lifecycle.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{{StepKey: "first"}, {StepKey: "second"}})
	runner := NewRunner(executor, nil, func(_ context.Context, step StepContext) StepResult { return StepResult{Output: step.Step.StepKey} })
	result, err := runner.Run(context.Background(), lifecycle.RuntimeState{})
	if err != nil || len(result.Results) != 2 {
		t.Fatalf("forward transition invalidated completed output: %+v %v", result, err)
	}
}
