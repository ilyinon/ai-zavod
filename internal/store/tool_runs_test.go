package store

import (
	"context"
	"path/filepath"
	"testing"
	"zavod_ai/internal/toolruntime"
)

func TestToolAuditIsolationRecoveryAndCascade(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")
	s, err := New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "Project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateTask(ctx, p.ID, "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateTask(ctx, p.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	inv := toolruntime.Invocation{ID: "i", LoopID: "loop", StartedAt: "2026-09-06T00:00:00Z", Scope: toolruntime.Scope{TaskID: first.ID, ProjectID: p.ID, AgentID: "agent"}, Result: toolruntime.Result{Status: "running"}}
	if err := s.SaveToolInvocation(ctx, inv); err != nil {
		t.Fatal(err)
	}
	wrong := inv
	wrong.TaskID = second.ID
	if err := s.SaveToolInvocation(ctx, wrong); err == nil {
		t.Fatal("reassigned audit to another chat")
	}
	wrong = inv
	wrong.ProjectID = "other"
	if err := s.SaveToolInvocation(ctx, wrong); err == nil {
		t.Fatal("project mismatch accepted")
	}
	if items, err := s.ListToolInvocations(ctx, second.ID); err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
	s.Close()
	s, err = New(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items, err := s.ListToolInvocations(ctx, first.ID)
	if err != nil || len(items) != 1 || items[0].Result.Status != "interrupted" {
		t.Fatalf("recovery: %+v %v", items, err)
	}
	if err := s.DeleteChat(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if items, err := s.ListToolInvocations(ctx, first.ID); err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
}
