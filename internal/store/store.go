package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zavod_ai/internal/chat"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/workflow"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON;`,
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			path TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			last_opened_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			role TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS model_configs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key_ref TEXT NOT NULL DEFAULT '',
			model_name TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'unknown',
			last_checked_at TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			latency_ms INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS agent_runs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			status TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_runs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			status TEXT NOT NULL,
			current_step TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_run_id TEXT NOT NULL,
			step_key TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			status TEXT NOT NULL,
			input TEXT NOT NULL DEFAULT '',
			output TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_task ON messages(task_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_runs_task ON workflow_runs(task_id, started_at);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_steps_run ON workflow_steps(workflow_run_id, started_at);`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "status", definition: "TEXT NOT NULL DEFAULT 'unknown'"},
		{name: "last_checked_at", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "last_error", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "latency_ms", definition: "INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.ensureColumn(ctx, "model_configs", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table string, column string, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
}

func (s *Store) CountProjects(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects`).Scan(&count)
	return count, err
}

func (s *Store) ListProjects(ctx context.Context, query string) ([]project.Project, error) {
	query = strings.TrimSpace(query)
	args := []any{}
	sqlQuery := `SELECT id, name, path, created_at, last_opened_at FROM projects`
	if query != "" {
		needle := "%" + strings.ToLower(query) + "%"
		sqlQuery += ` WHERE lower(name) LIKE ? OR lower(path) LIKE ?`
		args = append(args, needle, needle)
	}
	sqlQuery += ` ORDER BY last_opened_at DESC, name ASC`

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []project.Project
	for rows.Next() {
		var item project.Project
		if err := rows.Scan(&item.ID, &item.Name, &item.Path, &item.CreatedAt, &item.LastOpenedAt); err != nil {
			return nil, err
		}
		projects = append(projects, item)
	}
	return projects, rows.Err()
}

func (s *Store) GetProject(ctx context.Context, id string) (project.Project, error) {
	var item project.Project
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, path, created_at, last_opened_at
		FROM projects
		WHERE id = ?
	`, id).Scan(&item.ID, &item.Name, &item.Path, &item.CreatedAt, &item.LastOpenedAt)
	return item, err
}

func (s *Store) CreateProject(ctx context.Context, name string, path string) (project.Project, error) {
	now := nowString()
	item := project.Project{
		ID:           newID("project"),
		Name:         strings.TrimSpace(name),
		Path:         path,
		CreatedAt:    now,
		LastOpenedAt: now,
	}
	if item.Name == "" {
		item.Name = filepath.Base(path)
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, name, path, created_at, last_opened_at)
		VALUES (?, ?, ?, ?, ?)
	`, item.ID, item.Name, item.Path, item.CreatedAt, item.LastOpenedAt)
	if err != nil {
		return project.Project{}, err
	}
	return item, nil
}

func (s *Store) TouchProject(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE projects SET last_opened_at = ? WHERE id = ?
	`, nowString(), id)
	return err
}

func (s *Store) GetActiveTask(ctx context.Context, projectID string) (*chat.Task, error) {
	var item chat.Task
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, title, status, created_at, updated_at
		FROM tasks
		WHERE project_id = ? AND status = 'active'
		ORDER BY created_at DESC
		LIMIT 1
	`, projectID).Scan(&item.ID, &item.ProjectID, &item.Title, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) CreateTask(ctx context.Context, projectID string, title string) (chat.Task, error) {
	now := nowString()
	item := chat.Task{
		ID:        newID("task"),
		ProjectID: projectID,
		Title:     strings.TrimSpace(title),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if item.Title == "" {
		item.Title = "Новая задача"
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, project_id, title, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.ID, item.ProjectID, item.Title, item.Status, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return chat.Task{}, err
	}
	return item, nil
}

func (s *Store) TouchTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE tasks SET updated_at = ? WHERE id = ?
	`, nowString(), id)
	return err
}

func (s *Store) AddMessage(ctx context.Context, taskID string, role string, agentID string, content string) (chat.Message, error) {
	item := chat.Message{
		ID:        newID("msg"),
		TaskID:    taskID,
		Role:      role,
		AgentID:   agentID,
		Content:   strings.TrimSpace(content),
		CreatedAt: nowString(),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messages (id, task_id, role, agent_id, content, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.ID, item.TaskID, item.Role, item.AgentID, item.Content, item.CreatedAt)
	if err != nil {
		return chat.Message{}, err
	}
	return item, nil
}

func (s *Store) ListMessages(ctx context.Context, taskID string) ([]chat.Message, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, task_id, role, agent_id, content, created_at
		FROM messages
		WHERE task_id = ?
		ORDER BY created_at ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []chat.Message
	for rows.Next() {
		var item chat.Message
		if err := rows.Scan(&item.ID, &item.TaskID, &item.Role, &item.AgentID, &item.Content, &item.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, item)
	}
	return messages, rows.Err()
}

func (s *Store) CreateAgentRun(ctx context.Context, taskID string, agentID string) (string, error) {
	id := newID("run")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_runs (id, task_id, agent_id, status, started_at)
		VALUES (?, ?, ?, 'running', ?)
	`, id, taskID, agentID, nowString())
	return id, err
}

func (s *Store) FinishAgentRun(ctx context.Context, id string, status string, errText string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_runs
		SET status = ?, finished_at = ?, error = ?
		WHERE id = ?
	`, status, nowString(), errText, id)
	return err
}

func (s *Store) CreateWorkflowRun(ctx context.Context, taskID string) (workflow.Run, error) {
	item := workflow.Run{
		ID:        newID("workflow"),
		TaskID:    taskID,
		Status:    workflow.StatusRunning,
		StartedAt: nowString(),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workflow_runs (id, task_id, status, current_step, started_at)
		VALUES (?, ?, ?, ?, ?)
	`, item.ID, item.TaskID, item.Status, item.CurrentStep, item.StartedAt)
	if err != nil {
		return workflow.Run{}, err
	}
	return item, nil
}

func (s *Store) UpdateWorkflowRun(ctx context.Context, id string, status string, currentStep string, errText string) error {
	finishedAt := ""
	if status == workflow.StatusDone || status == workflow.StatusFailed || status == workflow.StatusWaitingUser {
		finishedAt = nowString()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflow_runs
		SET status = ?, current_step = ?, finished_at = ?, error = ?
		WHERE id = ?
	`, status, currentStep, finishedAt, errText, id)
	return err
}

func (s *Store) LatestWorkflowRun(ctx context.Context, taskID string) (*workflow.Run, error) {
	var item workflow.Run
	err := s.db.QueryRowContext(ctx, `
		SELECT id, task_id, status, current_step, started_at, finished_at, error
		FROM workflow_runs
		WHERE task_id = ?
		ORDER BY started_at DESC
		LIMIT 1
	`, taskID).Scan(&item.ID, &item.TaskID, &item.Status, &item.CurrentStep, &item.StartedAt, &item.FinishedAt, &item.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) CreateWorkflowStep(ctx context.Context, workflowRunID string, stepKey string, agentID string, input string) (workflow.Step, error) {
	item := workflow.Step{
		ID:            newID("step"),
		WorkflowRunID: workflowRunID,
		StepKey:       strings.TrimSpace(stepKey),
		AgentID:       strings.TrimSpace(agentID),
		Status:        workflow.StepStatusRunning,
		Input:         strings.TrimSpace(input),
		StartedAt:     nowString(),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workflow_steps (id, workflow_run_id, step_key, agent_id, status, input, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.WorkflowRunID, item.StepKey, item.AgentID, item.Status, item.Input, item.StartedAt)
	if err != nil {
		return workflow.Step{}, err
	}
	return item, nil
}

func (s *Store) FinishWorkflowStep(ctx context.Context, id string, status string, output string, errText string) (workflow.Step, error) {
	finishedAt := nowString()
	_, err := s.db.ExecContext(ctx, `
		UPDATE workflow_steps
		SET status = ?, output = ?, finished_at = ?, error = ?
		WHERE id = ?
	`, status, strings.TrimSpace(output), finishedAt, errText, id)
	if err != nil {
		return workflow.Step{}, err
	}
	return s.GetWorkflowStep(ctx, id)
}

func (s *Store) GetWorkflowStep(ctx context.Context, id string) (workflow.Step, error) {
	var item workflow.Step
	err := s.db.QueryRowContext(ctx, `
		SELECT id, workflow_run_id, step_key, agent_id, status, input, output, started_at, finished_at, error
		FROM workflow_steps
		WHERE id = ?
	`, id).Scan(
		&item.ID,
		&item.WorkflowRunID,
		&item.StepKey,
		&item.AgentID,
		&item.Status,
		&item.Input,
		&item.Output,
		&item.StartedAt,
		&item.FinishedAt,
		&item.Error,
	)
	return item, err
}

func (s *Store) ListWorkflowSteps(ctx context.Context, workflowRunID string) ([]workflow.Step, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_run_id, step_key, agent_id, status, input, output, started_at, finished_at, error
		FROM workflow_steps
		WHERE workflow_run_id = ?
		ORDER BY started_at ASC
	`, workflowRunID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []workflow.Step
	for rows.Next() {
		var item workflow.Step
		if err := rows.Scan(
			&item.ID,
			&item.WorkflowRunID,
			&item.StepKey,
			&item.AgentID,
			&item.Status,
			&item.Input,
			&item.Output,
			&item.StartedAt,
			&item.FinishedAt,
			&item.Error,
		); err != nil {
			return nil, err
		}
		steps = append(steps, item)
	}
	return steps, rows.Err()
}

func (s *Store) EnsureDefaultModels(ctx context.Context) error {
	now := nowString()
	defaults := []llm.ModelConfig{
		{
			ID:        "qwen-remote",
			Name:      "Qwen по сети",
			Provider:  "remote-qwen",
			BaseURL:   "http://192.168.50.120:8000/v1",
			APIKeyRef: "",
			ModelName: "qwen3:8b",
			IsActive:  true,
			Status:    "unknown",
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:        "openai-chatgpt",
			Name:      "OpenAI ChatGPT",
			Provider:  "openai",
			BaseURL:   "https://api.openai.com/v1",
			APIKeyRef: "OPENAI_API_KEY",
			ModelName: "gpt-5-mini",
			IsActive:  false,
			Status:    "unknown",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	for _, model := range defaults {
		active := 0
		if model.IsActive {
			active = 1
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO model_configs
				(id, name, provider, base_url, api_key_ref, model_name, is_active, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, model.ID, model.Name, model.Provider, model.BaseURL, model.APIKeyRef, model.ModelName, active, model.Status, model.CreatedAt, model.UpdatedAt)
		if err != nil {
			return err
		}
	}

	var activeCount int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_configs WHERE is_active = 1`).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount == 0 {
		_, err := s.db.ExecContext(ctx, `UPDATE model_configs SET is_active = 1 WHERE id = 'qwen-remote'`)
		return err
	}
	return nil
}

func (s *Store) ListModelConfigs(ctx context.Context) ([]llm.ModelConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, provider, base_url, api_key_ref, model_name, is_active,
			status, last_checked_at, last_error, latency_ms, created_at, updated_at
		FROM model_configs
		ORDER BY is_active DESC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []llm.ModelConfig
	for rows.Next() {
		var model llm.ModelConfig
		var active int
		if err := rows.Scan(
			&model.ID,
			&model.Name,
			&model.Provider,
			&model.BaseURL,
			&model.APIKeyRef,
			&model.ModelName,
			&active,
			&model.Status,
			&model.LastCheckedAt,
			&model.LastError,
			&model.LatencyMS,
			&model.CreatedAt,
			&model.UpdatedAt,
		); err != nil {
			return nil, err
		}
		model.IsActive = active == 1
		if model.Status == "" {
			model.Status = "unknown"
		}
		models = append(models, model)
	}
	return models, rows.Err()
}

func (s *Store) ActiveModelConfig(ctx context.Context) (llm.ModelConfig, error) {
	var model llm.ModelConfig
	var active int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, provider, base_url, api_key_ref, model_name, is_active,
			status, last_checked_at, last_error, latency_ms, created_at, updated_at
		FROM model_configs
		ORDER BY is_active DESC, name ASC
		LIMIT 1
	`).Scan(
		&model.ID,
		&model.Name,
		&model.Provider,
		&model.BaseURL,
		&model.APIKeyRef,
		&model.ModelName,
		&active,
		&model.Status,
		&model.LastCheckedAt,
		&model.LastError,
		&model.LatencyMS,
		&model.CreatedAt,
		&model.UpdatedAt,
	)
	if err != nil {
		return llm.ModelConfig{}, err
	}
	model.IsActive = active == 1
	if model.Status == "" {
		model.Status = "unknown"
	}
	return model, nil
}

func (s *Store) GetModelConfig(ctx context.Context, id string) (llm.ModelConfig, error) {
	var model llm.ModelConfig
	var active int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, provider, base_url, api_key_ref, model_name, is_active,
			status, last_checked_at, last_error, latency_ms, created_at, updated_at
		FROM model_configs
		WHERE id = ?
	`, id).Scan(
		&model.ID,
		&model.Name,
		&model.Provider,
		&model.BaseURL,
		&model.APIKeyRef,
		&model.ModelName,
		&active,
		&model.Status,
		&model.LastCheckedAt,
		&model.LastError,
		&model.LatencyMS,
		&model.CreatedAt,
		&model.UpdatedAt,
	)
	if err != nil {
		return llm.ModelConfig{}, err
	}
	model.IsActive = active == 1
	if model.Status == "" {
		model.Status = "unknown"
	}
	return model, nil
}

func (s *Store) SaveModelConfig(ctx context.Context, model llm.ModelConfig) (llm.ModelConfig, error) {
	now := nowString()
	if model.ID == "" {
		model.ID = newID("model")
		model.CreatedAt = now
	}
	if model.Status == "" {
		model.Status = "unknown"
	}
	model.UpdatedAt = now

	active := 0
	if model.IsActive {
		active = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO model_configs
			(id, name, provider, base_url, api_key_ref, model_name, is_active,
			 status, last_checked_at, last_error, latency_ms, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			provider = excluded.provider,
			base_url = excluded.base_url,
			api_key_ref = excluded.api_key_ref,
			model_name = excluded.model_name,
			is_active = excluded.is_active,
			updated_at = excluded.updated_at
	`,
		model.ID,
		model.Name,
		model.Provider,
		model.BaseURL,
		model.APIKeyRef,
		model.ModelName,
		active,
		model.Status,
		model.LastCheckedAt,
		model.LastError,
		model.LatencyMS,
		model.CreatedAt,
		model.UpdatedAt,
	)
	if err != nil {
		return llm.ModelConfig{}, err
	}
	return model, nil
}

func (s *Store) UpdateModelCheck(ctx context.Context, result llm.ModelCheckResult) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE model_configs
		SET status = ?, last_checked_at = ?, last_error = ?, latency_ms = ?, updated_at = ?
		WHERE id = ?
	`, result.Status, result.LastCheckedAt, result.LastError, result.LatencyMS, nowString(), result.ModelID)
	return err
}

func (s *Store) SetActiveModel(ctx context.Context, modelID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE model_configs SET is_active = 0`); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE model_configs SET is_active = 1, updated_at = ? WHERE id = ?`, nowString(), modelID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("модель %s не найдена", modelID)
	}
	return tx.Commit()
}

func (s *Store) SetSetting(ctx context.Context, key string, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	return err
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
