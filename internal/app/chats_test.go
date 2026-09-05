package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"zavod_ai/internal/agents"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/config"
	"zavod_ai/internal/store"
)

func chatTestService(t *testing.T) *Service {
	t.Helper()
	db, err := store.New(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.EnsureDefaultModels(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := db.EnsureDefaultAgentGroups(context.Background(), "qwen-remote"); err != nil {
		t.Fatal(err)
	}
	return &Service{store: db, agentStatuses: map[string]agents.Status{}}
}

func TestFreshBootstrapHasNoRequiredProject(t *testing.T) {
	s := chatTestService(t)
	root := t.TempDir()
	s.paths = config.Paths{CodeDir: root, ProjectsDir: filepath.Join(root, "projects"), AgentsDir: filepath.Join(root, "agents")}
	state, err := s.Bootstrap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.Projects == nil || len(state.Projects) != 0 || state.Chat.Task != nil || state.SelectedProjectID != "" {
		t.Fatalf("unexpected initial state: %+v", state)
	}
	if _, err := os.Stat(s.paths.ProjectsDir); !os.IsNotExist(err) {
		t.Fatalf("bootstrap created a workspace: %v", err)
	}
}

func TestChatGroupDoesNotChangeProjectDefault(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	p, err := s.store.CreateProject(ctx, "Dev", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.store.ProjectGroupBinding(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.UpdateChat(ctx, UpdateChatInput{TaskID: created.Task.ID, ProjectID: p.ID, Title: "Research", GroupID: "group_research_squad"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := s.store.ProjectGroupBinding(s.chatGroupContext(ctx, task, task.GroupID), p.ID)
	if err != nil || binding.GroupID != "group_research_squad" {
		t.Fatalf("missing override: %+v %v", binding, err)
	}
	unchanged, err := s.store.ProjectGroupBinding(ctx, p.ID)
	if err != nil || unchanged.GroupID != original.GroupID {
		t.Fatalf("changed default: %+v %v", unchanged, err)
	}
}

func TestChatSelectionDoesNotLeakAnotherTask(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	p, err := s.store.CreateProject(ctx, "Project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.store.AddMessage(ctx, first.Task.ID, "user", "", "First only")
	second, err := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.store.AddMessage(ctx, second.Task.ID, "user", "", "Second only")
	if _, err := s.SelectChat(ctx, second.Task.ID); err != nil {
		t.Fatal(err)
	}
	execution := context.WithValue(ctx, chatContextKey{}, first.Task.ID)
	state := s.emitChatState(execution, p.ID, "")
	if state.Task.ID != first.Task.ID || len(state.Messages) != 1 || state.Messages[0].Content != "First only" {
		t.Fatalf("wrong chat in event: %+v", state)
	}
	free, err := s.CreateChat(ctx, CreateChatInput{})
	if err != nil {
		t.Fatal(err)
	}
	if free.Project.ID != "" || len(free.Messages) != 0 {
		t.Fatalf("new chat inherited project: %+v", free)
	}
	_, err = s.UpdateChat(ctx, UpdateChatInput{TaskID: free.Task.ID, Title: "Moved", ProjectID: p.ID, Pinned: true})
	if err != nil {
		t.Fatal(err)
	}
	state2, err := s.SelectChat(ctx, free.Task.ID)
	if err != nil || state2.Project.ID != p.ID || !state2.Task.Pinned {
		t.Fatalf("attachment failed: %+v %v", state2, err)
	}
	if err := s.DeleteChat(ctx, free.Task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.Path); err != nil {
		t.Fatal("workspace removed", err)
	}
}

func TestChatQueueSerializesProjectAndHonorsCancellation(t *testing.T) {
	s := chatTestService(t)
	ctx := context.Background()
	first := chat.Task{ID: "first", ProjectID: "p"}
	second := chat.Task{ID: "second", ProjectID: "p"}
	release, err := s.acquireChatWork(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.acquireChatWork(ctx, first); err == nil {
		t.Fatal("duplicate execution allowed")
	}
	canceled, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if unlock, err := s.acquireChatWork(canceled, second); err == nil {
		unlock()
		t.Fatal("concurrent writes allowed")
	}
	other, err := s.acquireChatWork(ctx, chat.Task{ID: "other", ProjectID: "different"})
	if err != nil {
		t.Fatal(err)
	}
	other()
	release()
	unlock, err := s.acquireChatWork(ctx, second)
	if err != nil {
		t.Fatal("canceled queue entry stuck", err)
	}
	unlock()
}

func TestProjectlessQuestionAndWorkspaceRequest(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"Привет!"}}]}`))
	}))
	defer server.Close()
	model, err := s.store.ActiveModelConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	model.BaseURL = server.URL
	if _, err := s.store.SaveModelConfig(ctx, model); err != nil {
		t.Fatal(err)
	}
	created, err := s.CreateChat(ctx, CreateChatInput{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.SendMessage(ctx, SendMessageInput{TaskID: created.Task.ID, Content: "привет"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Messages) != 2 || result.WorkflowRun != nil || result.Task.Title == "Новый чат" {
		t.Fatalf("direct answer failed: %+v", result)
	}
	code, err := s.CreateChat(ctx, CreateChatInput{})
	if err != nil {
		t.Fatal(err)
	}
	result, err = s.SendMessage(ctx, SendMessageInput{TaskID: code.Task.ID, Content: "Напиши на Go скрипт проверки доступности сайта"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.PendingRequest == "" || result.WorkflowRun != nil {
		t.Fatalf("missing workspace gate: %+v", result)
	}
	projects, err := s.store.ListProjects(ctx, "")
	if err != nil || len(projects) != 0 {
		t.Fatalf("created an unwanted project: %+v %v", projects, err)
	}
}
