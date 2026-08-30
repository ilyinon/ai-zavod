package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"zavod_ai/internal/artifacts"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/reviews"
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

	artifact, err := s.CreateArtifact(ctx, artifacts.Artifact{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		AgentID:       "manager",
		Kind:          "task_spec",
		Title:         "Спека задачи",
		Path:          filepath.Join(project.Path, "docs", "task-spec.md"),
		RelativePath:  filepath.Join("docs", "task-spec.md"),
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if artifact.ID == "" {
		t.Fatal("expected artifact id")
	}

	artifactsList, err := s.ListArtifacts(ctx, project.ID, 10)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(artifactsList) != 1 || artifactsList[0].RelativePath != filepath.Join("docs", "task-spec.md") {
		t.Fatalf("expected persisted artifact, got %#v", artifactsList)
	}

	proposed, err := s.CreateProposedChange(ctx, changes.ProposedChange{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		AgentID:       "developer",
		FilePath:      "check_llm.py",
		Action:        changes.ActionCreate,
		Content:       "print('ok')\n",
		Reason:        "проверка LLM",
		Status:        changes.StatusPending,
	})
	if err != nil {
		t.Fatalf("create proposed change: %v", err)
	}
	if proposed.ID == "" || proposed.Status != changes.StatusPending {
		t.Fatalf("unexpected proposed change: %#v", proposed)
	}
	pendingChanges, err := s.ListPendingProposedChanges(ctx, workflowRun.ID)
	if err != nil {
		t.Fatalf("list pending proposed changes: %v", err)
	}
	if len(pendingChanges) != 1 || pendingChanges[0].FilePath != "check_llm.py" {
		t.Fatalf("expected pending change, got %#v", pendingChanges)
	}
	if err := s.MarkProposedChangeApplied(
		ctx,
		proposed.ID,
		filepath.Join(".zavod", "backups", proposed.ID, "check_llm.py"),
		"old",
		"new",
		"--- a/check_llm.py\n+++ b/check_llm.py\n-old\n+new",
	); err != nil {
		t.Fatalf("mark proposed change applied: %v", err)
	}
	allChanges, err := s.ListProposedChanges(ctx, project.ID, workflowRun.ID, 10)
	if err != nil {
		t.Fatalf("list proposed changes: %v", err)
	}
	if len(allChanges) != 1 ||
		allChanges[0].Status != changes.StatusApplied ||
		allChanges[0].AppliedAt == "" ||
		allChanges[0].BeforeContent != "old" ||
		allChanges[0].AfterContent != "new" ||
		!strings.Contains(allChanges[0].DiffText, "+new") {
		t.Fatalf("expected applied change, got %#v", allChanges)
	}

	testRun, err := s.CreateTestRun(ctx, checks.TestRun{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		Command:       "go test ./...",
		Reason:        "backend",
		Status:        checks.StatusPending,
	})
	if err != nil {
		t.Fatalf("create test run: %v", err)
	}
	if testRun.ID == "" || testRun.Status != checks.StatusPending {
		t.Fatalf("unexpected test run: %#v", testRun)
	}
	if err := s.MarkTestRunRunning(ctx, testRun.ID); err != nil {
		t.Fatalf("mark test run running: %v", err)
	}
	if err := s.FinishTestRun(ctx, testRun.ID, checks.RunResult{
		Status:   checks.StatusPassed,
		ExitCode: 0,
		Stdout:   "ok",
	}); err != nil {
		t.Fatalf("finish test run: %v", err)
	}
	testRuns, err := s.ListTestRuns(ctx, project.ID, workflowRun.ID, 10)
	if err != nil {
		t.Fatalf("list test runs: %v", err)
	}
	if len(testRuns) != 1 || testRuns[0].Status != checks.StatusPassed || testRuns[0].Stdout != "ok" {
		t.Fatalf("expected passed test run, got %#v", testRuns)
	}

	reviewRun, err := s.CreateReviewRun(ctx, reviews.ReviewRun{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRun.ID,
		Status:        reviews.StatusRunning,
	})
	if err != nil {
		t.Fatalf("create review run: %v", err)
	}
	if err := s.FinishReviewRun(ctx, reviewRun.ID, reviews.ParsedReview{
		Status:  reviews.StatusNeedsWork,
		Summary: "Нужна доработка",
		Findings: []reviews.Finding{{
			Severity:   "major",
			FilePath:   "check_llm.py",
			Message:    "нет проверки статуса",
			Suggestion: "добавить обработку non-2xx",
		}},
		RequiredChanges:     []string{"добавить обработку non-2xx"},
		RecommendedNextStep: "Вернуть Разработчику",
	}, ""); err != nil {
		t.Fatalf("finish review run: %v", err)
	}
	reviewRuns, err := s.ListReviewRuns(ctx, project.ID, workflowRun.ID, 10)
	if err != nil {
		t.Fatalf("list review runs: %v", err)
	}
	if len(reviewRuns) != 1 ||
		reviewRuns[0].Status != reviews.StatusNeedsWork ||
		len(reviewRuns[0].Findings) != 1 ||
		reviewRuns[0].RequiredChanges[0] != "добавить обработку non-2xx" {
		t.Fatalf("expected persisted review run, got %#v", reviewRuns)
	}
}
