package store

import (
	"context"
	"encoding/json"
	"errors"

	"zavod_ai/internal/toolruntime"
)

func (s *Store) migrateToolRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tool_invocations (
        id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
        project_id TEXT NOT NULL, loop_id TEXT NOT NULL, started_at TEXT NOT NULL, payload TEXT NOT NULL
    ); CREATE INDEX IF NOT EXISTS tool_invocations_task ON tool_invocations(task_id, started_at);
    UPDATE tool_invocations SET payload=json_set(payload, '$.result.status', 'interrupted', '$.result.error', 'Приложение завершилось до получения результата', '$.finishedAt', strftime('%Y-%m-%dT%H:%M:%SZ','now'))
    WHERE json_extract(payload,'$.result.status')='running';`)
	return err
}

func (s *Store) SaveToolInvocation(ctx context.Context, item toolruntime.Invocation) error {
	task, err := s.GetTask(ctx, item.TaskID)
	if err != nil {
		return err
	}
	if task.ProjectID != item.ProjectID || item.ProjectID == "" {
		return errors.New("tool invocation project/task mismatch")
	}
	if item.WorkflowRunID != "" {
		workflowTask, err := s.WorkflowTask(ctx, item.WorkflowRunID, item.ProjectID)
		if err != nil {
			return err
		}
		if workflowTask.ID != task.ID {
			return errors.New("tool invocation workflow/task mismatch")
		}
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO tool_invocations(id,task_id,project_id,loop_id,started_at,payload) VALUES(?,?,?,?,?,?)
        ON CONFLICT(id) DO UPDATE SET payload=excluded.payload
        WHERE tool_invocations.task_id=excluded.task_id AND tool_invocations.project_id=excluded.project_id AND tool_invocations.loop_id=excluded.loop_id`,
		item.ID, item.TaskID, item.ProjectID, item.LoopID, item.StartedAt, string(data))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return errors.New("tool invocation ownership mismatch")
	}
	return err
}

func (s *Store) ListToolInvocations(ctx context.Context, taskID string) ([]toolruntime.Invocation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM tool_invocations WHERE task_id=? ORDER BY started_at DESC,id DESC LIMIT 100`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []toolruntime.Invocation{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var item toolruntime.Invocation
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
