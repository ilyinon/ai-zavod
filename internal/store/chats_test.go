package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestChatsMigrateLegacyDatabaseWithoutLosingHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`CREATE TABLE projects (id TEXT PRIMARY KEY, name TEXT NOT NULL, path TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL, last_opened_at TEXT NOT NULL)`,
		`CREATE TABLE tasks (id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE, title TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE messages (id TEXT PRIMARY KEY, task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE, role TEXT NOT NULL, agent_id TEXT NOT NULL DEFAULT '', content TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`INSERT INTO projects VALUES ('p', 'Existing', '/existing', '2026', '2026')`,
		`INSERT INTO tasks VALUES ('t', 'p', 'History', 'active', '2026', '2026')`,
		`INSERT INTO messages VALUES ('m', 't', 'user', '', 'preserve me', '2026')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.GetTask(ctx, "t")
	if err != nil || task.ProjectID != "p" {
		t.Fatalf("lost task: %+v %v", task, err)
	}
	messages, err := s.ListMessages(ctx, "t")
	if err != nil || len(messages) != 1 || messages[0].Content != "preserve me" {
		t.Fatalf("lost messages: %+v %v", messages, err)
	}
	free, err := s.CreateTask(ctx, "", "Unbound")
	if err != nil {
		t.Fatal(err)
	}
	free.Pinned = true
	free.Status = "archived"
	if _, err := s.UpdateChat(ctx, free); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	free, err = s.GetTask(ctx, free.ID)
	if err != nil || !free.Pinned || free.Status != "archived" || free.ProjectID != "" {
		t.Fatalf("reopen lost metadata: %+v %v", free, err)
	}
	if err := s.DeleteProject(ctx, "p"); err != nil {
		t.Fatal(err)
	}
	task, err = s.GetTask(ctx, "t")
	if err != nil || task.ProjectID != "" {
		t.Fatalf("project deletion lost chat: %+v %v", task, err)
	}
	if err := s.DeleteChat(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	messages, err = s.ListMessages(ctx, "t")
	if err != nil || len(messages) != 0 {
		t.Fatalf("messages not removed: %+v %v", messages, err)
	}
}

func TestChatsInSameProjectAreIndependent(t *testing.T) {
	ctx := context.Background()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.CreateProject(ctx, "Project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, _ := s.CreateTask(ctx, p.ID, "First")
	second, _ := s.CreateTask(ctx, p.ID, "Second")
	if _, err := s.AddMessage(ctx, first.ID, "user", "", "Only first"); err != nil {
		t.Fatal(err)
	}
	messages, err := s.ListMessages(ctx, second.ID)
	if err != nil || len(messages) != 0 {
		t.Fatalf("cross-chat history: %+v %v", messages, err)
	}
	if err := s.DeleteChat(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProject(ctx, p.ID); err != nil {
		t.Fatal("deleting a chat removed its project", err)
	}
	if _, err := s.GetTask(ctx, second.ID); err != nil {
		t.Fatal("deleting a chat removed another chat", err)
	}
}
