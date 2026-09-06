package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/agents"
	"zavod_ai/internal/lifecycle"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/store"
	zw "zavod_ai/internal/workflow"
)

func TestBubbleSortQuestionNeverStartsDevelopment(t *testing.T) {
	for _, bound := range []bool{false, true} {
		t.Run(map[bool]string{false: "projectless", true: "dev_project"}[bound], func(t *testing.T) {
			ctx := context.Background()
			s := chatTestService(t)
			s.paths.AgentsDir = t.TempDir()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var req struct {
					Tools    []any
					Messages []llm.Message
				}
				json.NewDecoder(r.Body).Decode(&req)
				if len(req.Tools) > 0 {
					t.Error("explanation received tools")
				}
				for _, msg := range req.Messages {
					if strings.Contains(msg.Content, "OLD_FAILED_TASK") {
						t.Error("previous workflow contaminated explanation")
					}
				}
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": "Пример:\n```go\npackage main\n```"}}}})
			}))
			defer server.Close()
			model, _ := s.store.ActiveModelConfig(ctx)
			model.BaseURL = server.URL
			s.store.SaveModelConfig(ctx, model)
			projectID := ""
			root := ""
			oldRunID := ""
			if bound {
				root = t.TempDir()
				p, err := s.store.CreateProject(ctx, "Dev", root)
				if err != nil {
					t.Fatal(err)
				}
				projectID = p.ID
			}
			created, err := s.CreateChat(ctx, CreateChatInput{ProjectID: projectID})
			if err != nil {
				t.Fatal(err)
			}
			if bound {
				old, _ := s.store.CreateWorkflowRun(ctx, created.Task.ID)
				oldRunID = old.ID
				s.store.UpdateWorkflowRun(ctx, old.ID, zw.StatusFailed, zw.StepDeveloperPlan, "OLD_FAILED_TASK")
				s.store.AddMessage(ctx, created.Task.ID, "user", "", "OLD_FAILED_TASK")
			}
			state, err := s.SendMessage(ctx, SendMessageInput{ProjectID: projectID, TaskID: created.Task.ID, Content: "как написать сортировку пузырько в Go lang?", ToolConsentModelID: model.ID})
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 || state.RequestState == nil || state.RequestState.Mode != "direct" || len(state.ToolInvocations) > 0 || state.Task.PendingRequest != "" {
				t.Fatalf("incorrect routing: calls=%d state=%+v", calls.Load(), state.RequestState)
			}
			if bound {
				if state.WorkflowRun.ID != oldRunID {
					t.Fatal("created new workflow")
				}
				entries, _ := os.ReadDir(root)
				if len(entries) > 0 {
					t.Fatalf("question wrote files: %v", entries)
				}
			} else if state.WorkflowRun != nil {
				t.Fatal("projectless question created workflow")
			}
		})
	}
}

func TestAmbiguousChoiceIsBoundToRequest(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) }))
	defer server.Close()
	model, _ := s.store.ActiveModelConfig(ctx)
	model.BaseURL = server.URL
	s.store.SaveModelConfig(ctx, model)
	created, _ := s.CreateChat(ctx, CreateChatInput{})
	state, err := s.SendMessage(ctx, SendMessageInput{TaskID: created.Task.ID, Content: "Нужна сортировка"})
	if err != nil || state.RequestState == nil || state.RequestState.Mode != "clarify" || state.WorkflowRun != nil || state.Task.PendingRequest != "" {
		t.Fatalf("missing safe clarification: %+v %v", state.RequestState, err)
	}
	id := state.RequestState.ID
	chosen, err := s.SendMessage(ctx, SendMessageInput{TaskID: created.Task.ID, Content: "Добавить в проект", RoutingAnswerFor: id})
	if err != nil || !strings.Contains(chosen.Task.PendingRequest, "Нужна сортировка") {
		t.Fatalf("lost original request: %+v %v", chosen.Task, err)
	}
	if _, err = s.SendMessage(ctx, SendMessageInput{TaskID: created.Task.ID, Content: "Показать в чате", RoutingAnswerFor: id}); err == nil {
		t.Fatal("stale choice accepted")
	}
}

func TestProviderFailureStopsLifecycleAndPersistsBudgets(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	p, _ := s.store.CreateProject(ctx, "Dev", t.TempDir())
	created, _ := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	run, _ := s.store.CreateWorkflowRun(ctx, created.Task.ID)
	executor := lifecycle.NewExecutor(agentgroups.LifecycleDefinition{MaxRepairIterations: 3}, []agentgroups.LifecycleStep{{StepKey: "develop", CanRetry: true, MaxRetries: 3, OnFailureStepKey: "develop"}})
	failure := &llm.ProviderError{Kind: "provider_timeout", Attempt: 3, MaxAttempts: 3}
	calls := 0
	execute := func(key string, force bool) (string, error) { calls++; return "", failure }
	err := s.runV03RuntimeLifecycle(ctx, &run, &v03WorkflowResult{}, execute, executor)
	if !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("transport error entered repair: %v calls=%d", err, calls)
	}
	state, found, err := s.store.LoadLifecycleState(ctx, run.ID)
	if err != nil || !found || state.ExecutionCounts["develop"] != 1 || !state.Results["develop"].Terminal {
		t.Fatalf("missing checkpoint: %+v %v", state, err)
	}
	_ = s.runV03RuntimeLifecycle(ctx, &run, &v03WorkflowResult{}, execute, executor)
	if calls != 1 {
		t.Fatal("restoration retried exhausted provider")
	}
	ctx = context.WithValue(ctx, chatContextKey{}, created.Task.ID)
	s.setAgentStatus(agents.DeveloperID, "calling_model", "waiting", "mock")
	result := s.handleWorkflowError(ctx, p.ID, created.Task.ID, run.ID, "develop", "mock", failure)
	if result.WorkflowFailure == nil || result.WorkflowFailure.Kind != "provider_timeout" || strings.Contains(result.Messages[len(result.Messages)-1].Content, "уточни задачу") {
		t.Fatalf("wrong failure presentation: %+v", result.WorkflowFailure)
	}
	for _, agent := range result.Agents {
		if agent.Status == "calling_model" {
			t.Fatal("agent still running")
		}
	}
}

func TestSafeContinuationChecksWorkspaceAndDoesNotCloneFailedStep(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	root := t.TempDir()
	p, _ := s.store.CreateProject(ctx, "Dev", root)
	created, _ := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	run, _ := s.store.CreateWorkflowRun(ctx, created.Task.ID)
	for _, key := range []string{zw.StepManagerIntake, zw.StepDeveloperPlan} {
		step, _ := s.store.CreateWorkflowStep(ctx, run.ID, key, agents.ManagerID, "input")
		s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusDone, "old output", "")
	}
	state := lifecycle.RuntimeState{CurrentStepKey: zw.StepDeveloperPlan, Results: map[string]lifecycle.StepResult{zw.StepManagerIntake: {StepKey: zw.StepManagerIntake, Status: zw.StepStatusDone}, zw.StepDeveloperPlan: {StepKey: zw.StepDeveloperPlan, Status: zw.StepStatusFailed, Terminal: true}}, ExecutionCounts: map[string]int{zw.StepManagerIntake: 1, zw.StepDeveloperPlan: 2}, Attempts: map[string]int{}, HumanGates: map[string]bool{}}
	s.store.SaveLifecycleState(ctx, run.ID, state)
	s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepDeveloperPlan, "timeout")
	failure := store.WorkflowFailure{RunID: run.ID, Provider: &llm.ProviderError{Kind: "provider_timeout"}}
	s.enableSafeContinuation(ctx, p.ID, created.Task.ID, run.ID, &failure)
	if !failure.CanResume {
		t.Fatal("pure preparation cannot resume")
	}
	s.store.SaveWorkflowFailure(ctx, failure)
	parent, snapshot, err := s.validateContinuation(ctx, *created.Task, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "changed.go"), []byte("package main"), 0600)
	if _, _, err := s.validateContinuation(ctx, *created.Task, run.ID); err == nil {
		t.Fatal("changed project accepted")
	}
	child, err := s.store.ContinueWorkflow(ctx, *parent, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	steps, _ := s.store.ListWorkflowSteps(ctx, child.ID)
	if len(steps) != 1 || steps[0].StepKey != zw.StepManagerIntake {
		t.Fatalf("cloned failed step output: %+v", steps)
	}
	if _, err := s.store.ContinueWorkflow(ctx, *parent, snapshot); err == nil {
		t.Fatal("duplicate continuation accepted")
	}
	latest, _ := s.store.LatestWorkflowRun(ctx, created.Task.ID)
	if latest.ID != child.ID {
		t.Fatal("latest run tie selected parent")
	}
}

func TestRestoredCompletedInflightAttemptIsNotReplayed(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	p, _ := s.store.CreateProject(ctx, "Dev", t.TempDir())
	created, _ := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	run, _ := s.store.CreateWorkflowRun(ctx, created.Task.ID)
	step, _ := s.store.CreateWorkflowStep(ctx, run.ID, "one", agents.ManagerID, "input")
	s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusDone, "persisted response", "")
	snapshot := lifecycle.RuntimeState{CurrentStepKey: "one", InFlight: map[string]int{"one": 1}, ExecutionCounts: map[string]int{"one": 1}, Results: map[string]lifecycle.StepResult{}, TransitionCount: 1}
	if err := s.store.SaveLifecycleState(ctx, run.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	executor := lifecycle.NewExecutor(agentgroups.LifecycleDefinition{}, []agentgroups.LifecycleStep{{StepKey: "one"}})
	err := s.runV03RuntimeLifecycle(ctx, &run, &v03WorkflowResult{}, func(string, bool) (string, error) { t.Fatal("replayed completed call after crash"); return "", nil }, executor)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHiddenRuntimeStepDoesNotAdvanceVisiblePlan(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	p, _ := s.store.CreateProject(ctx, "Dev", t.TempDir())
	created, _ := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	run, _ := s.store.CreateWorkflowRun(ctx, created.Task.ID)
	_, _, err := s.store.CreateWorkflowPlan(ctx, zw.Plan{ProjectID: p.ID, TaskID: created.Task.ID, WorkflowRunID: run.ID, Title: "visible"}, []zw.PlanStep{{StepKey: "visible", Title: "Visible", AgentID: agents.ManagerID, Status: zw.StepStatusQueued}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.FinishWorkflowPlanStep(ctx, run.ID, "hidden", agents.ManagerID, zw.StepStatusDone, ""); err != nil {
		t.Fatal(err)
	}
	_, steps, _ := s.store.LatestWorkflowPlan(ctx, run.ID)
	if len(steps) != 1 || steps[0].Status != zw.StepStatusQueued {
		t.Fatalf("hidden step changed progress: %+v", steps)
	}
}
