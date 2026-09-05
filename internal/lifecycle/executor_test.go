package lifecycle

import (
	"testing"

	"zavod_ai/internal/agentgroups"
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
