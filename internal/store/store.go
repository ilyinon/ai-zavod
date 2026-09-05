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

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/artifacts"
	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/projectmemory"
	"zavod_ai/internal/reviews"
	"zavod_ai/internal/taskspec"
	"zavod_ai/internal/webresearch"
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
		`CREATE TABLE IF NOT EXISTS workflow_plans (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_run_id TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'running',
			current_step_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS workflow_plan_steps (
			id TEXT PRIMARY KEY,
			plan_id TEXT NOT NULL,
			step_key TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued',
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(plan_id) REFERENCES workflow_plans(id) ON DELETE CASCADE
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
		`CREATE TABLE IF NOT EXISTS task_specs (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL UNIQUE,
			workflow_run_id TEXT NOT NULL DEFAULT '',
			user_request TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			goal TEXT NOT NULL DEFAULT '',
			requirements_json TEXT NOT NULL DEFAULT '[]',
			acceptance_criteria_json TEXT NOT NULL DEFAULT '[]',
			decisions_json TEXT NOT NULL DEFAULT '[]',
			open_questions_json TEXT NOT NULL DEFAULT '[]',
			accepted_answers_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'draft',
			source TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS project_memory (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL UNIQUE,
			architecture TEXT NOT NULL DEFAULT '',
			stack TEXT NOT NULL DEFAULT '',
			runtime TEXT NOT NULL DEFAULT '',
			project_type TEXT NOT NULL DEFAULT '',
			build_commands_json TEXT NOT NULL DEFAULT '[]',
			test_commands_json TEXT NOT NULL DEFAULT '[]',
			style_guide_json TEXT NOT NULL DEFAULT '[]',
			decisions_json TEXT NOT NULL DEFAULT '[]',
			environment_json TEXT NOT NULL DEFAULT '[]',
			updated_from_task_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS web_sources (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			workflow_run_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			query TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL,
			snippet TEXT NOT NULL DEFAULT '',
			content_excerpt TEXT NOT NULL DEFAULT '',
			source_type TEXT NOT NULL DEFAULT '',
			trust_level TEXT NOT NULL DEFAULT '',
			fetched_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS agent_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			default_model_id TEXT NOT NULL DEFAULT '',
			default_lifecycle_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tool_profiles (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			kind TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			allowed_commands_json TEXT NOT NULL DEFAULT '[]',
			blocked_commands_json TEXT NOT NULL DEFAULT '[]',
			requires_scope INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS agent_profiles (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			name TEXT NOT NULL,
			role_key TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			avatar_path TEXT NOT NULL DEFAULT '',
			soul_path TEXT NOT NULL DEFAULT '',
			model_id TEXT NOT NULL DEFAULT '',
			tool_profile_id TEXT NOT NULL DEFAULT '',
			skills_json TEXT NOT NULL DEFAULT '["pony-tail"]',
			capabilities_json TEXT NOT NULL DEFAULT '[]',
			allowed_tools_json TEXT NOT NULL DEFAULT '[]',
			read_paths_json TEXT NOT NULL DEFAULT '[]',
			write_paths_json TEXT NOT NULL DEFAULT '[]',
			handoff_rules_json TEXT NOT NULL DEFAULT '[]',
			temperature REAL NOT NULL DEFAULT 0.1,
			context_budget INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(group_id) REFERENCES agent_groups(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS lifecycle_definitions (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			name TEXT NOT NULL,
			kind TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			max_total_iterations INTEGER NOT NULL DEFAULT 16,
			max_repair_iterations INTEGER NOT NULL DEFAULT 2,
			same_error_limit INTEGER NOT NULL DEFAULT 2,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(group_id) REFERENCES agent_groups(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS lifecycle_steps (
			id TEXT PRIMARY KEY,
			lifecycle_id TEXT NOT NULL,
			step_key TEXT NOT NULL,
			title TEXT NOT NULL,
			agent_profile_id TEXT NOT NULL DEFAULT '',
			mode TEXT NOT NULL DEFAULT 'llm',
			required INTEGER NOT NULL DEFAULT 1,
			can_retry INTEGER NOT NULL DEFAULT 0,
			max_retries INTEGER NOT NULL DEFAULT 0,
			on_success_step_key TEXT NOT NULL DEFAULT '',
			on_failure_step_key TEXT NOT NULL DEFAULT '',
			output_schema TEXT NOT NULL DEFAULT '',
			visible_to_user INTEGER NOT NULL DEFAULT 1,
			sort_order INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY(lifecycle_id) REFERENCES lifecycle_definitions(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS project_group_bindings (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL UNIQUE,
			group_id TEXT NOT NULL,
			lifecycle_id TEXT NOT NULL DEFAULT '',
			is_default INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY(group_id) REFERENCES agent_groups(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_task ON messages(task_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_runs_task ON workflow_runs(task_id, started_at);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_steps_run ON workflow_steps(workflow_run_id, started_at);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_plans_run ON workflow_plans(workflow_run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_plan_steps_plan ON workflow_plan_steps(plan_id, sort_order);`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_project ON artifacts(project_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_proposed_changes_project ON proposed_changes(project_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_proposed_changes_workflow ON proposed_changes(workflow_run_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_test_runs_workflow ON test_runs(workflow_run_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_review_runs_workflow ON review_runs(workflow_run_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_task_blueprints_workflow ON task_blueprints(workflow_run_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_task_specs_project ON task_specs(project_id, updated_at);`,
		`CREATE INDEX IF NOT EXISTS idx_project_memory_project ON project_memory(project_id);`,
		`CREATE INDEX IF NOT EXISTS idx_web_sources_workflow ON web_sources(workflow_run_id, created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_profiles_group ON agent_profiles(group_id, sort_order);`,
		`CREATE INDEX IF NOT EXISTS idx_lifecycle_definitions_group ON lifecycle_definitions(group_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_lifecycle_steps_lifecycle ON lifecycle_steps(lifecycle_id, sort_order);`,
		`CREATE INDEX IF NOT EXISTS idx_project_group_bindings_project ON project_group_bindings(project_id);`,
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
		{name: "capabilities_json", definition: "TEXT NOT NULL DEFAULT '[]'"},
		{name: "allowed_tools_json", definition: "TEXT NOT NULL DEFAULT '[]'"},
		{name: "skills_json", definition: "TEXT NOT NULL DEFAULT '[\"pony-tail\"]'"},
		{name: "read_paths_json", definition: "TEXT NOT NULL DEFAULT '[]'"},
		{name: "write_paths_json", definition: "TEXT NOT NULL DEFAULT '[]'"},
		{name: "handoff_rules_json", definition: "TEXT NOT NULL DEFAULT '[]'"},
	} {
		if err := s.ensureColumn(ctx, "agent_profiles", column.name, column.definition); err != nil {
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

func (s *Store) UpsertTaskSpec(ctx context.Context, item taskspec.Spec) (taskspec.Spec, error) {
	now := nowString()
	item.ID = strings.TrimSpace(item.ID)
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	item.TaskID = strings.TrimSpace(item.TaskID)
	item.WorkflowRunID = strings.TrimSpace(item.WorkflowRunID)
	item.UserRequest = strings.TrimSpace(item.UserRequest)
	item.Summary = strings.TrimSpace(item.Summary)
	item.Goal = strings.TrimSpace(item.Goal)
	item.Status = strings.TrimSpace(item.Status)
	item.Source = strings.TrimSpace(item.Source)
	item.Requirements = cleanStringList(item.Requirements)
	item.AcceptanceCriteria = cleanStringList(item.AcceptanceCriteria)
	item.Decisions = cleanStringList(item.Decisions)
	item.OpenQuestions = cleanStringList(item.OpenQuestions)
	item.AcceptedAnswers = cleanAcceptedAnswers(item.AcceptedAnswers)
	if item.ProjectID == "" || item.TaskID == "" {
		return taskspec.Spec{}, fmt.Errorf("project_id и task_id обязательны для task spec")
	}
	if item.ID == "" {
		item.ID = newID("spec")
		item.CreatedAt = now
	}
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	if item.Status == "" {
		item.Status = taskspec.StatusDraft
	}
	item.UpdatedAt = now

	requirementsJSON := marshalJSON(item.Requirements, "[]")
	acceptanceJSON := marshalJSON(item.AcceptanceCriteria, "[]")
	decisionsJSON := marshalJSON(item.Decisions, "[]")
	openQuestionsJSON := marshalJSON(item.OpenQuestions, "[]")
	acceptedAnswersJSON := marshalJSON(item.AcceptedAnswers, "[]")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO task_specs
			(id, project_id, task_id, workflow_run_id, user_request, summary, goal,
			 requirements_json, acceptance_criteria_json, decisions_json, open_questions_json,
			 accepted_answers_json, status, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			workflow_run_id = excluded.workflow_run_id,
			user_request = excluded.user_request,
			summary = excluded.summary,
			goal = excluded.goal,
			requirements_json = excluded.requirements_json,
			acceptance_criteria_json = excluded.acceptance_criteria_json,
			decisions_json = excluded.decisions_json,
			open_questions_json = excluded.open_questions_json,
			accepted_answers_json = excluded.accepted_answers_json,
			status = excluded.status,
			source = excluded.source,
			updated_at = excluded.updated_at
	`, item.ID, item.ProjectID, item.TaskID, item.WorkflowRunID, item.UserRequest, item.Summary, item.Goal, requirementsJSON, acceptanceJSON, decisionsJSON, openQuestionsJSON, acceptedAnswersJSON, item.Status, item.Source, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return taskspec.Spec{}, err
	}
	return s.LatestTaskSpecByTask(ctx, item.TaskID)
}

func (s *Store) LatestTaskSpecByTask(ctx context.Context, taskID string) (taskspec.Spec, error) {
	return s.readTaskSpec(ctx, `WHERE task_id = ?`, strings.TrimSpace(taskID))
}

func (s *Store) LatestTaskSpecByProject(ctx context.Context, projectID string) (taskspec.Spec, error) {
	return s.readTaskSpec(ctx, `WHERE project_id = ? ORDER BY updated_at DESC LIMIT 1`, strings.TrimSpace(projectID))
}

func (s *Store) readTaskSpec(ctx context.Context, clause string, arg string) (taskspec.Spec, error) {
	var item taskspec.Spec
	var requirementsJSON, acceptanceJSON, decisionsJSON, openQuestionsJSON, acceptedAnswersJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, user_request, summary, goal,
			requirements_json, acceptance_criteria_json, decisions_json, open_questions_json,
			accepted_answers_json, status, source, created_at, updated_at
		FROM task_specs
		`+clause, arg).Scan(
		&item.ID,
		&item.ProjectID,
		&item.TaskID,
		&item.WorkflowRunID,
		&item.UserRequest,
		&item.Summary,
		&item.Goal,
		&requirementsJSON,
		&acceptanceJSON,
		&decisionsJSON,
		&openQuestionsJSON,
		&acceptedAnswersJSON,
		&item.Status,
		&item.Source,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return taskspec.Spec{}, err
	}
	item.Requirements = decodeStringList(requirementsJSON)
	item.AcceptanceCriteria = decodeStringList(acceptanceJSON)
	item.Decisions = decodeStringList(decisionsJSON)
	item.OpenQuestions = decodeStringList(openQuestionsJSON)
	item.AcceptedAnswers = decodeAcceptedAnswers(acceptedAnswersJSON)
	return item, nil
}

func (s *Store) UpsertProjectMemory(ctx context.Context, item projectmemory.Memory) (projectmemory.Memory, error) {
	now := nowString()
	item.ID = strings.TrimSpace(item.ID)
	item.ProjectID = strings.TrimSpace(item.ProjectID)
	item.Architecture = strings.TrimSpace(item.Architecture)
	item.Stack = strings.TrimSpace(item.Stack)
	item.Runtime = strings.TrimSpace(item.Runtime)
	item.ProjectType = strings.TrimSpace(item.ProjectType)
	item.UpdatedFromTaskID = strings.TrimSpace(item.UpdatedFromTaskID)
	item.BuildCommands = cleanStringList(item.BuildCommands)
	item.TestCommands = cleanStringList(item.TestCommands)
	item.StyleGuide = cleanStringList(item.StyleGuide)
	item.Decisions = cleanStringList(item.Decisions)
	item.Environment = cleanStringList(item.Environment)
	if item.ProjectID == "" {
		return projectmemory.Memory{}, fmt.Errorf("project_id обязателен для project memory")
	}
	if item.ID == "" {
		item.ID = newID("mem")
		item.CreatedAt = now
	}
	if item.CreatedAt == "" {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	buildCommandsJSON := marshalJSON(item.BuildCommands, "[]")
	testCommandsJSON := marshalJSON(item.TestCommands, "[]")
	styleGuideJSON := marshalJSON(item.StyleGuide, "[]")
	decisionsJSON := marshalJSON(item.Decisions, "[]")
	environmentJSON := marshalJSON(item.Environment, "[]")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO project_memory
			(id, project_id, architecture, stack, runtime, project_type,
			 build_commands_json, test_commands_json, style_guide_json, decisions_json,
			 environment_json, updated_from_task_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			architecture = excluded.architecture,
			stack = excluded.stack,
			runtime = excluded.runtime,
			project_type = excluded.project_type,
			build_commands_json = excluded.build_commands_json,
			test_commands_json = excluded.test_commands_json,
			style_guide_json = excluded.style_guide_json,
			decisions_json = excluded.decisions_json,
			environment_json = excluded.environment_json,
			updated_from_task_id = excluded.updated_from_task_id,
			updated_at = excluded.updated_at
	`, item.ID, item.ProjectID, item.Architecture, item.Stack, item.Runtime, item.ProjectType, buildCommandsJSON, testCommandsJSON, styleGuideJSON, decisionsJSON, environmentJSON, item.UpdatedFromTaskID, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return projectmemory.Memory{}, err
	}
	return s.ProjectMemory(ctx, item.ProjectID)
}

func (s *Store) ProjectMemory(ctx context.Context, projectID string) (projectmemory.Memory, error) {
	var item projectmemory.Memory
	var buildCommandsJSON, testCommandsJSON, styleGuideJSON, decisionsJSON, environmentJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, architecture, stack, runtime, project_type,
			build_commands_json, test_commands_json, style_guide_json, decisions_json,
			environment_json, updated_from_task_id, created_at, updated_at
		FROM project_memory
		WHERE project_id = ?
	`, strings.TrimSpace(projectID)).Scan(
		&item.ID,
		&item.ProjectID,
		&item.Architecture,
		&item.Stack,
		&item.Runtime,
		&item.ProjectType,
		&buildCommandsJSON,
		&testCommandsJSON,
		&styleGuideJSON,
		&decisionsJSON,
		&environmentJSON,
		&item.UpdatedFromTaskID,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return projectmemory.Memory{}, nil
	}
	if err != nil {
		return projectmemory.Memory{}, err
	}
	item.BuildCommands = decodeStringList(buildCommandsJSON)
	item.TestCommands = decodeStringList(testCommandsJSON)
	item.StyleGuide = decodeStringList(styleGuideJSON)
	item.Decisions = decodeStringList(decisionsJSON)
	item.Environment = decodeStringList(environmentJSON)
	return item, nil
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

func (s *Store) CreateWorkflowPlan(ctx context.Context, plan workflow.Plan, steps []workflow.PlanStep) (workflow.Plan, []workflow.PlanStep, error) {
	plan.ID = newID("plan")
	plan.ProjectID = strings.TrimSpace(plan.ProjectID)
	plan.TaskID = strings.TrimSpace(plan.TaskID)
	plan.WorkflowRunID = strings.TrimSpace(plan.WorkflowRunID)
	plan.Title = strings.TrimSpace(plan.Title)
	plan.Status = strings.TrimSpace(plan.Status)
	plan.CreatedAt = nowString()
	plan.UpdatedAt = plan.CreatedAt
	if plan.Status == "" {
		plan.Status = workflow.StatusRunning
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Plan{}, nil, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO workflow_plans
			(id, project_id, task_id, workflow_run_id, title, status, current_step_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, plan.ID, plan.ProjectID, plan.TaskID, plan.WorkflowRunID, plan.Title, plan.Status, plan.CurrentStepID, plan.CreatedAt, plan.UpdatedAt)
	if err != nil {
		return workflow.Plan{}, nil, err
	}

	savedSteps := make([]workflow.PlanStep, 0, len(steps))
	for index, step := range steps {
		step.ID = newID("planstep")
		step.PlanID = plan.ID
		step.StepKey = strings.TrimSpace(step.StepKey)
		step.Title = strings.TrimSpace(step.Title)
		step.Description = strings.TrimSpace(step.Description)
		step.AgentID = strings.TrimSpace(step.AgentID)
		step.Status = strings.TrimSpace(step.Status)
		step.Error = strings.TrimSpace(step.Error)
		step.SortOrder = index
		if step.StepKey == "" {
			step.StepKey = fmt.Sprintf("step_%d", index+1)
		}
		if step.Title == "" {
			step.Title = step.StepKey
		}
		if step.Status == "" {
			step.Status = workflow.StepStatusQueued
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO workflow_plan_steps
				(id, plan_id, step_key, title, description, agent_id, status, started_at, finished_at, error, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, step.ID, step.PlanID, step.StepKey, step.Title, step.Description, step.AgentID, step.Status, step.StartedAt, step.FinishedAt, step.Error, step.SortOrder)
		if err != nil {
			return workflow.Plan{}, nil, err
		}
		savedSteps = append(savedSteps, step)
	}
	if err := tx.Commit(); err != nil {
		return workflow.Plan{}, nil, err
	}
	return plan, savedSteps, nil
}

func (s *Store) LatestWorkflowPlan(ctx context.Context, workflowRunID string) (*workflow.Plan, []workflow.PlanStep, error) {
	var plan workflow.Plan
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, title, status, current_step_id, created_at, updated_at
		FROM workflow_plans
		WHERE workflow_run_id = ?
	`, workflowRunID).Scan(
		&plan.ID,
		&plan.ProjectID,
		&plan.TaskID,
		&plan.WorkflowRunID,
		&plan.Title,
		&plan.Status,
		&plan.CurrentStepID,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	steps, err := s.ListWorkflowPlanSteps(ctx, plan.ID)
	if err != nil {
		return nil, nil, err
	}
	return &plan, steps, nil
}

func (s *Store) ListWorkflowPlanSteps(ctx context.Context, planID string) ([]workflow.PlanStep, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, plan_id, step_key, title, description, agent_id, status, started_at, finished_at, error, sort_order
		FROM workflow_plan_steps
		WHERE plan_id = ?
		ORDER BY sort_order ASC
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []workflow.PlanStep
	for rows.Next() {
		var step workflow.PlanStep
		if err := rows.Scan(
			&step.ID,
			&step.PlanID,
			&step.StepKey,
			&step.Title,
			&step.Description,
			&step.AgentID,
			&step.Status,
			&step.StartedAt,
			&step.FinishedAt,
			&step.Error,
			&step.SortOrder,
		); err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (s *Store) StartWorkflowPlanStep(ctx context.Context, workflowRunID string, stepKey string, fallbackAgentID string) error {
	stepKey = strings.TrimSpace(stepKey)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	plan, steps, err := workflowPlanForRunTx(ctx, tx, workflowRunID)
	if err != nil || plan == nil || len(steps) == 0 {
		return err
	}
	step := choosePlanStep(steps, stepKey, fallbackAgentID)
	if step.ID == "" {
		return nil
	}
	now := nowString()
	_, err = tx.ExecContext(ctx, `
		UPDATE workflow_plan_steps
		SET status = ?, finished_at = CASE WHEN finished_at = '' THEN ? ELSE finished_at END
		WHERE plan_id = ? AND status = ?
	`, workflow.StepStatusDone, now, plan.ID, workflow.StepStatusRunning)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE workflow_plan_steps
		SET status = ?,
			agent_id = CASE WHEN ? <> '' THEN ? ELSE agent_id END,
			started_at = CASE WHEN started_at = '' THEN ? ELSE started_at END,
			error = ''
		WHERE id = ?
	`, workflow.StepStatusRunning, fallbackAgentID, fallbackAgentID, now, step.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE workflow_plans
		SET status = ?, current_step_id = ?, updated_at = ?
		WHERE id = ?
	`, workflow.StatusRunning, step.ID, now, plan.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FinishWorkflowPlanStep(ctx context.Context, workflowRunID string, stepKey string, fallbackAgentID string, status string, errText string) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = workflow.StepStatusDone
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	plan, steps, err := workflowPlanForRunTx(ctx, tx, workflowRunID)
	if err != nil || plan == nil || len(steps) == 0 {
		return err
	}
	step := choosePlanStep(steps, stepKey, fallbackAgentID)
	if step.ID == "" {
		return nil
	}
	now := nowString()
	_, err = tx.ExecContext(ctx, `
		UPDATE workflow_plan_steps
		SET status = ?, finished_at = ?, error = ?
		WHERE id = ?
	`, status, now, strings.TrimSpace(errText), step.ID)
	if err != nil {
		return err
	}
	planStatus := workflow.StatusRunning
	if status == workflow.StepStatusFailed {
		planStatus = workflow.StatusFailed
	} else if status == workflow.StatusBlocked || status == workflow.StepStatusFailed {
		planStatus = workflow.StatusBlocked
	} else if allPlanStepsDone(steps, step.ID, status) {
		planStatus = workflow.StatusDone
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE workflow_plans
		SET status = ?, current_step_id = ?, updated_at = ?
		WHERE id = ?
	`, planStatus, step.ID, now, plan.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FinishWorkflowPlan(ctx context.Context, workflowRunID string, status string, errText string) error {
	plan, _, err := s.LatestWorkflowPlan(ctx, workflowRunID)
	if err != nil || plan == nil {
		return err
	}
	now := nowString()
	if status == workflow.StatusDone {
		_, err = s.db.ExecContext(ctx, `
			UPDATE workflow_plan_steps
			SET status = ?, finished_at = CASE WHEN finished_at = '' THEN ? ELSE finished_at END
			WHERE plan_id = ? AND status IN (?, ?)
		`, workflow.StepStatusDone, now, plan.ID, workflow.StepStatusQueued, workflow.StepStatusRunning)
		if err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE workflow_plans
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, strings.TrimSpace(status), now, plan.ID)
	_ = errText
	return err
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

func (s *Store) CreateWebSource(ctx context.Context, source webresearch.Source) (webresearch.Source, error) {
	source.ID = newID("websrc")
	source.ProjectID = strings.TrimSpace(source.ProjectID)
	source.TaskID = strings.TrimSpace(source.TaskID)
	source.WorkflowRunID = strings.TrimSpace(source.WorkflowRunID)
	source.AgentID = strings.TrimSpace(source.AgentID)
	source.Query = strings.TrimSpace(source.Query)
	source.Title = strings.TrimSpace(source.Title)
	source.URL = strings.TrimSpace(source.URL)
	source.Snippet = strings.TrimSpace(source.Snippet)
	source.ContentExcerpt = strings.TrimSpace(source.ContentExcerpt)
	source.SourceType = strings.TrimSpace(source.SourceType)
	source.TrustLevel = strings.TrimSpace(source.TrustLevel)
	source.FetchedAt = strings.TrimSpace(source.FetchedAt)
	source.CreatedAt = nowString()
	if source.AgentID == "" {
		source.AgentID = "manager"
	}
	if source.SourceType == "" {
		source.SourceType = "web"
	}
	if source.Title == "" {
		source.Title = source.URL
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO web_sources
			(id, project_id, task_id, workflow_run_id, agent_id, query, title, url, snippet,
			 content_excerpt, source_type, trust_level, fetched_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		source.ID,
		source.ProjectID,
		source.TaskID,
		source.WorkflowRunID,
		source.AgentID,
		source.Query,
		source.Title,
		source.URL,
		source.Snippet,
		source.ContentExcerpt,
		source.SourceType,
		source.TrustLevel,
		source.FetchedAt,
		source.CreatedAt,
	)
	if err != nil {
		return webresearch.Source{}, err
	}
	return source, nil
}

func (s *Store) ListWebSources(ctx context.Context, projectID string, workflowRunID string, limit int) ([]webresearch.Source, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, agent_id, query, title, url, snippet,
		       content_excerpt, source_type, trust_level, fetched_at, created_at
		FROM web_sources
		WHERE project_id = ? AND (? = '' OR workflow_run_id = ?)
		ORDER BY created_at DESC
		LIMIT ?
	`, projectID, workflowRunID, workflowRunID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []webresearch.Source
	for rows.Next() {
		var item webresearch.Source
		if err := rows.Scan(
			&item.ID,
			&item.ProjectID,
			&item.TaskID,
			&item.WorkflowRunID,
			&item.AgentID,
			&item.Query,
			&item.Title,
			&item.URL,
			&item.Snippet,
			&item.ContentExcerpt,
			&item.SourceType,
			&item.TrustLevel,
			&item.FetchedAt,
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

func (s *Store) MarkProposedChangeRolledBack(ctx context.Context, id string, diffText string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE proposed_changes
		SET status = ?, error = '', diff_text = ?
		WHERE id = ?
	`, changes.StatusRolledBack, diffText, id)
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

func (s *Store) EnsureDefaultAgentGroups(ctx context.Context, defaultModelID string) error {
	defaultModelID = strings.TrimSpace(defaultModelID)
	if defaultModelID == "" {
		defaultModelID = "qwen-remote"
	}
	now := nowString()

	toolProfiles := []agentgroups.ToolProfile{
		{ID: "tool_go_dev", Name: "Go development", Kind: "go_dev", Description: "Go-проекты: gofmt, go test, go vet; go mod/make/wails только после подтверждения.", AllowedCommands: `["gofmt","go test ./...","go vet ./..."]`},
		{ID: "tool_python_dev", Name: "Python development", Kind: "python_dev", Description: "Python-проекты только через project-local virtualenv; venv и requirements готовит backend.", AllowedCommands: `[".venv/bin/python <script.py>",".venv/bin/python -m pytest",".venv/bin/python -m py_compile"]`},
		{ID: "tool_research", Name: "Research", Kind: "research", Description: "Поиск, чтение источников, проверка свежести, сравнение и research notes.", AllowedCommands: `[]`},
		{ID: "tool_ctf_web", Name: "CTF web", Kind: "ctf_web", Description: "Web CTF в рамках scope: локальные solver scripts auto, HTTP/DNS только после подтверждения scope.", AllowedCommands: `[".venv/bin/python <solver.py>","file","strings","curl confirm","dig confirm","whois confirm"]`, RequiresScope: true},
		{ID: "tool_ctf_lfi", Name: "CTF LFI", Kind: "ctf_lfi", Description: "LFI/path traversal CTF: локальные solver scripts auto, HTTP-проверки только в scope.", AllowedCommands: `[".venv/bin/python <solver.py>","file","strings","curl confirm"]`, RequiresScope: true},
		{ID: "tool_ctf_rce", Name: "CTF RCE", Kind: "ctf_rce", Description: "RCE/command injection CTF: локальный анализ auto, активные HTTP-проверки только в scope.", AllowedCommands: `[".venv/bin/python <solver.py>","file","strings","curl confirm"]`, RequiresScope: true},
		{ID: "tool_ctf_sqli", Name: "CTF SQLi", Kind: "ctf_sqli", Description: "SQLi CTF: локальные SQL/solver scripts auto, curl/sqlmap только при явном scope.", AllowedCommands: `[".venv/bin/python <solver.py>","file","strings","curl confirm","sqlmap confirm"]`, RequiresScope: true},
		{ID: "tool_ctf_pwn", Name: "CTF pwn", Kind: "ctf_pwn", Description: "Локальный binary exploitation lab: static triage auto, pwntools через .venv, debugger только с подтверждением.", AllowedCommands: `["file","strings","checksec","readelf","objdump","nm",".venv/bin/python <pwntools solver.py>","gdb confirm","ROPgadget confirm","one_gadget confirm"]`},
		{ID: "tool_ctf_crypto", Name: "CTF crypto", Kind: "ctf_crypto", Description: "Локальные crypto solvers через project virtualenv; Sage только после подтверждения.", AllowedCommands: `[".venv/bin/python <solver.py>","file","strings","sage confirm"]`},
		{ID: "tool_ctf_reverse", Name: "CTF reverse", Kind: "ctf_reverse", Description: "Reverse engineering локальных артефактов: static triage auto, interactive tools confirm.", AllowedCommands: `["file","strings","readelf","objdump","nm",".venv/bin/python <helper.py>","radare2 confirm","r2 confirm","ghidra confirm"]`},
		{ID: "tool_ctf_forensics", Name: "CTF forensics", Kind: "ctf_forensics", Description: "Форензика файлов и дампов: metadata/binwalk без extract auto; извлечение и тяжелые tools confirm.", AllowedCommands: `["file","strings","exiftool","binwalk","xxd",".venv/bin/python <helper.py>","binwalk -e confirm","foremost confirm","tshark confirm"]`},
		{ID: "tool_ctf_validator", Name: "CTF validator", Kind: "ctf_validator", Description: "Проверка flag, solver scripts и writeup/evidence consistency без активных сетевых действий.", AllowedCommands: `[".venv/bin/python <validator.py>","file","strings"]`},
	}
	for _, profile := range toolProfiles {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO tool_profiles
				(id, name, kind, description, allowed_commands_json, blocked_commands_json, requires_scope, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				kind = excluded.kind,
				description = excluded.description,
				allowed_commands_json = excluded.allowed_commands_json,
				blocked_commands_json = excluded.blocked_commands_json,
				requires_scope = excluded.requires_scope,
				updated_at = excluded.updated_at
		`, profile.ID, profile.Name, profile.Kind, profile.Description, profile.AllowedCommands, profile.BlockedCommands, boolInt(profile.RequiresScope), now, now); err != nil {
			return err
		}
	}

	if err := s.ensureSeedGroup(ctx, agentgroups.Group{
		ID:             "group_dev_squad",
		Name:           "Dev Squad",
		Slug:           "dev-squad",
		Kind:           agentgroups.GroupKindDev,
		Description:    "Команда для разработки Python/Go задач: требования, архитектура, код, проверки и ревью.",
		DefaultModelID: defaultModelID,
		Status:         agentgroups.StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, []agentgroups.Profile{
		{ID: "agent_dev_lumen", Name: "Люмен", RoleKey: "manager", Description: "Принимает задачу, роутит intent, держит task spec и итог.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_dev_product", Name: "Продакт", RoleKey: "product", Description: "Формулирует требования, сценарии и критерии готовности.", Temperature: 0.2, ContextBudget: 9000, Enabled: true},
		{ID: "agent_dev_architect", Name: "Архитектор", RoleKey: "architect", Description: "Проектирует структуру решения, blueprint, риски и порядок внедрения.", ToolProfileID: "tool_go_dev", Temperature: 0.15, ContextBudget: 10000, Enabled: true},
		{ID: "agent_dev_developer", Name: "Разработчик", RoleKey: "developer", Description: "Готовит structured changes и кодовые изменения.", ToolProfileID: "tool_go_dev", Temperature: 0.12, ContextBudget: 14000, Enabled: true},
		{ID: "agent_dev_tester", Name: "Тестировщик", RoleKey: "tester", Description: "Подбирает и запускает минимальные проверки по стеку проекта.", ToolProfileID: "tool_python_dev", Temperature: 0.1, ContextBudget: 8000, Enabled: true},
		{ID: "agent_dev_reviewer", Name: "Ревьюер", RoleKey: "reviewer", Description: "Обязательный gate качества перед итогом.", Temperature: 0.08, ContextBudget: 10000, Enabled: true},
		{ID: "agent_dev_docs", Name: "Докер", RoleKey: "docs", Description: "Обновляет README, инструкции сборки и разработческий контекст.", Temperature: 0.15, ContextBudget: 8000, Enabled: false},
		{ID: "agent_dev_releaser", Name: "Релизер", RoleKey: "release", Description: "Готовит changelog, сборку и release notes.", Temperature: 0.1, ContextBudget: 8000, Enabled: false},
	}, agentgroups.LifecycleDefinition{
		ID:                  "lifecycle_dev_default",
		Name:                "Dev Autopilot",
		Kind:                agentgroups.GroupKindDev,
		Description:         "Совместимый dev workflow: intake, requirements, blueprint, architecture, development, checks, review, final.",
		MaxTotalIterations:  16,
		MaxRepairIterations: 2,
		SameErrorLimit:      2,
		Status:              agentgroups.StatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, []agentgroups.LifecycleStep{
		{ID: "lstep_dev_intake", StepKey: "manager_intake", Title: "Постановка задачи", AgentProfileID: "agent_dev_lumen", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_dev_requirements", StepKey: "product_requirements", Title: "Требования", AgentProfileID: "agent_dev_product", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_dev_blueprint", StepKey: "task_blueprint", Title: "Blueprint", AgentProfileID: "agent_dev_architect", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_dev_architecture", StepKey: "architect_plan", Title: "Архитектурный план", AgentProfileID: "agent_dev_architect", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_dev_implementation", StepKey: "developer_plan", Title: "Разработка", AgentProfileID: "agent_dev_developer", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 2, OnFailureStepKey: "developer_plan", VisibleToUser: true},
		{ID: "lstep_dev_checks", StepKey: "tester_commands", Title: "Проверка", AgentProfileID: "agent_dev_tester", Mode: "checks", Required: true, CanRetry: true, MaxRetries: 2, OnFailureStepKey: "developer_plan", OutputSchema: lifecycleReturnConfig("developer_plan"), VisibleToUser: true},
		{ID: "lstep_dev_review", StepKey: "review", Title: "Ревью", AgentProfileID: "agent_dev_reviewer", Mode: "review", Required: true, CanRetry: true, MaxRetries: 2, OnFailureStepKey: "developer_plan", OutputSchema: lifecycleReturnConfig("developer_plan"), VisibleToUser: true},
		{ID: "lstep_dev_final", StepKey: "manager_final", Title: "Итог", AgentProfileID: "agent_dev_lumen", Mode: "final", Required: true, VisibleToUser: true},
	}); err != nil {
		return err
	}

	if err := s.ensureSeedGroup(ctx, agentgroups.Group{
		ID:             "group_ctf_cell",
		Name:           "CTF Cell",
		Slug:           "ctf-cell",
		Kind:           agentgroups.GroupKindCTF,
		Description:    "Команда для CTF и легитимных lab-задач: triage, scope, категория, решение и writeup.",
		DefaultModelID: defaultModelID,
		Status:         agentgroups.StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, []agentgroups.Profile{
		{ID: "agent_ctf_lumen", Name: "Люмен", RoleKey: "manager", Description: "Принимает CTF-задачу, классифицирует категорию и собирает writeup.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_scout", Name: "Разведчик", RoleKey: "scout", Description: "Собирает вводные, артефакты, scope и первые наблюдения.", ToolProfileID: "tool_research", Temperature: 0.12, ContextBudget: 10000, Enabled: true},
		{ID: "agent_ctf_web", Name: "Web Exploiter", RoleKey: "ctf_web", Description: "Решает общие web challenge только в рамках scope.", ToolProfileID: "tool_ctf_web", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_lfi", Name: "LFI Hunter", RoleKey: "ctf_lfi", Description: "Решает LFI/path traversal challenge только в рамках scope.", ToolProfileID: "tool_ctf_lfi", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_rce", Name: "RCE Analyst", RoleKey: "ctf_rce", Description: "Решает RCE/command injection challenge только в рамках scope.", ToolProfileID: "tool_ctf_rce", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_sqli", Name: "SQLi Solver", RoleKey: "ctf_sqli", Description: "Решает SQL injection challenge только в рамках scope.", ToolProfileID: "tool_ctf_sqli", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_pwn", Name: "Pwner", RoleKey: "ctf_pwn", Description: "Разбирает локальные pwn/binary exploitation challenge.", ToolProfileID: "tool_ctf_pwn", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_crypto", Name: "Криптограф", RoleKey: "ctf_crypto", Description: "Строит solver для crypto challenge.", ToolProfileID: "tool_ctf_crypto", Temperature: 0.08, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_reverse", Name: "Реверсер", RoleKey: "ctf_reverse", Description: "Анализирует reverse engineering задачи и локальные бинарные артефакты.", ToolProfileID: "tool_ctf_reverse", Temperature: 0.08, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_forensics", Name: "Форензик", RoleKey: "ctf_forensics", Description: "Разбирает файлы, дампы, изображения и сетевые артефакты.", ToolProfileID: "tool_ctf_forensics", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_ctf_validator", Name: "Валидатор", RoleKey: "validator", Description: "Проверяет flag, воспроизводимость решения и writeup.", ToolProfileID: "tool_ctf_validator", Temperature: 0.05, ContextBudget: 9000, Enabled: true},
	}, agentgroups.LifecycleDefinition{
		ID:                  "lifecycle_ctf_default",
		Name:                "CTF Challenge",
		Kind:                agentgroups.GroupKindCTF,
		Description:         "CTF workflow: intake, scope, artifacts, triage, hypothesis, category solver, validation, writeup.",
		MaxTotalIterations:  18,
		MaxRepairIterations: 2,
		SameErrorLimit:      2,
		Status:              agentgroups.StatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, []agentgroups.LifecycleStep{
		{ID: "lstep_ctf_intake", StepKey: "intake", Title: "Постановка CTF", AgentProfileID: "agent_ctf_lumen", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_ctf_scope", StepKey: "scope_check", Title: "Scope", AgentProfileID: "agent_ctf_scout", Mode: "human_gate", Required: true, CanRetry: true, MaxRetries: 1, OutputSchema: lifecycleHumanGateConfig("Подтверди CTF/lab scope перед активными сетевыми действиями.", []string{"target", "authorization", "allowed actions"}), VisibleToUser: true},
		{ID: "lstep_ctf_artifacts", StepKey: "artifact_collection", Title: "Артефакты", AgentProfileID: "agent_ctf_scout", Mode: "tool", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_ctf_triage", StepKey: "triage", Title: "Категория", AgentProfileID: "agent_ctf_scout", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_ctf_hypothesis", StepKey: "hypothesis_board", Title: "Гипотезы", AgentProfileID: "agent_ctf_lumen", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_ctf_solver", StepKey: "category_solver", Title: "Решение", AgentProfileID: "agent_ctf_web", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 3, VisibleToUser: true},
		{ID: "lstep_ctf_validation", StepKey: "validation", Title: "Проверка flag", AgentProfileID: "agent_ctf_validator", Mode: "review", Required: true, CanRetry: true, MaxRetries: 2, OnFailureStepKey: "category_solver", OutputSchema: lifecycleReturnConfig("category_solver"), VisibleToUser: true},
		{ID: "lstep_ctf_writeup", StepKey: "writeup", Title: "Writeup", AgentProfileID: "agent_ctf_lumen", Mode: "final", Required: true, VisibleToUser: true},
	}); err != nil {
		return err
	}

	if err := s.ensureSeedGroup(ctx, agentgroups.Group{
		ID:             "group_research_squad",
		Name:           "Research Squad",
		Slug:           "research-squad",
		Kind:           agentgroups.GroupKindResearch,
		Description:    "Команда для поиска в интернете, проверки источников, сравнения, аналитики и research notes.",
		DefaultModelID: defaultModelID,
		Status:         agentgroups.StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, []agentgroups.Profile{
		{ID: "agent_research_lumen", Name: "Люмен", RoleKey: "manager", Description: "Понимает вопрос, держит рамку исследования и собирает итоговый ответ.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		{ID: "agent_research_researcher", Name: "Исследователь", RoleKey: "researcher", Description: "Формирует поисковые запросы, собирает источники и evidence.", ToolProfileID: "tool_research", Temperature: 0.12, ContextBudget: 12000, Enabled: true},
		{ID: "agent_research_source_reviewer", Name: "Проверяющая источники", RoleKey: "source_reviewer", Description: "Проверяет свежесть, доверие, прямые ссылки и противоречия.", ToolProfileID: "tool_research", Temperature: 0.05, ContextBudget: 9000, Enabled: true},
		{ID: "agent_research_analyst", Name: "Аналитик", RoleKey: "analyst", Description: "Сравнивает источники, отделяет факты от выводов и собирает аналитику.", ToolProfileID: "tool_research", Temperature: 0.12, ContextBudget: 11000, Enabled: true},
	}, agentgroups.LifecycleDefinition{
		ID:                  "lifecycle_research_default",
		Name:                "Research Workflow",
		Kind:                agentgroups.GroupKindResearch,
		Description:         "Research workflow: план поиска, сбор источников, проверка свежести, синтез, notes и итог.",
		MaxTotalIterations:  12,
		MaxRepairIterations: 1,
		SameErrorLimit:      2,
		Status:              agentgroups.StatusActive,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, []agentgroups.LifecycleStep{
		{ID: "lstep_research_search", StepKey: "web_research", Title: "Поиск", AgentProfileID: "agent_research_researcher", Mode: "tool", Required: true, CanRetry: true, MaxRetries: 2, VisibleToUser: true},
		{ID: "lstep_research_source_review", StepKey: "source_review", Title: "Источники", AgentProfileID: "agent_research_source_reviewer", Mode: "review", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_research_synthesis", StepKey: "research_synthesis", Title: "Аналитика", AgentProfileID: "agent_research_analyst", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_research_notes", StepKey: "research_notes", Title: "Research notes", AgentProfileID: "agent_research_researcher", Mode: "artifact", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
		{ID: "lstep_research_final", StepKey: "manager_final", Title: "Итог", AgentProfileID: "agent_research_lumen", Mode: "final", Required: true, VisibleToUser: true},
	}); err != nil {
		return err
	}

	return s.EnsureDefaultProjectGroupBindings(ctx)
}

func (s *Store) ListAgentGroups(ctx context.Context, includeArchived bool) ([]agentgroups.Group, error) {
	query := `
		SELECT g.id, g.name, g.slug, g.kind, g.description, g.default_model_id, g.default_lifecycle_id,
			g.status, g.created_at, g.updated_at, COUNT(p.id) AS agent_count
		FROM agent_groups g
		LEFT JOIN agent_profiles p ON p.group_id = g.id
	`
	args := []any{}
	if !includeArchived {
		query += ` WHERE g.status <> ?`
		args = append(args, agentgroups.StatusArchived)
	}
	query += ` GROUP BY g.id ORDER BY CASE g.kind WHEN 'dev' THEN 0 WHEN 'ctf' THEN 1 WHEN 'research' THEN 2 WHEN 'security' THEN 3 ELSE 4 END, g.name ASC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []agentgroups.Group
	for rows.Next() {
		var item agentgroups.Group
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Slug,
			&item.Kind,
			&item.Description,
			&item.DefaultModelID,
			&item.DefaultLifecycleID,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.AgentCount,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetAgentGroup(ctx context.Context, id string) (agentgroups.Group, error) {
	var item agentgroups.Group
	err := s.db.QueryRowContext(ctx, `
		SELECT g.id, g.name, g.slug, g.kind, g.description, g.default_model_id, g.default_lifecycle_id,
			g.status, g.created_at, g.updated_at, COUNT(p.id) AS agent_count
		FROM agent_groups g
		LEFT JOIN agent_profiles p ON p.group_id = g.id
		WHERE g.id = ?
		GROUP BY g.id
	`, strings.TrimSpace(id)).Scan(
		&item.ID,
		&item.Name,
		&item.Slug,
		&item.Kind,
		&item.Description,
		&item.DefaultModelID,
		&item.DefaultLifecycleID,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.AgentCount,
	)
	return item, err
}

func (s *Store) CreateAgentGroup(ctx context.Context, group agentgroups.Group) (agentgroups.Group, error) {
	now := nowString()
	group.ID = newID("group")
	group.Name = strings.TrimSpace(group.Name)
	group.Slug = uniqueSlug(group.Slug, group.Name, group.ID)
	group.Kind = normalizeGroupKind(group.Kind)
	group.Description = strings.TrimSpace(group.Description)
	group.DefaultModelID = strings.TrimSpace(group.DefaultModelID)
	group.Status = agentgroups.StatusActive
	group.CreatedAt = now
	group.UpdatedAt = now
	if group.Name == "" {
		return agentgroups.Group{}, fmt.Errorf("название группы пустое")
	}
	if group.DefaultLifecycleID == "" {
		group.DefaultLifecycleID = newID("lifecycle")
	}
	slug, err := s.uniqueAgentGroupSlug(ctx, group.Slug)
	if err != nil {
		return agentgroups.Group{}, err
	}
	group.Slug = slug

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return agentgroups.Group{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO agent_groups
			(id, name, slug, kind, description, default_model_id, default_lifecycle_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, group.ID, group.Name, group.Slug, group.Kind, group.Description, group.DefaultModelID, group.DefaultLifecycleID, group.Status, group.CreatedAt, group.UpdatedAt); err != nil {
		return agentgroups.Group{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO lifecycle_definitions
			(id, group_id, name, kind, description, max_total_iterations, max_repair_iterations, same_error_limit, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 16, 2, 2, ?, ?, ?)
	`, group.DefaultLifecycleID, group.ID, group.Name+" lifecycle", group.Kind, "Пустой lifecycle для настройки в V0.7.4.", agentgroups.StatusActive, now, now); err != nil {
		return agentgroups.Group{}, err
	}
	if err := tx.Commit(); err != nil {
		return agentgroups.Group{}, err
	}
	return s.GetAgentGroup(ctx, group.ID)
}

func (s *Store) UpdateAgentGroup(ctx context.Context, group agentgroups.Group) (agentgroups.Group, error) {
	group.ID = strings.TrimSpace(group.ID)
	group.Name = strings.TrimSpace(group.Name)
	group.Kind = normalizeGroupKind(group.Kind)
	group.Description = strings.TrimSpace(group.Description)
	group.DefaultModelID = strings.TrimSpace(group.DefaultModelID)
	if group.ID == "" {
		return agentgroups.Group{}, fmt.Errorf("group_id пустой")
	}
	if group.Name == "" {
		return agentgroups.Group{}, fmt.Errorf("название группы пустое")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_groups
		SET name = ?, kind = ?, description = ?, default_model_id = ?, updated_at = ?
		WHERE id = ?
	`, group.Name, group.Kind, group.Description, group.DefaultModelID, nowString(), group.ID)
	if err != nil {
		return agentgroups.Group{}, err
	}
	return s.GetAgentGroup(ctx, group.ID)
}

func (s *Store) ArchiveAgentGroup(ctx context.Context, groupID string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return fmt.Errorf("group_id пустой")
	}
	if groupID == "group_dev_squad" || groupID == "group_ctf_cell" || groupID == "group_research_squad" {
		return fmt.Errorf("seed-группу нельзя архивировать")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_groups SET status = ?, updated_at = ? WHERE id = ?
	`, agentgroups.StatusArchived, nowString(), groupID)
	return err
}

func (s *Store) ListAgentProfiles(ctx context.Context, groupID string) ([]agentgroups.Profile, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, name, role_key, description, avatar_path, soul_path, model_id, tool_profile_id,
			skills_json, capabilities_json, allowed_tools_json, read_paths_json, write_paths_json, handoff_rules_json,
			temperature, context_budget, enabled, sort_order, created_at, updated_at
		FROM agent_profiles
		WHERE group_id = ?
		ORDER BY sort_order ASC, name ASC
	`, strings.TrimSpace(groupID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []agentgroups.Profile
	for rows.Next() {
		var item agentgroups.Profile
		var enabled int
		var skillsJSON, capabilitiesJSON, allowedToolsJSON, readPathsJSON, writePathsJSON, handoffRulesJSON string
		if err := rows.Scan(
			&item.ID,
			&item.GroupID,
			&item.Name,
			&item.RoleKey,
			&item.Description,
			&item.AvatarPath,
			&item.SoulPath,
			&item.ModelID,
			&item.ToolProfileID,
			&skillsJSON,
			&capabilitiesJSON,
			&allowedToolsJSON,
			&readPathsJSON,
			&writePathsJSON,
			&handoffRulesJSON,
			&item.Temperature,
			&item.ContextBudget,
			&enabled,
			&item.SortOrder,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		item.DefaultSkills = decodeStringList(skillsJSON)
		item.Capabilities = decodeStringList(capabilitiesJSON)
		item.AllowedTools = decodeStringList(allowedToolsJSON)
		item.ReadPaths = decodeStringList(readPathsJSON)
		item.WritePaths = decodeStringList(writePathsJSON)
		item.HandoffRules = decodeStringList(handoffRulesJSON)
		item = agentgroups.NormalizeCapabilities(item)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetAgentProfile(ctx context.Context, profileID string) (agentgroups.Profile, error) {
	var item agentgroups.Profile
	var enabled int
	var skillsJSON, capabilitiesJSON, allowedToolsJSON, readPathsJSON, writePathsJSON, handoffRulesJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, group_id, name, role_key, description, avatar_path, soul_path, model_id, tool_profile_id,
			skills_json, capabilities_json, allowed_tools_json, read_paths_json, write_paths_json, handoff_rules_json,
			temperature, context_budget, enabled, sort_order, created_at, updated_at
		FROM agent_profiles
		WHERE id = ?
	`, strings.TrimSpace(profileID)).Scan(
		&item.ID,
		&item.GroupID,
		&item.Name,
		&item.RoleKey,
		&item.Description,
		&item.AvatarPath,
		&item.SoulPath,
		&item.ModelID,
		&item.ToolProfileID,
		&skillsJSON,
		&capabilitiesJSON,
		&allowedToolsJSON,
		&readPathsJSON,
		&writePathsJSON,
		&handoffRulesJSON,
		&item.Temperature,
		&item.ContextBudget,
		&enabled,
		&item.SortOrder,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return agentgroups.Profile{}, err
	}
	item.Enabled = enabled != 0
	item.DefaultSkills = decodeStringList(skillsJSON)
	item.Capabilities = decodeStringList(capabilitiesJSON)
	item.AllowedTools = decodeStringList(allowedToolsJSON)
	item.ReadPaths = decodeStringList(readPathsJSON)
	item.WritePaths = decodeStringList(writePathsJSON)
	item.HandoffRules = decodeStringList(handoffRulesJSON)
	item = agentgroups.NormalizeCapabilities(item)
	return item, nil
}

func (s *Store) SaveAgentProfile(ctx context.Context, profile agentgroups.Profile) (agentgroups.Profile, error) {
	now := nowString()
	profile.ID = strings.TrimSpace(profile.ID)
	profile.GroupID = strings.TrimSpace(profile.GroupID)
	profile.Name = strings.TrimSpace(profile.Name)
	profile.RoleKey = strings.TrimSpace(profile.RoleKey)
	profile.Description = strings.TrimSpace(profile.Description)
	profile.AvatarPath = strings.TrimSpace(profile.AvatarPath)
	profile.SoulPath = strings.TrimSpace(profile.SoulPath)
	profile.ModelID = strings.TrimSpace(profile.ModelID)
	profile.ToolProfileID = strings.TrimSpace(profile.ToolProfileID)
	profile.DefaultSkills = agentgroups.NormalizeDefaultSkills(profile.DefaultSkills)
	profile.Capabilities = cleanStringList(profile.Capabilities)
	profile.AllowedTools = cleanStringList(profile.AllowedTools)
	profile.ReadPaths = cleanStringList(profile.ReadPaths)
	profile.WritePaths = cleanStringList(profile.WritePaths)
	profile.HandoffRules = cleanStringList(profile.HandoffRules)
	if profile.GroupID == "" {
		return agentgroups.Profile{}, fmt.Errorf("group_id пустой")
	}
	if profile.Name == "" {
		return agentgroups.Profile{}, fmt.Errorf("имя агента пустое")
	}
	if profile.RoleKey == "" {
		profile.RoleKey = slugify(profile.Name)
	}
	if profile.ContextBudget == 0 {
		profile.ContextBudget = 8000
	}
	profile = agentgroups.NormalizeCapabilities(profile)
	if profile.ID == "" {
		profile.ID = newID("agent")
		profile.CreatedAt = now
	}
	profile.UpdatedAt = now
	skillsJSON := marshalJSON(profile.DefaultSkills, "[]")
	capabilitiesJSON := marshalJSON(profile.Capabilities, "[]")
	allowedToolsJSON := marshalJSON(profile.AllowedTools, "[]")
	readPathsJSON := marshalJSON(profile.ReadPaths, "[]")
	writePathsJSON := marshalJSON(profile.WritePaths, "[]")
	handoffRulesJSON := marshalJSON(profile.HandoffRules, "[]")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_profiles
			(id, group_id, name, role_key, description, avatar_path, soul_path, model_id, tool_profile_id,
			 skills_json, capabilities_json, allowed_tools_json, read_paths_json, write_paths_json, handoff_rules_json,
			 temperature, context_budget, enabled, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			role_key = excluded.role_key,
			description = excluded.description,
			avatar_path = excluded.avatar_path,
			soul_path = excluded.soul_path,
			model_id = excluded.model_id,
			tool_profile_id = excluded.tool_profile_id,
			skills_json = excluded.skills_json,
			capabilities_json = excluded.capabilities_json,
			allowed_tools_json = excluded.allowed_tools_json,
			read_paths_json = excluded.read_paths_json,
			write_paths_json = excluded.write_paths_json,
			handoff_rules_json = excluded.handoff_rules_json,
			temperature = excluded.temperature,
			context_budget = excluded.context_budget,
			enabled = excluded.enabled,
			sort_order = excluded.sort_order,
			updated_at = excluded.updated_at
	`, profile.ID, profile.GroupID, profile.Name, profile.RoleKey, profile.Description, profile.AvatarPath, profile.SoulPath, profile.ModelID, profile.ToolProfileID, skillsJSON, capabilitiesJSON, allowedToolsJSON, readPathsJSON, writePathsJSON, handoffRulesJSON, profile.Temperature, profile.ContextBudget, boolInt(profile.Enabled), profile.SortOrder, profile.CreatedAt, profile.UpdatedAt)
	if err != nil {
		return agentgroups.Profile{}, err
	}
	profiles, err := s.ListAgentProfiles(ctx, profile.GroupID)
	if err != nil {
		return agentgroups.Profile{}, err
	}
	for _, item := range profiles {
		if item.ID == profile.ID {
			return item, nil
		}
	}
	return profile, nil
}

func (s *Store) SetAgentProfileEnabled(ctx context.Context, profileID string, enabled bool) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_profiles SET enabled = ?, updated_at = ? WHERE id = ?
	`, boolInt(enabled), nowString(), strings.TrimSpace(profileID))
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("агент %s не найден", profileID)
	}
	return nil
}

func (s *Store) ListLifecycleDefinitions(ctx context.Context, groupID string) ([]agentgroups.LifecycleDefinition, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, name, kind, description, max_total_iterations, max_repair_iterations,
			same_error_limit, status, created_at, updated_at
		FROM lifecycle_definitions
		WHERE group_id = ? AND status <> ?
		ORDER BY name ASC
	`, strings.TrimSpace(groupID), agentgroups.StatusArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []agentgroups.LifecycleDefinition
	for rows.Next() {
		var item agentgroups.LifecycleDefinition
		if err := rows.Scan(
			&item.ID,
			&item.GroupID,
			&item.Name,
			&item.Kind,
			&item.Description,
			&item.MaxTotalIterations,
			&item.MaxRepairIterations,
			&item.SameErrorLimit,
			&item.Status,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetLifecycleDefinition(ctx context.Context, lifecycleID string) (agentgroups.LifecycleDefinition, error) {
	var item agentgroups.LifecycleDefinition
	err := s.db.QueryRowContext(ctx, `
		SELECT id, group_id, name, kind, description, max_total_iterations, max_repair_iterations,
			same_error_limit, status, created_at, updated_at
		FROM lifecycle_definitions
		WHERE id = ?
	`, strings.TrimSpace(lifecycleID)).Scan(
		&item.ID,
		&item.GroupID,
		&item.Name,
		&item.Kind,
		&item.Description,
		&item.MaxTotalIterations,
		&item.MaxRepairIterations,
		&item.SameErrorLimit,
		&item.Status,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return agentgroups.LifecycleDefinition{}, err
	}
	return item, nil
}

func (s *Store) SaveLifecycleDefinition(ctx context.Context, item agentgroups.LifecycleDefinition) (agentgroups.LifecycleDefinition, error) {
	item.ID = strings.TrimSpace(item.ID)
	item.GroupID = strings.TrimSpace(item.GroupID)
	item.Name = strings.TrimSpace(item.Name)
	item.Kind = strings.TrimSpace(item.Kind)
	item.Description = strings.TrimSpace(item.Description)
	item.Status = strings.TrimSpace(item.Status)
	if item.GroupID == "" {
		return agentgroups.LifecycleDefinition{}, errors.New("group id is required")
	}
	if item.Name == "" {
		return agentgroups.LifecycleDefinition{}, errors.New("lifecycle name is required")
	}
	if item.Kind == "" {
		item.Kind = agentgroups.GroupKindCustom
	}
	if item.Status == "" {
		item.Status = agentgroups.StatusActive
	}
	if item.MaxTotalIterations <= 0 {
		item.MaxTotalIterations = 16
	}
	if item.MaxRepairIterations < 0 {
		item.MaxRepairIterations = 0
	}
	if item.SameErrorLimit <= 0 {
		item.SameErrorLimit = 2
	}
	now := nowString()
	if item.ID == "" {
		item.ID = newID("lifecycle")
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO lifecycle_definitions
				(id, group_id, name, kind, description, max_total_iterations, max_repair_iterations,
				 same_error_limit, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, item.ID, item.GroupID, item.Name, item.Kind, item.Description, item.MaxTotalIterations,
			item.MaxRepairIterations, item.SameErrorLimit, item.Status, now, now)
		if err != nil {
			return agentgroups.LifecycleDefinition{}, err
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			UPDATE lifecycle_definitions
			SET name = ?, kind = ?, description = ?, max_total_iterations = ?, max_repair_iterations = ?,
				same_error_limit = ?, status = ?, updated_at = ?
			WHERE id = ? AND group_id = ?
		`, item.Name, item.Kind, item.Description, item.MaxTotalIterations, item.MaxRepairIterations,
			item.SameErrorLimit, item.Status, now, item.ID, item.GroupID)
		if err != nil {
			return agentgroups.LifecycleDefinition{}, err
		}
	}
	if item.Status == agentgroups.StatusActive {
		_, _ = s.db.ExecContext(ctx, `
			UPDATE agent_groups SET default_lifecycle_id = ?, updated_at = ? WHERE id = ? AND default_lifecycle_id = ''
		`, item.ID, now, item.GroupID)
	}
	return s.GetLifecycleDefinition(ctx, item.ID)
}

func (s *Store) ListLifecycleSteps(ctx context.Context, lifecycleID string) ([]agentgroups.LifecycleStep, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, lifecycle_id, step_key, title, agent_profile_id, mode, required, can_retry, max_retries,
			on_success_step_key, on_failure_step_key, output_schema, visible_to_user, sort_order
		FROM lifecycle_steps
		WHERE lifecycle_id = ?
		ORDER BY sort_order ASC
	`, strings.TrimSpace(lifecycleID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []agentgroups.LifecycleStep
	for rows.Next() {
		var item agentgroups.LifecycleStep
		var required int
		var canRetry int
		var visible int
		if err := rows.Scan(
			&item.ID,
			&item.LifecycleID,
			&item.StepKey,
			&item.Title,
			&item.AgentProfileID,
			&item.Mode,
			&required,
			&canRetry,
			&item.MaxRetries,
			&item.OnSuccessStepKey,
			&item.OnFailureStepKey,
			&item.OutputSchema,
			&visible,
			&item.SortOrder,
		); err != nil {
			return nil, err
		}
		item.Required = required != 0
		item.CanRetry = canRetry != 0
		item.VisibleToUser = visible != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetLifecycleStep(ctx context.Context, stepID string) (agentgroups.LifecycleStep, error) {
	var item agentgroups.LifecycleStep
	var required int
	var canRetry int
	var visible int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, lifecycle_id, step_key, title, agent_profile_id, mode, required, can_retry, max_retries,
			on_success_step_key, on_failure_step_key, output_schema, visible_to_user, sort_order
		FROM lifecycle_steps
		WHERE id = ?
	`, strings.TrimSpace(stepID)).Scan(
		&item.ID,
		&item.LifecycleID,
		&item.StepKey,
		&item.Title,
		&item.AgentProfileID,
		&item.Mode,
		&required,
		&canRetry,
		&item.MaxRetries,
		&item.OnSuccessStepKey,
		&item.OnFailureStepKey,
		&item.OutputSchema,
		&visible,
		&item.SortOrder,
	)
	if err != nil {
		return agentgroups.LifecycleStep{}, err
	}
	item.Required = required != 0
	item.CanRetry = canRetry != 0
	item.VisibleToUser = visible != 0
	return item, nil
}

func (s *Store) SaveLifecycleStep(ctx context.Context, item agentgroups.LifecycleStep) (agentgroups.LifecycleStep, error) {
	item.ID = strings.TrimSpace(item.ID)
	item.LifecycleID = strings.TrimSpace(item.LifecycleID)
	item.StepKey = strings.TrimSpace(item.StepKey)
	item.Title = strings.TrimSpace(item.Title)
	item.AgentProfileID = strings.TrimSpace(item.AgentProfileID)
	item.Mode = strings.TrimSpace(item.Mode)
	item.OnSuccessStepKey = strings.TrimSpace(item.OnSuccessStepKey)
	item.OnFailureStepKey = strings.TrimSpace(item.OnFailureStepKey)
	item.OutputSchema = strings.TrimSpace(item.OutputSchema)
	if item.LifecycleID == "" {
		return agentgroups.LifecycleStep{}, errors.New("lifecycle id is required")
	}
	if item.StepKey == "" {
		return agentgroups.LifecycleStep{}, errors.New("step key is required")
	}
	if item.Title == "" {
		item.Title = item.StepKey
	}
	if item.Mode == "" {
		item.Mode = "llm"
	}
	if item.MaxRetries < 0 {
		item.MaxRetries = 0
	}
	if item.ID == "" {
		item.ID = newID("lstep")
		if item.SortOrder < 0 {
			item.SortOrder = 0
		}
		if item.SortOrder == 0 {
			_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order), -1) + 1 FROM lifecycle_steps WHERE lifecycle_id = ?`, item.LifecycleID).Scan(&item.SortOrder)
		}
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO lifecycle_steps
				(id, lifecycle_id, step_key, title, agent_profile_id, mode, required, can_retry, max_retries,
				 on_success_step_key, on_failure_step_key, output_schema, visible_to_user, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, item.ID, item.LifecycleID, item.StepKey, item.Title, item.AgentProfileID, item.Mode,
			boolInt(item.Required), boolInt(item.CanRetry), item.MaxRetries, item.OnSuccessStepKey,
			item.OnFailureStepKey, item.OutputSchema, boolInt(item.VisibleToUser), item.SortOrder)
		if err != nil {
			return agentgroups.LifecycleStep{}, err
		}
	} else {
		_, err := s.db.ExecContext(ctx, `
			UPDATE lifecycle_steps
			SET step_key = ?, title = ?, agent_profile_id = ?, mode = ?, required = ?, can_retry = ?,
				max_retries = ?, on_success_step_key = ?, on_failure_step_key = ?, output_schema = ?,
				visible_to_user = ?, sort_order = ?
			WHERE id = ? AND lifecycle_id = ?
		`, item.StepKey, item.Title, item.AgentProfileID, item.Mode, boolInt(item.Required),
			boolInt(item.CanRetry), item.MaxRetries, item.OnSuccessStepKey, item.OnFailureStepKey,
			item.OutputSchema, boolInt(item.VisibleToUser), item.SortOrder, item.ID, item.LifecycleID)
		if err != nil {
			return agentgroups.LifecycleStep{}, err
		}
	}
	return s.GetLifecycleStep(ctx, item.ID)
}

func (s *Store) DeleteLifecycleStep(ctx context.Context, stepID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM lifecycle_steps WHERE id = ?`, strings.TrimSpace(stepID))
	return err
}

func (s *Store) ProjectLifecycle(ctx context.Context, projectID string) (agentgroups.LifecycleDefinition, []agentgroups.LifecycleStep, error) {
	binding, err := s.ProjectGroupBinding(ctx, projectID)
	if err != nil {
		return agentgroups.LifecycleDefinition{}, nil, err
	}
	lifecycleID := strings.TrimSpace(binding.LifecycleID)
	if lifecycleID == "" {
		groups, listErr := s.ListLifecycleDefinitions(ctx, binding.GroupID)
		if listErr != nil {
			return agentgroups.LifecycleDefinition{}, nil, listErr
		}
		if len(groups) == 0 {
			return agentgroups.LifecycleDefinition{}, nil, sql.ErrNoRows
		}
		lifecycleID = groups[0].ID
	}
	definition, err := s.GetLifecycleDefinition(ctx, lifecycleID)
	if err != nil {
		return agentgroups.LifecycleDefinition{}, nil, err
	}
	steps, err := s.ListLifecycleSteps(ctx, lifecycleID)
	if err != nil {
		return agentgroups.LifecycleDefinition{}, nil, err
	}
	return definition, steps, nil
}

func (s *Store) BindProjectToAgentGroup(ctx context.Context, projectID string, groupID string, lifecycleID string) (agentgroups.ProjectBinding, error) {
	projectID = strings.TrimSpace(projectID)
	groupID = strings.TrimSpace(groupID)
	lifecycleID = strings.TrimSpace(lifecycleID)
	if projectID == "" {
		return agentgroups.ProjectBinding{}, fmt.Errorf("project_id пустой")
	}
	if groupID == "" {
		return agentgroups.ProjectBinding{}, fmt.Errorf("group_id пустой")
	}
	if lifecycleID == "" {
		group, err := s.GetAgentGroup(ctx, groupID)
		if err != nil {
			return agentgroups.ProjectBinding{}, err
		}
		lifecycleID = group.DefaultLifecycleID
	}

	now := nowString()
	id := newID("binding")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO project_group_bindings
			(id, project_id, group_id, lifecycle_id, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			group_id = excluded.group_id,
			lifecycle_id = excluded.lifecycle_id,
			updated_at = excluded.updated_at
	`, id, projectID, groupID, lifecycleID, now, now)
	if err != nil {
		return agentgroups.ProjectBinding{}, err
	}
	return s.ProjectGroupBinding(ctx, projectID)
}

func (s *Store) ProjectGroupBinding(ctx context.Context, projectID string) (agentgroups.ProjectBinding, error) {
	var item agentgroups.ProjectBinding
	var isDefault int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, group_id, lifecycle_id, is_default, created_at, updated_at
		FROM project_group_bindings
		WHERE project_id = ?
	`, strings.TrimSpace(projectID)).Scan(
		&item.ID,
		&item.ProjectID,
		&item.GroupID,
		&item.LifecycleID,
		&isDefault,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		if _, bindErr := s.BindProjectToAgentGroup(ctx, projectID, "group_dev_squad", "lifecycle_dev_default"); bindErr != nil {
			return agentgroups.ProjectBinding{}, bindErr
		}
		return s.ProjectGroupBinding(ctx, projectID)
	}
	if err != nil {
		return agentgroups.ProjectBinding{}, err
	}
	item.IsDefault = isDefault != 0
	return item, nil
}

func (s *Store) EnsureDefaultProjectGroupBindings(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM projects
		WHERE id NOT IN (SELECT project_id FROM project_group_bindings)
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var projectIDs []string
	for rows.Next() {
		var projectID string
		if err := rows.Scan(&projectID); err != nil {
			return err
		}
		projectIDs = append(projectIDs, projectID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, projectID := range projectIDs {
		if _, err := s.BindProjectToAgentGroup(ctx, projectID, "group_dev_squad", "lifecycle_dev_default"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ensureSeedGroup(ctx context.Context, group agentgroups.Group, profiles []agentgroups.Profile, lifecycle agentgroups.LifecycleDefinition, steps []agentgroups.LifecycleStep) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO agent_groups
			(id, name, slug, kind, description, default_model_id, default_lifecycle_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, group.ID, group.Name, group.Slug, group.Kind, group.Description, group.DefaultModelID, lifecycle.ID, group.Status, group.CreatedAt, group.UpdatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE agent_groups
		SET default_lifecycle_id = CASE WHEN default_lifecycle_id = '' THEN ? ELSE default_lifecycle_id END,
			default_model_id = CASE WHEN default_model_id = '' THEN ? ELSE default_model_id END
		WHERE id = ?
	`, lifecycle.ID, group.DefaultModelID, group.ID); err != nil {
		return err
	}

	lifecycle.GroupID = group.ID
	if lifecycle.MaxTotalIterations == 0 {
		lifecycle.MaxTotalIterations = 16
	}
	if lifecycle.MaxRepairIterations == 0 {
		lifecycle.MaxRepairIterations = 2
	}
	if lifecycle.SameErrorLimit == 0 {
		lifecycle.SameErrorLimit = 2
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO lifecycle_definitions
			(id, group_id, name, kind, description, max_total_iterations, max_repair_iterations, same_error_limit, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, lifecycle.ID, lifecycle.GroupID, lifecycle.Name, lifecycle.Kind, lifecycle.Description, lifecycle.MaxTotalIterations, lifecycle.MaxRepairIterations, lifecycle.SameErrorLimit, lifecycle.Status, lifecycle.CreatedAt, lifecycle.UpdatedAt); err != nil {
		return err
	}

	for index, profile := range profiles {
		profile.GroupID = group.ID
		profile.ModelID = strings.TrimSpace(profile.ModelID)
		if profile.ModelID == "" {
			profile.ModelID = group.DefaultModelID
		}
		if profile.ContextBudget == 0 {
			profile.ContextBudget = 8000
		}
		profile.DefaultSkills = agentgroups.NormalizeDefaultSkills(profile.DefaultSkills)
		if profile.CreatedAt == "" {
			profile.CreatedAt = group.CreatedAt
		}
		if profile.UpdatedAt == "" {
			profile.UpdatedAt = group.UpdatedAt
		}
		profile = agentgroups.NormalizeCapabilities(profile)
		skillsJSON := marshalJSON(profile.DefaultSkills, "[]")
		capabilitiesJSON := marshalJSON(profile.Capabilities, "[]")
		allowedToolsJSON := marshalJSON(profile.AllowedTools, "[]")
		readPathsJSON := marshalJSON(profile.ReadPaths, "[]")
		writePathsJSON := marshalJSON(profile.WritePaths, "[]")
		handoffRulesJSON := marshalJSON(profile.HandoffRules, "[]")
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO agent_profiles
				(id, group_id, name, role_key, description, avatar_path, soul_path, model_id, tool_profile_id,
				 skills_json, capabilities_json, allowed_tools_json, read_paths_json, write_paths_json, handoff_rules_json,
				 temperature, context_budget, enabled, sort_order, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, profile.ID, profile.GroupID, profile.Name, profile.RoleKey, profile.Description, profile.AvatarPath, profile.SoulPath, profile.ModelID, profile.ToolProfileID, skillsJSON, capabilitiesJSON, allowedToolsJSON, readPathsJSON, writePathsJSON, handoffRulesJSON, profile.Temperature, profile.ContextBudget, boolInt(profile.Enabled), index, profile.CreatedAt, profile.UpdatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE agent_profiles
			SET skills_json = CASE WHEN skills_json = '[]' THEN ? ELSE skills_json END,
				capabilities_json = CASE WHEN capabilities_json = '[]' THEN ? ELSE capabilities_json END,
				allowed_tools_json = CASE WHEN allowed_tools_json = '[]' THEN ? ELSE allowed_tools_json END,
				read_paths_json = CASE WHEN read_paths_json = '[]' THEN ? ELSE read_paths_json END,
				write_paths_json = CASE WHEN write_paths_json = '[]' THEN ? ELSE write_paths_json END,
				handoff_rules_json = CASE WHEN handoff_rules_json = '[]' THEN ? ELSE handoff_rules_json END
			WHERE id = ?
		`, skillsJSON, capabilitiesJSON, allowedToolsJSON, readPathsJSON, writePathsJSON, handoffRulesJSON, profile.ID); err != nil {
			return err
		}
	}

	for index, step := range steps {
		if step.Mode == "" {
			step.Mode = "llm"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO lifecycle_steps
				(id, lifecycle_id, step_key, title, agent_profile_id, mode, required, can_retry, max_retries,
				 on_success_step_key, on_failure_step_key, output_schema, visible_to_user, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, step.ID, lifecycle.ID, step.StepKey, step.Title, step.AgentProfileID, step.Mode, boolInt(step.Required), boolInt(step.CanRetry), step.MaxRetries, step.OnSuccessStepKey, step.OnFailureStepKey, step.OutputSchema, boolInt(step.VisibleToUser), index); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE lifecycle_steps
			SET output_schema = CASE WHEN output_schema = '' THEN ? ELSE output_schema END,
				on_failure_step_key = CASE WHEN on_failure_step_key = '' THEN ? ELSE on_failure_step_key END
			WHERE id = ?
		`, step.OutputSchema, step.OnFailureStepKey, step.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func lifecycleReturnConfig(stepKey string) string {
	return fmt.Sprintf(`{"returnToStepKey":%q}`, stepKey)
}

func lifecycleHumanGateConfig(reason string, requiredInputs []string) string {
	var quoted []string
	for _, item := range requiredInputs {
		quoted = append(quoted, fmt.Sprintf("%q", item))
	}
	return fmt.Sprintf(`{"humanGate":{"reason":%q,"requiredInputs":[%s]}}`, reason, strings.Join(quoted, ","))
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

func normalizeGroupKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case agentgroups.GroupKindDev:
		return agentgroups.GroupKindDev
	case agentgroups.GroupKindCTF:
		return agentgroups.GroupKindCTF
	case agentgroups.GroupKindResearch:
		return agentgroups.GroupKindResearch
	case agentgroups.GroupKindSecurity:
		return agentgroups.GroupKindSecurity
	default:
		return agentgroups.GroupKindCustom
	}
}

func uniqueSlug(slug string, name string, fallback string) string {
	base := slugify(slug)
	if base == "" {
		base = slugify(name)
	}
	if base == "" {
		base = slugify(fallback)
	}
	if base == "" {
		base = "group"
	}
	return base
}

func (s *Store) uniqueAgentGroupSlug(ctx context.Context, base string) (string, error) {
	base = uniqueSlug(base, "", "")
	for index := 0; index < 1000; index++ {
		candidate := base
		if index > 0 {
			candidate = fmt.Sprintf("%s-%d", base, index+1)
		}
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_groups WHERE slug = ?`, candidate).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("не удалось подобрать уникальный slug для группы")
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if r >= 'а' && r <= 'я' || r == 'ё' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func marshalJSON(value any, fallback string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}

func decodeStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return nil
	}
	return cleanStringList(values)
}

func decodeAcceptedAnswers(raw string) []taskspec.AcceptedAnswer {
	var values []taskspec.AcceptedAnswer
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return nil
	}
	return cleanAcceptedAnswers(values)
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func cleanAcceptedAnswers(values []taskspec.AcceptedAnswer) []taskspec.AcceptedAnswer {
	out := make([]taskspec.AcceptedAnswer, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value.QuestionID = strings.TrimSpace(value.QuestionID)
		value.Question = strings.TrimSpace(value.Question)
		value.Answer = strings.TrimSpace(value.Answer)
		if value.Answer == "" {
			continue
		}
		key := value.QuestionID
		if key == "" {
			key = value.Question
		}
		if key == "" {
			key = value.Answer
		}
		key = strings.ToLower(key)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func workflowPlanForRunTx(ctx context.Context, tx *sql.Tx, workflowRunID string) (*workflow.Plan, []workflow.PlanStep, error) {
	var plan workflow.Plan
	err := tx.QueryRowContext(ctx, `
		SELECT id, project_id, task_id, workflow_run_id, title, status, current_step_id, created_at, updated_at
		FROM workflow_plans
		WHERE workflow_run_id = ?
	`, workflowRunID).Scan(
		&plan.ID,
		&plan.ProjectID,
		&plan.TaskID,
		&plan.WorkflowRunID,
		&plan.Title,
		&plan.Status,
		&plan.CurrentStepID,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, plan_id, step_key, title, description, agent_id, status, started_at, finished_at, error, sort_order
		FROM workflow_plan_steps
		WHERE plan_id = ?
		ORDER BY sort_order ASC
	`, plan.ID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var steps []workflow.PlanStep
	for rows.Next() {
		var step workflow.PlanStep
		if err := rows.Scan(
			&step.ID,
			&step.PlanID,
			&step.StepKey,
			&step.Title,
			&step.Description,
			&step.AgentID,
			&step.Status,
			&step.StartedAt,
			&step.FinishedAt,
			&step.Error,
			&step.SortOrder,
		); err != nil {
			return nil, nil, err
		}
		steps = append(steps, step)
	}
	return &plan, steps, rows.Err()
}

func choosePlanStep(steps []workflow.PlanStep, stepKey string, fallbackAgentID string) workflow.PlanStep {
	stepKey = strings.TrimSpace(stepKey)
	fallbackAgentID = strings.TrimSpace(fallbackAgentID)
	for _, step := range steps {
		if step.StepKey == stepKey {
			return step
		}
	}
	for _, step := range steps {
		if fallbackAgentID != "" && step.AgentID == fallbackAgentID && (step.Status == workflow.StepStatusQueued || step.Status == workflow.StepStatusRunning) {
			return step
		}
	}
	for _, step := range steps {
		if step.Status == workflow.StepStatusQueued || step.Status == workflow.StepStatusRunning {
			return step
		}
	}
	if len(steps) > 0 {
		return steps[len(steps)-1]
	}
	return workflow.PlanStep{}
}

func allPlanStepsDone(steps []workflow.PlanStep, updatedStepID string, updatedStatus string) bool {
	for _, step := range steps {
		status := step.Status
		if step.ID == updatedStepID {
			status = updatedStatus
		}
		if status != workflow.StepStatusDone && status != workflow.StepStatusSkipped {
			return false
		}
	}
	return true
}

func newID(prefix string) string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(bytes[:])
}
