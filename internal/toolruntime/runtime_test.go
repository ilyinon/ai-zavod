package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zavod_ai/internal/llm"
)

type testProvider struct {
	generate func(context.Context, llm.Request) (*llm.Response, error)
	noTools  bool
}

func (p testProvider) Generate(c context.Context, r llm.Request) (*llm.Response, error) {
	return p.generate(c, r)
}
func (p testProvider) Stream(context.Context, llm.Request) (<-chan llm.Event, error) {
	return nil, errors.New("unused")
}
func (p testProvider) Capabilities() llm.Capabilities { return llm.Capabilities{Tools: !p.noTools} }
func call(id, name, args string) llm.ToolCall {
	return llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: name, Arguments: args}}
}
func fixture(t *testing.T) Scope {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{"main.go": "package main\n// diagnostic needle\n", ".env": "SECRET=private", "go.mod": "module fixture\n\ngo 1.25\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return Scope{ProjectID: "p", TaskID: "t", AgentID: "a", WorkingDir: root, ToolProfileID: "tool_go_dev", AllowedTools: []string{"list_files", "read_file", "search_files", "run_check"}, ReadPaths: []string{"**", "!private/**"}}
}

func TestLoopUsesRealFilesAndCorrelatedResults(t *testing.T) {
	scope := fixture(t)
	var records []Invocation
	r := Runtime{Scope: scope, Record: func(_ context.Context, i Invocation) error { records = append(records, i); return nil }}
	round := 0
	p := testProvider{generate: func(_ context.Context, req llm.Request) (*llm.Response, error) {
		round++
		if round == 1 {
			if len(req.Tools) != 4 {
				t.Fatal("missing schemas")
			}
			return &llm.Response{ToolCalls: []llm.ToolCall{call("list", "list_files", `{}`), call("read", "read_file", `{"path":"main.go"}`), call("search", "search_files", `{"query":"needle"}`)}, InputTokens: 3}, nil
		}
		got := map[string]Result{}
		for _, m := range req.Messages {
			if m.Role == "tool" {
				var result Result
				if err := json.Unmarshal([]byte(m.Content), &result); err != nil {
					t.Fatal(err)
				}
				got[m.ToolCallID] = result
			}
		}
		if strings.Contains(got["list"].Output, ".env") || !strings.Contains(got["read"].Output, "2: // diagnostic needle") || !strings.Contains(got["search"].Output, "main.go:2") {
			t.Fatalf("bad evidence: %+v", got)
		}
		return &llm.Response{Content: "Found diagnostic needle in main.go:2", InputTokens: 5}, nil
	}}
	resp, err := r.Generate(context.Background(), p, llm.Request{})
	if err != nil || resp.InputTokens != 8 || len(records) != 6 {
		t.Fatalf("resp=%+v err=%v records=%d", resp, err, len(records))
	}
	for i, rec := range records {
		if rec.TaskID != "t" || rec.AgentID != "a" || rec.LoopID == "" || (i%2 == 0 && rec.Result.Status != "running") || (i%2 == 1 && rec.FinishedAt == "") {
			t.Fatalf("audit: %+v", rec)
		}
	}
}

func TestToolsDenyEscapesSecretsInvalidArgsAndCommands(t *testing.T) {
	scope := fixture(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(scope.WorkingDir, "outside")); err != nil {
		t.Fatal(err)
	}
	w, err := openWorkspace(scope)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for _, tc := range []llm.ToolCall{
		call("1", "read_file", `{"path":"../other"}`), call("1", "read_file", `{"path":"/etc/passwd"}`),
		call("1", "read_file", `{"path":".env"}`), call("1", "list_files", `{"path":"outside"}`),
		call("1", "read_file", `{"path":"main.go","command":"go test ./..."}`),
		call("1", "read_file", `{"path":"main.go","offset":-1}`), call("1", "read_file", `null`),
		call("1", "run_check", `{"command":"go test ./...; touch hacked"}`), call("1", "run_check", `{"command":"go mod tidy"}`),
		call("1", "unknown", `{}`),
	} {
		if got := w.execute(context.Background(), tc, 1024); got.Status != "blocked" {
			t.Errorf("%+v: %+v", tc, got)
		}
	}
	w.scope.AllowedTools = []string{"list_files"}
	if got := w.execute(context.Background(), call("1", "read_file", `{"path":"main.go"}`), 1024); got.Status != "blocked" {
		t.Fatal(got)
	}
	for _, p := range []string{"private/a.go", "private/nested/a.go"} {
		if permitted(p, scope.ReadPaths) {
			t.Fatal("deny ignored", p)
		}
	}
	if !permitted("internal/a/b.go", []string{"internal/**/*.go"}) {
		t.Fatal("recursive glob broken")
	}
}

func TestActualFailingGoCheck(t *testing.T) {
	scope := fixture(t)
	if err := os.WriteFile(filepath.Join(scope.WorkingDir, "main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestBroken(t *testing.T) { t.Fatal(\"actual failure marker\") }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w, err := openWorkspace(scope)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	result := w.execute(context.Background(), call("1", "run_check", `{"command":"go test ./..."}`), 12000)
	if result.Status != "failed" || result.ExitCode == nil || *result.ExitCode == 0 || !strings.Contains(result.Output, "actual failure marker") {
		t.Fatalf("not real test output: %+v", result)
	}
	w.scope.ToolProfileID = "tool_research"
	if result := w.execute(context.Background(), call("2", "run_check", `{"command":"go test ./..."}`), 12000); result.Status != "blocked" {
		t.Fatal(result)
	}
}

func TestLoopLimitsAndAuditFailure(t *testing.T) {
	scope := fixture(t)
	scope.ReadPaths = []string{"main.go"}
	count := 0
	records := 0
	p := testProvider{generate: func(_ context.Context, req llm.Request) (*llm.Response, error) {
		count++
		if count == 1 {
			return &llm.Response{ToolCalls: []llm.ToolCall{call("1", "read_file", `{"path":"main.go"}`)}}, nil
		}
		if len(req.Tools) != 0 {
			t.Fatal("tools retained after limit")
		}
		return &llm.Response{Content: "Evidence only"}, nil
	}}
	r := Runtime{Scope: scope, Limits: Limits{Calls: 1, Duration: time.Second, OutputBytes: 256, TotalOutputBytes: 256}, Record: func(_ context.Context, i Invocation) error { records++; return nil }}
	if _, err := r.Generate(context.Background(), p, llm.Request{}); err != nil || records != 2 {
		t.Fatalf("%v %d", err, records)
	}
	count = 0
	r.Record = func(context.Context, Invocation) error { return errors.New("database unavailable") }
	if _, err := r.Generate(context.Background(), p, llm.Request{}); err == nil || count != 1 {
		t.Fatalf("did not stop on audit failure: %v", err)
	}
	count = 0
	p.noTools = true
	if _, err := r.Generate(context.Background(), p, llm.Request{}); err == nil || count != 0 {
		t.Fatal("unsupported provider called")
	}
}

func TestDuplicateCallsNeverExecute(t *testing.T) {
	count := 0
	r := Runtime{Scope: fixture(t), Record: func(context.Context, Invocation) error { count++; return nil }}
	p := testProvider{generate: func(context.Context, llm.Request) (*llm.Response, error) {
		return &llm.Response{ToolCalls: []llm.ToolCall{call("1", "list_files", `{}`), call("1", "list_files", `{}`)}}, nil
	}}
	if _, err := r.Generate(context.Background(), p, llm.Request{}); err == nil || count != 0 {
		t.Fatal("duplicate batch executed")
	}
}

func TestCancellationPersistsTerminalResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var final Invocation
	r := Runtime{Scope: fixture(t), Record: func(ctx context.Context, i Invocation) error {
		if i.Result.Status == "running" {
			cancel()
		} else {
			if ctx.Err() != nil {
				t.Fatal("cancelled persistence context")
			}
			final = i
		}
		return nil
	}}
	p := testProvider{generate: func(context.Context, llm.Request) (*llm.Response, error) {
		return &llm.Response{ToolCalls: []llm.ToolCall{call("1", "list_files", `{}`)}}, nil
	}}
	if _, err := r.Generate(ctx, p, llm.Request{}); !errors.Is(err, context.Canceled) || final.Result.Status != "cancelled" {
		t.Fatalf("%v %+v", err, final)
	}
}
