package openaiapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseStreamLine(t *testing.T) {
	event, ok := parseStreamLine(`data: {"choices":[{"delta":{"content":"Привет"}}]}`)
	if !ok {
		t.Fatal("expected stream event")
	}
	if event.Delta != "Привет" {
		t.Fatalf("expected delta, got %q", event.Delta)
	}
	if event.Done {
		t.Fatal("delta event must not be done")
	}
}

func TestParseStreamLineDone(t *testing.T) {
	event, ok := parseStreamLine("data: [DONE]")
	if !ok {
		t.Fatal("expected done event")
	}
	if !event.Done {
		t.Fatal("expected done")
	}
}

func TestNormalizeStreamDeltaHandlesCumulativeChunks(t *testing.T) {
	accumulated, delta, ok := normalizeStreamDelta("", "При")
	if !ok || delta != "При" || accumulated != "При" {
		t.Fatalf("unexpected first chunk: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}

	accumulated, delta, ok = normalizeStreamDelta(accumulated, "Привет")
	if !ok || delta != "вет" || accumulated != "Привет" {
		t.Fatalf("unexpected cumulative chunk: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}

	accumulated, delta, ok = normalizeStreamDelta(accumulated, "Привет!")
	if !ok || delta != "!" || accumulated != "Привет!" {
		t.Fatalf("unexpected cumulative suffix: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}
}

func TestNormalizeStreamDeltaKeepsRegularDeltaChunks(t *testing.T) {
	accumulated, delta, ok := normalizeStreamDelta("При", "вет")
	if !ok || delta != "вет" || accumulated != "Привет" {
		t.Fatalf("unexpected regular chunk: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}
}

func TestCheckUsesModelsEndpoint(t *testing.T) {
	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen3:8b"}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", "")
	result := client.Check(context.Background(), "qwen3:8b")
	if result.Status != "online" {
		t.Fatalf("expected online, got %#v", result)
	}
	if requestedPath != "/v1/models" {
		t.Fatalf("expected /v1/models, got %q", requestedPath)
	}
}

func TestCheckTreatsReachableModelsEndpointAsOnline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"other"}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL+"/v1", "")
	result := client.Check(context.Background(), "qwen3:8b")
	if result.Status != "online" {
		t.Fatalf("expected online, got %#v", result)
	}
	if result.LastError != "" {
		t.Fatalf("expected no model-name error, got %#v", result)
	}
}
