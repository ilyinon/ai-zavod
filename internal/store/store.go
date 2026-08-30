package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zavod_ai/internal/artifacts"
	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/reviews"
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
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_run_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			title TEXT NOT NULL,
			path TEXT NOT NULL,
			relative_path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS proposed_changes (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_run_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			file_path TEXT NOT NULL,
			action TEXT NOT NULL,
			content TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			backup_path TEXT NOT NULL DEFAULT '',
			before_content TEXT NOT NULL DEFAULT '',
			after_content TEXT NOT NULL DEFAULT '',
			diff_text TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS test_runs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_run_id TEXT NOT NULL,
			command TEXT NOT NULL,
			working_dir TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			exit_code INTEGER NOT NULL DEFAULT 0,
			stdout TEXT NOT NULL DEFAULT '',
			stderr TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS review_runs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_run_id TEXT NOT NULL,
			status TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			findings_json TEXT NOT NULL DEFAULT '[]',
			required_changes_json TEXT NOT NULL DEFAULT '[]',
			recommended_next_step TEXT NOT NULL DEFAULT '',
			return_to TEXT NOT NULL DEFAULT '',
			iteration INTEGER NOT NULL DEFAULT 0,
			blocking_reason TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS task_blueprints (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_run_id TEXT NOT NULL,
			stack TEXT NOT NULL,
			runtime TEXT NOT NULL DEFAULT '',
			project_type TEXT NOT NULL DEFAULT '',
			scaffold_required INTEGER NOT NULL DEFAULT 0,
			entrypoints_json TEXT NOT NULL DEFAULT '[]',
			expected_files_json TEXT NOT NULL DEFAULT '[]',
			forbidden_files_json TEXT NOT NULL DEFAULT '[]',
			dependencies_json TEXT NOT NULL DEFAULT '{}',
			test_commands_json TEXT NOT NULL DEFAULT '[]',
			open_questions_json TEXT NOT NULL DEFAULT '[]',
			confidence TEXT NOT NULL DEFAULT 'medium',
			raw_json TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_task ON messages(task_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_runs_task ON workflow_runs(task_id, started_at);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_steps_run ON workflow_steps(workflow_run_id, started_at);`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_project ON artifacts(project_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_proposed_changes_project ON proposed_changes(project_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_proposed_changes_workflow ON proposed_changes(workflow_run_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_test_runs_workflow ON test_runs(workflow_run_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_review_runs_workflow ON review_runs(workflow_run_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_task_blueprints_workflow ON task_blueprints(workflow_run_id, created_at);`,
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
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "before_content", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "after_content", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "diff_text", definition: "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.ensureColumn(ctx, "proposed_changes", column.name, column.definition); err != nil {
			return err
		}
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{name: "return_to", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "iteration", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "blocking_reason", definition: "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := s.ensureColumn(ctx, "review_runs", column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateTaskBlueprint(ctx context.Context, item blueprint.Blueprint) (blueprint.Blueprint, error) {
	item.ID = newID("blueprint")
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	item.TaskID = strings.TrimSpace(item.TaskID)
	item.WorkflowRunID = strings.TrimSpace(item.WorkflowRunID)
	item.Stack = strings.TrimSpace(item.Stack)
	item.Runtime = strings.TrimSpace(item.Runtime)
	item.ProjectType = strings.TrimSpace(item.ProjectType)
	item.Confidence = strings.TrimSpace(item.Confidence)
	item.RawJSON = strings.TrimSpace(item.RawJSON)
	item.CreatedAt = nowString()
	if item.Stack == "" {
		item.Stack = blueprint.StackUnknown
	}
	if item.Confidence == "" {
		item.Confidence = "medium"
	}
	entrypointsJSON := marshalJSON(item.Entrypoints, "[]")
	expectedFilesJSON := marshalJSON(item.ExpectedFiles, "[]")
	forbiddenFilesJSON := marshalJSON(item.ForbiddenFiles, "[]")
	dependenciesJSON := marshalJSON(item.Dependencies, "{}")
	testCommandsJSON := marshalJSON(item.TestCommands, "[]")
	openQuestionsJSON := marshalJSON(item.OpenQuestions, "[]")

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_blueprints
			(id, project_id, task_id, workflow_run_id, stack, runtime, project_type, scaffold_required,
			 entrypoints_json, expected_files_json, forbidden_files_json, dependencies_json,
			 test_commands_json, open_questions_json, confidence, raw_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.ProjectID,
		item.TaskID,
		item.WorkflowRunID,
		item.Stack,
		item.Runtime,
		item.ProjectType,
		boolInt(item.ScaffoldRequired),
		entrypointsJSON,
		expectedFilesJSON,
		forbiddenFilesJSON,
		dependenciesJSON,
		testCommandsJSON,
		openQuestionsJSON,
		item.Confidence,
		item.RawJSON,
		item.CreatedAt,
	)
	if err != nil {
		return blueprint.Blueprint{}, err
	}
	return item, nil
}

func (s *Store) LatestTaskBlueprint(ctx context.Context, workflowRunID string) (*blueprint.Blueprint, error) {
	var item blueprint.Blueprint
	var scaffoldRequired int
	var entrypointsJSON string
	var expectedFilesJSON string
	var forbiddenFilesJSON string
	var dependenciesJSON string
	var testCommandsJSON string
	var openQuestionsJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, stack, runtime, project_type, scaffold_required,
			entrypoints_json, expected_files_json, forbidden_files_json, dependencies_json,
			test_commands_json, open_questions_json, confidence, raw_json, created_at
		FROM task_blueprints
		WHERE workflow_run_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, strings.TrimSpace(workflowRunID)).Scan(
		&item.ID,
		&item.ProjectID,
		&item.TaskID,
		&item.WorkflowRunID,
		&item.Stack,
		&item.Runtime,
		&item.ProjectType,
		&scaffoldRequired,
		&entrypointsJSON,
		&expectedFilesJSON,
		&forbiddenFilesJSON,
		&dependenciesJSON,
		&testCommandsJSON,
		&openQuestionsJSON,
		&item.Confidence,
		&item.RawJSON,
		&item.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.ScaffoldRequired = scaffoldRequired != 0
	_ = json.Unmarshal([]byte(entrypointsJSON), &item.Entrypoints)
	_ = json.Unmarshal([]byte(expectedFilesJSON), &item.ExpectedFiles)
	_ = json.Unmarshal([]byte(forbiddenFilesJSON), &item.ForbiddenFiles)
	_ = json.Unmarshal([]byte(dependenciesJSON), &item.Dependencies)
	_ = json.Unmarshal([]byte(testCommandsJSON), &item.TestCommands)
	_ = json.Unmarshal([]byte(openQuestionsJSON), &item.OpenQuestions)
	return &item, nil
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

func (s *Store) UpdateProject(ctx context.Context, id string, name string, path string) (project.Project, error) {
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if name == "" {
		name = filepath.Base(path)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE projects
		SET name = ?, path = ?
		WHERE id = ?
	`, name, path, strings.TrimSpace(id))
	if err != nil {
		return project.Project{}, err
	}
	return s.GetProject(ctx, id)
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, strings.TrimSpace(id))
	return err
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
	if status == workflow.StatusDone || status == workflow.StatusFailed || status == workflow.StatusWaitingUser || status == workflow.StatusBlocked {
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

func (s *Store) CreateArtifact(ctx context.Context, artifact artifacts.Artifact) (artifacts.Artifact, error) {
	artifact.ID = newID("artifact")
	artifact.ProjectID = strings.TrimSpace(artifact.ProjectID)
	artifact.TaskID = strings.TrimSpace(artifact.TaskID)
	artifact.WorkflowRunID = strings.TrimSpace(artifact.WorkflowRunID)
	artifact.AgentID = strings.TrimSpace(artifact.AgentID)
	artifact.Kind = strings.TrimSpace(artifact.Kind)
	artifact.Title = strings.TrimSpace(artifact.Title)
	artifact.Path = strings.TrimSpace(artifact.Path)
	artifact.RelativePath = strings.TrimSpace(artifact.RelativePath)
	artifact.CreatedAt = nowString()
	if artifact.Title == "" {
		artifact.Title = artifact.RelativePath
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO artifacts
			(id, project_id, task_id, workflow_run_id, agent_id, kind, title, path, relative_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		artifact.ID,
		artifact.ProjectID,
		artifact.TaskID,
		artifact.WorkflowRunID,
		artifact.AgentID,
		artifact.Kind,
		artifact.Title,
		artifact.Path,
		artifact.RelativePath,
		artifact.CreatedAt,
	)
	if err != nil {
		return artifacts.Artifact{}, err
	}
	return artifact, nil
}

func (s *Store) ListArtifacts(ctx context.Context, projectID string, limit int) ([]artifacts.Artifact, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, agent_id, kind, title, path, relative_path, created_at
		FROM artifacts
		WHERE project_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []artifacts.Artifact
	for rows.Next() {
		var item artifacts.Artifact
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.TaskID,
			&item.WorkflowRunID,
			&item.AgentID,
			&item.Kind,
			&item.Title,
			&item.Path,
			&item.RelativePath,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateProposedChange(ctx context.Context, change changes.ProposedChange) (changes.ProposedChange, error) {
	change.ID = newID("change")
	change.ProjectID = strings.TrimSpace(change.ProjectID)
	change.TaskID = strings.TrimSpace(change.TaskID)
	change.WorkflowRunID = strings.TrimSpace(change.WorkflowRunID)
	change.AgentID = strings.TrimSpace(change.AgentID)
	change.FilePath = strings.TrimSpace(change.FilePath)
	change.Action = strings.TrimSpace(change.Action)
	change.Reason = strings.TrimSpace(change.Reason)
	change.Status = strings.TrimSpace(change.Status)
	change.CreatedAt = nowString()
	if change.AgentID == "" {
		change.AgentID = "developer"
	}
	if change.Status == "" {
		change.Status = changes.StatusPending
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO proposed_changes
			(id, project_id, task_id, workflow_run_id, agent_id, file_path, action, content,
			 reason, status, error, backup_path, before_content, after_content, diff_text, created_at, applied_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		change.ID,
		change.ProjectID,
		change.TaskID,
		change.WorkflowRunID,
		change.AgentID,
		change.FilePath,
		change.Action,
		change.Content,
		change.Reason,
		change.Status,
		change.Error,
		change.BackupPath,
		change.BeforeContent,
		change.AfterContent,
		change.DiffText,
		change.CreatedAt,
		change.AppliedAt,
	)
	if err != nil {
		return changes.ProposedChange{}, err
	}
	return change, nil
}

func (s *Store) ListProposedChanges(ctx context.Context, projectID string, workflowRunID string, limit int) ([]changes.ProposedChange, error) {
	if limit <= 0 {
		limit = 30
	}
	args := []any{projectID}
	sqlQuery := `
		SELECT id, project_id, task_id, workflow_run_id, agent_id, file_path, action, content,
			reason, status, error, backup_path, before_content, after_content, diff_text, created_at, applied_at
		FROM proposed_changes
		WHERE project_id = ?
	`
	if strings.TrimSpace(workflowRunID) != "" {
		sqlQuery += ` AND workflow_run_id = ?`
		args = append(args, workflowRunID)
	}
	sqlQuery += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProposedChanges(rows)
}

func (s *Store) ListPendingProposedChanges(ctx context.Context, workflowRunID string) ([]changes.ProposedChange, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, agent_id, file_path, action, content,
			reason, status, error, backup_path, before_content, after_content, diff_text, created_at, applied_at
		FROM proposed_changes
		WHERE workflow_run_id = ? AND status = ?
		ORDER BY created_at ASC
	`, workflowRunID, changes.StatusPending)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProposedChanges(rows)
}

func (s *Store) MarkProposedChangeApplied(ctx context.Context, id string, backupPath string, beforeContent string, afterContent string, diffText string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE proposed_changes
		SET status = ?, error = '', backup_path = ?, before_content = ?, after_content = ?, diff_text = ?, applied_at = ?
		WHERE id = ?
	`, changes.StatusApplied, backupPath, beforeContent, afterContent, diffText, nowString(), id)
	return err
}

func (s *Store) MarkProposedChangeFailed(ctx context.Context, id string, errText string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE proposed_changes
		SET status = ?, error = ?
		WHERE id = ?
	`, changes.StatusFailed, strings.TrimSpace(errText), id)
	return err
}

func scanProposedChanges(rows *sql.Rows) ([]changes.ProposedChange, error) {
	var items []changes.ProposedChange
	for rows.Next() {
		var item changes.ProposedChange
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.TaskID,
			&item.WorkflowRunID,
			&item.AgentID,
			&item.FilePath,
			&item.Action,
			&item.Content,
			&item.Reason,
			&item.Status,
			&item.Error,
			&item.BackupPath,
			&item.BeforeContent,
			&item.AfterContent,
			&item.DiffText,
			&item.CreatedAt,
			&item.AppliedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateTestRun(ctx context.Context, item checks.TestRun) (checks.TestRun, error) {
	item.ID = newID("test")
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	item.TaskID = strings.TrimSpace(item.TaskID)
	item.WorkflowRunID = strings.TrimSpace(item.WorkflowRunID)
	item.Command = strings.TrimSpace(item.Command)
	item.WorkingDir = strings.TrimSpace(item.WorkingDir)
	item.Reason = strings.TrimSpace(item.Reason)
	item.Status = strings.TrimSpace(item.Status)
	item.Error = strings.TrimSpace(item.Error)
	item.CreatedAt = nowString()
	if item.Status == "" {
		item.Status = checks.StatusPending
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO test_runs
			(id, project_id, task_id, workflow_run_id, command, working_dir, reason, status,
			 exit_code, stdout, stderr, error, started_at, finished_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.ProjectID,
		item.TaskID,
		item.WorkflowRunID,
		item.Command,
		item.WorkingDir,
		item.Reason,
		item.Status,
		item.ExitCode,
		item.Stdout,
		item.Stderr,
		item.Error,
		item.StartedAt,
		item.FinishedAt,
		item.CreatedAt,
	)
	if err != nil {
		return checks.TestRun{}, err
	}
	return item, nil
}

func (s *Store) ListTestRuns(ctx context.Context, projectID string, workflowRunID string, limit int) ([]checks.TestRun, error) {
	if limit <= 0 {
		limit = 30
	}
	args := []any{projectID}
	sqlQuery := `
		SELECT id, project_id, task_id, workflow_run_id, command, working_dir, reason, status,
			exit_code, stdout, stderr, error, started_at, finished_at, created_at
		FROM test_runs
		WHERE project_id = ?
	`
	if strings.TrimSpace(workflowRunID) != "" {
		sqlQuery += ` AND workflow_run_id = ?`
		args = append(args, workflowRunID)
	}
	sqlQuery += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTestRuns(rows)
}

func (s *Store) GetTestRun(ctx context.Context, id string) (checks.TestRun, error) {
	var item checks.TestRun
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, command, working_dir, reason, status,
			exit_code, stdout, stderr, error, started_at, finished_at, created_at
		FROM test_runs
		WHERE id = ?
	`, strings.TrimSpace(id)).Scan(
		&item.ID,
		&item.ProjectID,
		&item.TaskID,
		&item.WorkflowRunID,
		&item.Command,
		&item.WorkingDir,
		&item.Reason,
		&item.Status,
		&item.ExitCode,
		&item.Stdout,
		&item.Stderr,
		&item.Error,
		&item.StartedAt,
		&item.FinishedAt,
		&item.CreatedAt,
	)
	return item, err
}

func (s *Store) MarkTestRunRunning(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE test_runs
		SET status = ?, started_at = ?, finished_at = '', stdout = '', stderr = '', error = '', exit_code = 0
		WHERE id = ?
	`, checks.StatusRunning, nowString(), strings.TrimSpace(id))
	return err
}

func (s *Store) FinishTestRun(ctx context.Context, id string, result checks.RunResult) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE test_runs
		SET status = ?, exit_code = ?, stdout = ?, stderr = ?, error = ?, finished_at = ?
		WHERE id = ?
	`, result.Status, result.ExitCode, result.Stdout, result.Stderr, result.Error, nowString(), strings.TrimSpace(id))
	return err
}

func scanTestRuns(rows *sql.Rows) ([]checks.TestRun, error) {
	var items []checks.TestRun
	for rows.Next() {
		var item checks.TestRun
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.TaskID,
			&item.WorkflowRunID,
			&item.Command,
			&item.WorkingDir,
			&item.Reason,
			&item.Status,
			&item.ExitCode,
			&item.Stdout,
			&item.Stderr,
			&item.Error,
			&item.StartedAt,
			&item.FinishedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateReviewRun(ctx context.Context, item reviews.ReviewRun) (reviews.ReviewRun, error) {
	item.ID = newID("review")
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	item.TaskID = strings.TrimSpace(item.TaskID)
	item.WorkflowRunID = strings.TrimSpace(item.WorkflowRunID)
	item.Status = strings.TrimSpace(item.Status)
	item.Summary = strings.TrimSpace(item.Summary)
	item.RecommendedNextStep = strings.TrimSpace(item.RecommendedNextStep)
	item.ReturnTo = strings.TrimSpace(item.ReturnTo)
	item.BlockingReason = strings.TrimSpace(item.BlockingReason)
	item.Error = strings.TrimSpace(item.Error)
	item.CreatedAt = nowString()
	if item.Status == "" {
		item.Status = reviews.StatusPending
	}
	if item.Status == reviews.StatusRunning && item.StartedAt == "" {
		item.StartedAt = item.CreatedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO review_runs
			(id, project_id, task_id, workflow_run_id, status, summary, findings_json,
			 required_changes_json, recommended_next_step, return_to, iteration, blocking_reason,
			 error, started_at, finished_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		item.ID,
		item.ProjectID,
		item.TaskID,
		item.WorkflowRunID,
		item.Status,
		item.Summary,
		reviews.FindingsToJSON(item.Findings),
		reviews.RequiredChangesToJSON(item.RequiredChanges),
		item.RecommendedNextStep,
		item.ReturnTo,
		item.Iteration,
		item.BlockingReason,
		item.Error,
		item.StartedAt,
		item.FinishedAt,
		item.CreatedAt,
	)
	if err != nil {
		return reviews.ReviewRun{}, err
	}
	return item, nil
}

func (s *Store) FinishReviewRun(ctx context.Context, id string, parsed reviews.ParsedReview, errText string) error {
	status := parsed.Status
	if strings.TrimSpace(errText) != "" {
		status = reviews.StatusFailed
	}
	if status == "" {
		status = reviews.StatusFailed
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE review_runs
		SET status = ?, summary = ?, findings_json = ?, required_changes_json = ?,
			recommended_next_step = ?, return_to = ?, blocking_reason = ?, error = ?, finished_at = ?
		WHERE id = ?
	`,
		status,
		parsed.Summary,
		reviews.FindingsToJSON(parsed.Findings),
		reviews.RequiredChangesToJSON(parsed.RequiredChanges),
		parsed.RecommendedNextStep,
		parsed.ReturnTo,
		parsed.BlockingReason,
		strings.TrimSpace(errText),
		nowString(),
		strings.TrimSpace(id),
	)
	return err
}

func (s *Store) ListReviewRuns(ctx context.Context, projectID string, workflowRunID string, limit int) ([]reviews.ReviewRun, error) {
	if limit <= 0 {
		limit = 10
	}
	args := []any{projectID}
	sqlQuery := `
		SELECT id, project_id, task_id, workflow_run_id, status, summary, findings_json,
			required_changes_json, recommended_next_step, return_to, iteration, blocking_reason,
			error, started_at, finished_at, created_at
		FROM review_runs
		WHERE project_id = ?
	`
	if strings.TrimSpace(workflowRunID) != "" {
		sqlQuery += ` AND workflow_run_id = ?`
		args = append(args, workflowRunID)
	}
	sqlQuery += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReviewRuns(rows)
}

func scanReviewRuns(rows *sql.Rows) ([]reviews.ReviewRun, error) {
	var items []reviews.ReviewRun
	for rows.Next() {
		var item reviews.ReviewRun
		var findingsJSON string
		var requiredChangesJSON string
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.TaskID,
			&item.WorkflowRunID,
			&item.Status,
			&item.Summary,
			&findingsJSON,
			&requiredChangesJSON,
			&item.RecommendedNextStep,
			&item.ReturnTo,
			&item.Iteration,
			&item.BlockingReason,
			&item.Error,
			&item.StartedAt,
			&item.FinishedAt,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		item.Findings = reviews.FindingsFromJSON(findingsJSON)
		item.RequiredChanges = reviews.RequiredChangesFromJSON(requiredChangesJSON)
		items = append(items, item)
	}
	return items, rows.Err()
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

func marshalJSON(value any, fallback string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
