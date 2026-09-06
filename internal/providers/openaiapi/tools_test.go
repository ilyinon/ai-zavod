package openaiapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"zavod_ai/internal/llm"
)

func TestGenerateToolOnlyResponseAndMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if len(req.Tools) != 1 || req.Messages[1].ToolCalls[0].ID != "call1" || req.Messages[2].ToolCallID != "call1" || req.Messages[2].Role != "tool" {
			t.Errorf("lost tool protocol: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call2","type":"function","function":{"name":"list_files","arguments":"{}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "")
	req := llm.Request{Tools: []llm.Tool{{Type: "function", Function: llm.ToolFunction{Name: "list_files"}}}, Messages: []llm.Message{{Role: "user", Content: "inspect"}, {Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call1", Type: "function", Function: llm.FunctionCall{Name: "list_files", Arguments: "{}"}}}}, {Role: "tool", ToolCallID: "call1", Content: "[]"}}}
	req.Model = "test-model"
	resp, err := client.Generate(context.Background(), req)
	if err != nil || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].ID != "call2" || resp.TotalTokens != 15 {
		t.Fatalf("%+v %v", resp, err)
	}
	if _, err := client.Stream(context.Background(), req); err == nil {
		t.Fatal("stream silently discarded tool calls")
	}
}
