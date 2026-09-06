package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"zavod_ai/internal/chat"
	"zavod_ai/internal/lifecycle"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/workflow"
)

type WorkflowFailure struct {
	RunID             string             `json:"runId"`
	StepKey           string             `json:"stepKey"`
	Kind              string             `json:"kind"`
	Message           string             `json:"message"`
	Provider          *llm.ProviderError `json:"provider,omitempty"`
	CanResume         bool               `json:"canResume"`
	ResumeFingerprint string             `json:"resumeFingerprint,omitempty"`
}

func (s *Store) migrateRuntimeState(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS lifecycle_checkpoints(run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id) ON DELETE CASCADE, state TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS workflow_failures(run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id) ON DELETE CASCADE, payload TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS chat_request_state(task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,payload TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS workflow_continuations(parent_run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id) ON DELETE CASCADE, child_run_id TEXT NOT NULL UNIQUE REFERENCES workflow_runs(id) ON DELETE CASCADE);`)
	return err
}
func (s *Store) SaveLifecycleState(ctx context.Context, runID string, state lifecycle.RuntimeState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO lifecycle_checkpoints VALUES(?,?) ON CONFLICT(run_id) DO UPDATE SET state=excluded.state`, runID, string(data))
	return err
}

// Continuations copy observations, never execution history with side effects.
func (s *Store) ContinueWorkflow(ctx context.Context, parent workflow.Run, state lifecycle.RuntimeState) (workflow.Run, error) {
	child := workflow.Run{ID: newID("workflow"), TaskID: parent.TaskID, Status: workflow.StatusRunning, CurrentStep: state.CurrentStepKey, StartedAt: nowString()}
	data, err := json.Marshal(state)
	if err != nil {
		return child, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return child, err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM workflow_runs WHERE id=? AND task_id=?`, parent.ID, parent.TaskID).Scan(&status); err != nil {
		return child, err
	}
	if status != workflow.StatusFailed && status != workflow.StatusBlocked {
		return child, fmt.Errorf("запуск нельзя продолжить в текущем состоянии")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_runs(id,task_id,status,current_step,started_at) VALUES(?,?,?,?,?)`, child.ID, child.TaskID, child.Status, child.CurrentStep, child.StartedAt); err != nil {
		return child, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_continuations(parent_run_id,child_run_id) VALUES(?,?)`, parent.ID, child.ID); err != nil {
		return child, fmt.Errorf("продолжение этого запуска уже создано: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO lifecycle_checkpoints VALUES(?,?)`, child.ID, string(data)); err != nil {
		return child, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_steps(id,workflow_run_id,step_key,agent_id,status,input,output,started_at,finished_at,error)
 SELECT ? || '_' || id, ?,step_key,agent_id,status,input,output,started_at,finished_at,error FROM workflow_steps WHERE workflow_run_id=? AND status='done' AND step_key<>? ORDER BY rowid`, child.ID, child.ID, parent.ID, state.CurrentStepKey); err != nil {
		return child, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO task_blueprints(id,project_id,task_id,workflow_run_id,stack,runtime,project_type,scaffold_required,entrypoints_json,expected_files_json,forbidden_files_json,dependencies_json,test_commands_json,open_questions_json,confidence,raw_json,created_at)
 SELECT ? || '_' || id,project_id,task_id,?,stack,runtime,project_type,scaffold_required,entrypoints_json,expected_files_json,forbidden_files_json,dependencies_json,test_commands_json,open_questions_json,confidence,raw_json,created_at FROM task_blueprints WHERE workflow_run_id=?`, child.ID, child.ID, parent.ID); err != nil {
		return child, err
	}
	return child, tx.Commit()
}
func (s *Store) LoadLifecycleState(ctx context.Context, runID string) (lifecycle.RuntimeState, bool, error) {
	var data string
	var state lifecycle.RuntimeState
	err := s.db.QueryRowContext(ctx, `SELECT state FROM lifecycle_checkpoints WHERE run_id=?`, runID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return state, false, nil
	}
	if err != nil {
		return state, false, err
	}
	err = json.Unmarshal([]byte(data), &state)
	return state, true, err
}
func (s *Store) SaveWorkflowFailure(ctx context.Context, failure WorkflowFailure) error {
	data, err := json.Marshal(failure)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO workflow_failures VALUES(?,?) ON CONFLICT(run_id) DO UPDATE SET payload=excluded.payload`, failure.RunID, string(data))
	return err
}
func (s *Store) WorkflowFailure(ctx context.Context, runID string) (*WorkflowFailure, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM workflow_failures WHERE run_id=?`, runID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f WorkflowFailure
	err = json.Unmarshal([]byte(data), &f)
	return &f, err
}
func (s *Store) SaveRequestState(ctx context.Context, taskID string, state chat.RequestState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO chat_request_state VALUES(?,?) ON CONFLICT(task_id) DO UPDATE SET payload=excluded.payload`, taskID, string(data))
	return err
}
func (s *Store) RequestState(ctx context.Context, taskID string) (*chat.RequestState, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM chat_request_state WHERE task_id=?`, taskID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r chat.RequestState
	err = json.Unmarshal([]byte(data), &r)
	return &r, err
}
