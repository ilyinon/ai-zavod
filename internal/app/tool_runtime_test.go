package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zavod_ai/internal/config"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/providers/openaiapi"
	"zavod_ai/internal/toolruntime"
)

func TestProjectDiagnosticToolLoopAndConsent(t *testing.T) {
	ctx := context.Background()
	s := chatTestService(t)
	root := t.TempDir()
	s.paths = config.Paths{AgentsDir: filepath.Join(t.TempDir(), "agents")}
	for name, content := range map[string]string{"go.mod": "module fixture\n\ngo 1.25\n", "broken_test.go": "package fixture\nimport \"testing\"\nfunc TestBroken(t *testing.T){ t.Fatal(\"fixture diagnostic failure\") }\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.store.CreateProject(ctx, "Diagnostic", root)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateChat(ctx, CreateChatInput{ProjectID: p.ID})
	if err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, chatContextKey{}, chat.Task.ID)
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		var req struct {
			Messages []llm.Message `json:"messages"`
			Tools    []llm.Tool    `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			if len(req.Tools) != 4 {
				t.Errorf("missing tools: %+v", req.Tools)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"r","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"broken_test.go\"}"}},{"id":"c","type":"function","function":{"name":"run_check","arguments":"{\"command\":\"go test ./...\"}"}}]}}]}`))
			return
		}
		found := false
		for _, m := range req.Messages {
			if m.Role == "tool" && m.ToolCallID == "c" {
				var result toolruntime.Result
				_ = json.Unmarshal([]byte(m.Content), &result)
				found = result.Status == "failed" && strings.Contains(result.Output, "fixture diagnostic failure")
			}
		}
		if !found {
			t.Error("model did not receive actual failure")
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"Проверила: TestBroken падает из-за t.Fatal в broken_test.go:3. Код не изменяла."}}]}`))
	}))
	defer server.Close()
	model := llm.ModelConfig{ID: "model", ModelName: "test", BaseURL: server.URL}
	provider := openaiapi.NewClient(server.URL, "")
	req := llm.Request{Model: "test", Messages: []llm.Message{{Role: "user", Content: "разберись, почему падают тесты"}}}
	for _, consent := range []string{"", "other-model"} {
		resp, err := s.generateProjectAnswer(ctx, p, chat.Task.ID, provider, model, req, consent)
		if err != nil || count != 0 || !strings.Contains(resp.Content, "Разреши") {
			t.Fatalf("missing consent still executed: %+v %v", resp, err)
		}
	}
	resp, err := s.generateProjectAnswer(ctx, p, chat.Task.ID, provider, model, req, model.ID)
	if err != nil || count != 2 || !strings.Contains(resp.Content, "TestBroken") {
		t.Fatalf("%+v %v calls %d", resp, err, count)
	}
	calls, err := s.store.ListToolInvocations(ctx, chat.Task.ID)
	if err != nil || len(calls) != 2 {
		t.Fatal(calls, err)
	}
	for _, call := range calls {
		if call.AgentID != "agent_dev_developer" || call.FinishedAt == "" {
			t.Fatalf("wrong delegate: %+v", call)
		}
	}
	state, err := s.SelectChat(context.Background(), other.Task.ID)
	if err != nil || len(state.ToolInvocations) != 0 {
		t.Fatal("other chat leaked tools", err)
	}
	state, err = s.SelectChat(context.Background(), chat.Task.ID)
	if err != nil || len(state.ToolInvocations) != 2 {
		t.Fatal("audit did not reload", err)
	}
	if run, _ := s.store.LatestWorkflowRun(ctx, chat.Task.ID); run != nil {
		t.Fatal("diagnostic started workflow")
	}
}
