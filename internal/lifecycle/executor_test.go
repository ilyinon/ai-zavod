package lifecycle

import (
	"encoding/json"
	"testing"

	"zavod_ai/internal/agentgroups"
	zw "zavod_ai/internal/workflow"
)

func TestExecutorOrdersStepsAndFollowsTransitions(t *testing.T) {
	executor := NewExecutor(agentgroups.LifecycleDefinition{
		MaxRepairIterations: 2,
	}, []agentgroups.LifecycleStep{
		{StepKey: "review", Title: "Review", SortOrder: 3, OnFailureStepKey: "developer_plan"},
		{StepKey: "developer_plan", Title: "Code", SortOrder: 2, OnSuccessStepKey: "review"},
		{StepKey: "manager_intake", Title: "Intake", SortOrder: 1},
	})

	keys := executor.StepKeys()
	want := []string{"manager_intake", "developer_plan", "review"}
	for index, key := range want {
		if keys[index] != key {
			t.Fatalf("keys[%d] = %q, want %q", index, keys[index], key)
		}
	}

	next, ok := executor.Next("developer_plan", true)
	if !ok || next.StepKey != "review" {
		t.Fatalf("success transition = %q, %v; want review, true", next.StepKey, ok)
	}

	next, ok = executor.Next("review", false)
	if !ok || next.StepKey != "developer_plan" {
		t.Fatalf("failure transition = %q, %v; want developer_plan, true", next.StepKey, ok)
	}
}

func TestExecutorRetryLimit(t *testing.T) {
	executor := NewExecutor(agentgroups.LifecycleDefinition{
		MaxRepairIterations: 2,
	}, []agentgroups.LifecycleStep{
		{StepKey: "review", CanRetry: true, MaxRetries: 3},
		{StepKey: "checks", CanRetry: true},
		{StepKey: "final", CanRetry: false},
	})

	if !executor.CanRetry("review", 3) {
		t.Fatal("review should allow its explicit third retry")
	}
	if executor.CanRetry("review", 4) {
		t.Fatal("review should stop after explicit retry limit")
	}
	if !executor.CanRetry("checks", 2) {
		t.Fatal("checks should use lifecycle fallback retry limit")
	}
	if executor.CanRetry("final", 1) {
		t.Fatal("final should not retry")
	}
}

func TestRuntimeBranchesByCondition(t *testing.T) {
	cfg := mustRuntimeConfig(t, StepRuntimeConfig{
		Branches: []BranchRule{
			{When: Condition{Field: "var:intent", Operator: "equals", Value: "ctf"}, NextStepKey: "ctf"},
			{Default: true, NextStepKey: "dev"},
		},
	})
	executor := NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "route", Mode: ModeBranch, OutputSchema: cfg},
		{StepKey: "dev", Mode: ModeLLM},
		{StepKey: "ctf", Mode: ModeLLM},
	})

	decision := executor.NextAction(RuntimeState{
		CurrentStepKey: "route",
		Variables:      map[string]string{"intent": "ctf"},
	})
	if decision.Action != ActionJump || decision.NextStepKey != "ctf" {
		t.Fatalf("expected branch jump to ctf, got %#v", decision)
	}
}

func TestRuntimeHumanGateWaitsUntilApproved(t *testing.T) {
	cfg := mustRuntimeConfig(t, StepRuntimeConfig{
		HumanGate: HumanGateConfig{
			Reason:         "нужно подтвердить scope",
			RequiredInputs: []string{"target", "authorization"},
		},
	})
	executor := NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "scope", Mode: ModeHumanGate, OutputSchema: cfg},
		{StepKey: "solve", Mode: ModeLLM},
	})

	decision := executor.NextAction(RuntimeState{CurrentStepKey: "scope"})
	if decision.Action != ActionWaitHuman || len(decision.RequiredInputs) != 2 {
		t.Fatalf("expected human gate wait, got %#v", decision)
	}

	decision = executor.NextAction(RuntimeState{
		CurrentStepKey: "scope",
		HumanGates:     map[string]bool{"scope": true},
	})
	if decision.Action != ActionJump || decision.NextStepKey != "solve" {
		t.Fatalf("expected approved human gate to continue, got %#v", decision)
	}
}

func TestRuntimeParallelRunsTargetsAndJoins(t *testing.T) {
	cfg := mustRuntimeConfig(t, StepRuntimeConfig{
		ParallelSteps: []string{"lint", "tests"},
		ParallelWait:  "all",
		JoinStepKey:   "review",
	})
	executor := NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "fanout", Mode: ModeParallel, OutputSchema: cfg},
		{StepKey: "lint", Mode: ModeChecks},
		{StepKey: "tests", Mode: ModeChecks},
		{StepKey: "review", Mode: ModeReview},
	})

	decision := executor.NextAction(RuntimeState{CurrentStepKey: "fanout"})
	if decision.Action != ActionRunParallel || len(decision.Steps) != 2 {
		t.Fatalf("expected parallel run, got %#v", decision)
	}

	decision = executor.NextAction(RuntimeState{
		CurrentStepKey: "fanout",
		Results: map[string]StepResult{
			"lint":  {StepKey: "lint", Status: zw.StepStatusDone},
			"tests": {StepKey: "tests", Status: zw.StepStatusDone},
		},
	})
	if decision.Action != ActionJump || decision.NextStepKey != "review" {
		t.Fatalf("expected parallel join to review, got %#v", decision)
	}
}

func TestRuntimeRetriesThenReturnsToConfiguredStep(t *testing.T) {
	cfg := mustRuntimeConfig(t, StepRuntimeConfig{ReturnToStepKey: "developer"})
	executor := NewExecutor(agentgroups.LifecycleDefinition{MaxRepairIterations: 1}, []agentgroups.LifecycleStep{
		{StepKey: "developer", Mode: ModeLLM},
		{StepKey: "review", Mode: ModeReview, CanRetry: true, OutputSchema: cfg},
	})

	decision := executor.NextAction(RuntimeState{
		CurrentStepKey: "review",
		Results:        map[string]StepResult{"review": {StepKey: "review", Status: zw.StepStatusFailed}},
		Attempts:       map[string]int{"review": 0},
	})
	if decision.Action != ActionRetry {
		t.Fatalf("expected retry while budget remains, got %#v", decision)
	}

	decision = executor.NextAction(RuntimeState{
		CurrentStepKey: "review",
		Results:        map[string]StepResult{"review": {StepKey: "review", Status: zw.StepStatusFailed}},
		Attempts:       map[string]int{"review": 1},
	})
	if decision.Action != ActionJump || decision.NextStepKey != "developer" {
		t.Fatalf("expected return to developer after retry budget, got %#v", decision)
	}
}

func TestRuntimeCompletionCriteria(t *testing.T) {
	cfg := mustRuntimeConfig(t, StepRuntimeConfig{
		CompletionRules: []CompletionRule{{
			When:   Condition{Field: "output", Operator: "contains", Value: "accepted"},
			Status: zw.StatusDone,
			Reason: "review accepted",
		}},
	})
	executor := NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "review", Mode: ModeReview, OutputSchema: cfg},
		{StepKey: "final", Mode: ModeFinal},
	})

	decision := executor.NextAction(RuntimeState{
		CurrentStepKey: "review",
		Results:        map[string]StepResult{"review": {StepKey: "review", Status: zw.StepStatusDone, Output: "accepted"}},
	})
	if decision.Action != ActionComplete {
		t.Fatalf("expected completion rule to finish lifecycle, got %#v", decision)
	}
}

func TestRuntimeValidationFindsBadReferences(t *testing.T) {
	cfg := mustRuntimeConfig(t, StepRuntimeConfig{ParallelSteps: []string{"missing"}})
	executor := NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{
		{StepKey: "fanout", Mode: ModeParallel, OutputSchema: cfg},
	})
	issues := executor.ValidateRuntime()
	if len(issues) == 0 {
		t.Fatal("expected validation issue")
	}
}

func mustRuntimeConfig(t *testing.T, cfg StepRuntimeConfig) string {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
