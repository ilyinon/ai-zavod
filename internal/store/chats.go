package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"zavod_ai/internal/chat"
	"zavod_ai/internal/webresearch"
)

// Rebuild only the parent table, with foreign keys disabled on the single connection.
// Child rows and their references keep their original task IDs.
func (s *Store) migrateChats(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS chat_sources (task_id TEXT PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE, payload TEXT NOT NULL)`); err != nil {
		return err
	}
	var migrated int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pragma_table_info('tasks') WHERE name = 'pinned'`).Scan(&migrated); err != nil {
		return err
	}
	if migrated > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer s.db.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE tasks_chats (id TEXT PRIMARY KEY, project_id TEXT, title TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, pinned INTEGER NOT NULL DEFAULT 0, pending_request TEXT NOT NULL DEFAULT '', FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE SET NULL)`,
		`INSERT INTO tasks_chats (id, project_id, title, status, created_at, updated_at) SELECT id, project_id, title, status, created_at, updated_at FROM tasks`,
		`DROP TABLE tasks`, `ALTER TABLE tasks_chats RENAME TO tasks`,
		`CREATE INDEX idx_tasks_project ON tasks(project_id, updated_at)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	invalid := rows.Next()
	rows.Close()
	if invalid {
		return fmt.Errorf("chat migration: foreign key check failed")
	}
	return tx.Commit()
}

func (s *Store) SaveChatSources(ctx context.Context, taskID string, sources []webresearch.Source) error {
	payload, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO chat_sources (task_id, payload) VALUES (?, ?) ON CONFLICT(task_id) DO UPDATE SET payload=excluded.payload`, taskID, string(payload))
	return err
}

func (s *Store) ChatSources(ctx context.Context, taskID string) ([]webresearch.Source, error) {
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM chat_sources WHERE task_id = ?`, taskID).Scan(&payload)
	if err == sql.ErrNoRows {
		return []webresearch.Source{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sources []webresearch.Source
	err = json.Unmarshal([]byte(payload), &sources)
	return sources, err
}

const taskColumns = `id, COALESCE(project_id, ''), title, status, created_at, updated_at, pinned, pending_request, group_id, model_id`

func scanTask(row interface{ Scan(...any) error }) (chat.Task, error) {
	var t chat.Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Status, &t.CreatedAt, &t.UpdatedAt, &t.Pinned, &t.PendingRequest, &t.GroupID, &t.ModelID)
	return t, err
}

func (s *Store) GetTask(ctx context.Context, id string) (chat.Task, error) {
	return scanTask(s.db.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
}

func (s *Store) WorkflowTask(ctx context.Context, runID, projectID string) (*chat.Task, error) {
	var taskID string
	if err := s.db.QueryRowContext(ctx, `SELECT task_id FROM workflow_runs WHERE id = ?`, runID).Scan(&taskID); err != nil {
		return nil, err
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.ProjectID != projectID {
		return nil, fmt.Errorf("запуск принадлежит другому проекту")
	}
	return &task, nil
}

func (s *Store) ListChats(ctx context.Context) ([]chat.Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskColumns+` FROM tasks ORDER BY pinned DESC, updated_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []chat.Task{}
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) UpdateChat(ctx context.Context, t chat.Task) (chat.Task, error) {
	t.Title = strings.TrimSpace(t.Title)
	if t.Title == "" {
		return t, fmt.Errorf("название чата пустое")
	}
	if t.Status != "active" && t.Status != "archived" {
		return t, fmt.Errorf("некорректный статус чата")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET project_id = NULLIF(?, ''), title = ?, status = ?, pinned = ?, pending_request = ?, group_id = ?, model_id = ?, updated_at = ? WHERE id = ?`, t.ProjectID, t.Title, t.Status, t.Pinned, t.PendingRequest, t.GroupID, t.ModelID, nowString(), t.ID)
	if err != nil {
		return t, err
	}
	return s.GetTask(ctx, t.ID)
}

func (s *Store) DeleteChat(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
