package lifecycleruntime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"zavod_ai/internal/agentgroups"
	lifecycler "zavod_ai/internal/lifecycle"
	zw "zavod_ai/internal/workflow"
)

func TestRunnerExecutesOrderedSteps(t *testing.T) {
	executor := lifecycler.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "first", Mode: lifecycler.ModeLLM, SortOrder: 1},
		{StepKey: "second", Mode: lifecycler.ModeLLM, SortOrder: 2},
	})
	var calls []string
	runner := NewRunner(executor, nil, func(ctx context.Context, step StepContext) StepResult {
		calls = append(calls, step.Step.StepKey)
		return StepResult{Output: step.Step.StepKey + " done"}
	})

	result, err := runner.Run(context.Background(), lifecycler.RuntimeState{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != zw.StatusDone {
		t.Fatalf("expected done, got %#v", result)
	}
	if !reflect.DeepEqual(calls, []string{"first", "second"}) {
		t.Fatalf("unexpected calls: %#v", calls)
	}
}

func TestRunnerRetriesThenReturns(t *testing.T) {
	cfg := mustRuntimeConfig(t, lifecycler.StepRuntimeConfig{ReturnToStepKey: "develop"})
	executor := lifecycler.NewExecutor(agentgroups.LifecycleDefinition{MaxRepairIterations: 1}, []agentgroups.LifecycleStep{
		{StepKey: "develop", Mode: lifecycler.ModeLLM, SortOrder: 1},
		{StepKey: "review", Mode: lifecycler.ModeReview, CanRetry: true, SortOrder: 2, OutputSchema: cfg},
		{StepKey: "final", Mode: lifecycler.ModeFinal, SortOrder: 3},
	})
	calls := map[string]int{}
	runner := NewRunner(executor, map[string]StepHandler{
		lifecycler.ModeReview: func(ctx context.Context, step StepContext) StepResult {
			calls[step.Step.StepKey]++
			return StepResult{Status: zw.StepStatusFailed, Error: "needs work"}
		},
	}, func(ctx context.Context, step StepContext) StepResult {
		calls[step.Step.StepKey]++
		return StepResult{Status: zw.StepStatusDone, Output: "ok"}
	}).WithMaxTurns(6)

	result, err := runner.Run(context.Background(), lifecycler.RuntimeState{})
	if err == nil {
		t.Fatal("expected max turn error because review keeps returning to develop")
	}
	if result.Status != zw.StatusBlocked {
		t.Fatalf("expected blocked by max turns, got %#v", result)
	}
	if calls["review"] < 2 || calls["develop"] < 2 {
		t.Fatalf("expected retry/return loop, got calls %#v", calls)
	}
}

func TestRunnerStopsOnHumanGateAndResumes(t *testing.T) {
	cfg := mustRuntimeConfig(t, lifecycler.StepRuntimeConfig{
		HumanGate: lifecycler.HumanGateConfig{
			Reason:         "scope required",
			RequiredInputs: []string{"target", "authorization"},
		},
	})
	executor := lifecycler.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "scope", Mode: lifecycler.ModeHumanGate, SortOrder: 1, OutputSchema: cfg},
		{StepKey: "solve", Mode: lifecycler.ModeLLM, SortOrder: 2},
	})
	runner := NewRunner(executor, nil, func(ctx context.Context, step StepContext) StepResult {
		return StepResult{Output: "ran " + step.Step.StepKey}
	})

	result, err := runner.Run(context.Background(), lifecycler.RuntimeState{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != zw.StatusWaitingUser || result.Reason != "scope required" {
		t.Fatalf("expected waiting user, got %#v", result)
	}

	result, err = runner.Run(context.Background(), lifecycler.RuntimeState{
		CurrentStepKey: "scope",
		HumanGates:     map[string]bool{"scope": true},
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if result.Status != zw.StatusDone || result.Results["solve"].Status != zw.StepStatusDone {
		t.Fatalf("expected resumed solve, got %#v", result)
	}
}

func TestRunnerRunsParallelTargetsSequentiallyWithJoinSemantics(t *testing.T) {
	cfg := mustRuntimeConfig(t, lifecycler.StepRuntimeConfig{
		ParallelSteps: []string{"lint", "tests"},
		ParallelWait:  "all",
		JoinStepKey:   "review",
	})
	executor := lifecycler.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "fanout", Mode: lifecycler.ModeParallel, SortOrder: 1, OutputSchema: cfg},
		{StepKey: "lint", Mode: lifecycler.ModeChecks, SortOrder: 2},
		{StepKey: "tests", Mode: lifecycler.ModeChecks, SortOrder: 3},
		{StepKey: "review", Mode: lifecycler.ModeReview, SortOrder: 4},
	})
	var parallelFlags []bool
	runner := NewRunner(executor, nil, func(ctx context.Context, step StepContext) StepResult {
		parallelFlags = append(parallelFlags, step.Parallel)
		return StepResult{Output: step.Step.StepKey}
	})

	result, err := runner.Run(context.Background(), lifecycler.RuntimeState{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != zw.StatusDone {
		t.Fatalf("expected done, got %#v", result)
	}
	if result.Results["lint"].Status != zw.StepStatusDone || result.Results["tests"].Status != zw.StepStatusDone || result.Results["review"].Status != zw.StepStatusDone {
		t.Fatalf("expected parallel targets and review to run, got %#v", result.Results)
	}
	if len(parallelFlags) < 3 || !parallelFlags[0] || !parallelFlags[1] || parallelFlags[2] {
		t.Fatalf("unexpected parallel flags: %#v", parallelFlags)
	}
}

func TestRunnerFailsWhenHandlerMissing(t *testing.T) {
	executor := lifecycler.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "tool", Mode: lifecycler.ModeTool, Required: true},
	})
	runner := NewRunner(executor, nil, nil)

	result, err := runner.Run(context.Background(), lifecycler.RuntimeState{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Status != zw.StatusBlocked {
		t.Fatalf("expected blocked result, got %#v", result)
	}
	if result.Results["tool"].Status != zw.StepStatusFailed {
		t.Fatalf("expected failed tool result, got %#v", result)
	}
}

func TestRunnerPropagatesContextCancel(t *testing.T) {
	executor := lifecycler.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "first", Mode: lifecycler.ModeLLM},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := NewRunner(executor, nil, func(ctx context.Context, step StepContext) StepResult {
		return StepResult{}
	})

	result, err := runner.Run(ctx, lifecycler.RuntimeState{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if result.Status != zw.StatusFailed {
		t.Fatalf("expected failed result, got %#v", result)
	}
}

func mustRuntimeConfig(t *testing.T, cfg lifecycler.StepRuntimeConfig) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
