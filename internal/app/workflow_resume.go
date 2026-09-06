package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"zavod_ai/internal/chat"
	"zavod_ai/internal/lifecycle"
	"zavod_ai/internal/store"
	zw "zavod_ai/internal/workflow"
)

func purePlanningStep(key string) bool {
	switch key {
	case zw.StepManagerIntake, zw.StepProductRequirements, zw.StepTaskBlueprint, zw.StepArchitectPlan, zw.StepDeveloperPlan:
		return true
	}
	return false
}

func (s *Service) resumeFingerprint(ctx context.Context, projectID string, task chat.Task, state lifecycle.RuntimeState) (string, error) {
	if !purePlanningStep(state.CurrentStepKey) {
		return "", fmt.Errorf("продолжение этого этапа требует проверки побочных эффектов")
	}
	for key, result := range state.Results {
		if !purePlanningStep(key) || (result.Status != zw.StepStatusDone && key != state.CurrentStepKey) {
			return "", fmt.Errorf("состояние шага %s нельзя безопасно восстановить", key)
		}
	}
	ctx = s.chatGroupContext(ctx, task, task.GroupID)
	executor, ok := s.lifecycleExecutor(ctx, projectID)
	if !ok {
		return "", fmt.Errorf("lifecycle недоступен")
	}
	for _, step := range executor.Steps() {
		if _, exists := state.Results[step.StepKey]; exists && step.Mode != "" && step.Mode != lifecycle.ModeLLM {
			return "", fmt.Errorf("повтор инструментального шага требует проверки")
		}
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(executor.Definition())
	_ = json.NewEncoder(h).Encode(executor.Steps())
	var size int64
	err = filepath.WalkDir(project.Path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if entry.IsDir() {
			if path != project.Path {
				switch entry.Name() {
				case ".git", ".venv", "node_modules", ".zavod", "build", "__pycache__":
					return filepath.SkipDir
				}
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("проект содержит специальные файлы")
		}
		size += info.Size()
		if size > 20*1024*1024 {
			return fmt.Errorf("слишком большой снимок для безопасного продолжения")
		}
		relative, _ := filepath.Rel(project.Path, path)
		fmt.Fprintf(h, "%s\x00%d\x00", relative, info.Size())
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(h, io.LimitReader(file, 20*1024*1024+1))
		file.Close()
		return err
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (s *Service) validateContinuation(ctx context.Context, task chat.Task, runID string) (*zw.Run, lifecycle.RuntimeState, error) {
	var state lifecycle.RuntimeState
	run, err := s.store.LatestWorkflowRun(ctx, task.ID)
	if err != nil {
		return nil, state, err
	}
	if run == nil || run.ID != runID || (run.Status != zw.StatusFailed && run.Status != zw.StatusBlocked) {
		return nil, state, fmt.Errorf("этот запуск больше нельзя продолжить")
	}
	failure, err := s.store.WorkflowFailure(ctx, run.ID)
	if err != nil {
		return nil, state, err
	}
	if failure == nil || !failure.CanResume {
		return nil, state, fmt.Errorf("безопасное продолжение недоступно: требуется проверка состояния проекта")
	}
	state, found, err := s.store.LoadLifecycleState(ctx, run.ID)
	if err != nil || !found {
		return nil, state, fmt.Errorf("не найден сохранённый checkpoint")
	}
	fingerprint, err := s.resumeFingerprint(ctx, task.ProjectID, task, state)
	if err != nil {
		return nil, state, err
	}
	if fingerprint != failure.ResumeFingerprint {
		return nil, state, fmt.Errorf("проект или lifecycle изменились после остановки; автоматический повтор небезопасен")
	}
	state.TransitionCount, state.RepairCount = 0, 0
	state.LastFailure, state.LastFailureStep = "", ""
	delete(state.Results, state.CurrentStepKey)
	delete(state.InFlight, state.CurrentStepKey)
	delete(state.Attempts, state.CurrentStepKey)
	delete(state.ExecutionCounts, state.CurrentStepKey)
	return run, state, nil
}

func (s *Service) enableSafeContinuation(ctx context.Context, projectID, taskID, runID string, failure *store.WorkflowFailure) {
	if failure.Provider == nil {
		return
	}
	state, found, err := s.store.LoadLifecycleState(ctx, runID)
	if err != nil || !found {
		return
	}
	task, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	changes, err := s.store.ListProposedChanges(ctx, projectID, runID, 1)
	if err != nil || len(changes) > 0 {
		return
	}
	tests, err := s.store.ListTestRuns(ctx, projectID, runID, 1)
	if err != nil || len(tests) > 0 {
		return
	}
	calls, err := s.store.ListToolInvocations(ctx, taskID)
	if err != nil {
		return
	}
	for _, call := range calls {
		if call.WorkflowRunID == runID {
			return
		}
	}
	fingerprint, err := s.resumeFingerprint(ctx, projectID, task, state)
	if err != nil {
		return
	}
	failure.CanResume, failure.ResumeFingerprint = true, fingerprint
}
