package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStorePersistsV01Entities(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "zavod.db")

	s, err := New(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	if err := s.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure default models: %v", err)
	}

	model, err := s.ActiveModelConfig(ctx)
	if err != nil {
		t.Fatalf("active model: %v", err)
	}
	if model.ID == "" {
		t.Fatal("expected active model")
	}

	project, err := s.CreateProject(ctx, "Тестовый проект", filepath.Join(t.TempDir(), "project"))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	task, err := s.CreateTask(ctx, project.ID, "Первая задача")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	if _, err := s.AddMessage(ctx, task.ID, "user", "", "Сделай план"); err != nil {
		t.Fatalf("add user message: %v", err)
	}
	if _, err := s.AddMessage(ctx, task.ID, "agent", "manager", "План готов"); err != nil {
		t.Fatalf("add agent message: %v", err)
	}

	messages, err := s.ListMessages(ctx, task.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	runID, err := s.CreateAgentRun(ctx, task.ID, "manager")
	if err != nil {
		t.Fatalf("create agent run: %v", err)
	}
	if err := s.FinishAgentRun(ctx, runID, "done", ""); err != nil {
		t.Fatalf("finish agent run: %v", err)
	}

	workflowRun, err := s.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	step, err := s.CreateWorkflowStep(ctx, workflowRun.ID, "manager_intake", "manager", "input")
	if err != nil {
		t.Fatalf("create workflow step: %v", err)
	}
	if _, err := s.FinishWorkflowStep(ctx, step.ID, "done", "output", ""); err != nil {
		t.Fatalf("finish workflow step: %v", err)
	}
	if err := s.UpdateWorkflowRun(ctx, workflowRun.ID, "done", "manager_intake", ""); err != nil {
		t.Fatalf("finish workflow run: %v", err)
	}

	latestRun, err := s.LatestWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("latest workflow run: %v", err)
	}
	if latestRun == nil || latestRun.ID != workflowRun.ID {
		t.Fatalf("expected latest workflow run %q, got %#v", workflowRun.ID, latestRun)
	}
	steps, err := s.ListWorkflowSteps(ctx, workflowRun.ID)
	if err != nil {
		t.Fatalf("list workflow steps: %v", err)
	}
	if len(steps) != 1 || steps[0].Output != "output" {
		t.Fatalf("expected persisted workflow step output, got %#v", steps)
	}
}
