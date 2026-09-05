package checks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateCommandAllowsExpectedCommands(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectPath, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(projectPath, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "go.mod"), []byte("module example.com/app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "frontend", "package.json"), []byte(`{"scripts":{"build":"vite","test":"vitest","lint":"eslint ."}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "check.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "requirements.txt"), []byte("# standard library only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "scripts", "check.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		command    string
		workingDir string
	}{
		{command: "go test ./..."},
		{command: "go test ./internal/app"},
		{command: "go vet ./..."},
		{command: "npm test", workingDir: "frontend"},
		{command: "npm run build", workingDir: "frontend"},
		{command: "npm run lint", workingDir: "frontend"},
		{command: ".venv/bin/python check.py"},
		{command: ".venv/bin/python scripts/check.py"},
		{command: ".venv/bin/python -m py_compile scripts/check.py"},
	}

	for _, tc := range cases {
		if err := ValidateCommand(projectPath, tc.command, tc.workingDir); err != nil {
			t.Fatalf("expected %q to be allowed: %v", tc.command, err)
		}
	}
}

func TestValidateCommandBlocksUnsafeCommands(t *testing.T) {
	projectPath := t.TempDir()
	cases := []struct {
		command    string
		workingDir string
	}{
		{command: "rm -rf ."},
		{command: "npm install"},
		{command: "go get ./..."},
		{command: "go test ./... && rm -rf ."},
		{command: "npm run build | cat"},
		{command: "npm run build", workingDir: "../other"},
		{command: "python -c print(1)"},
		{command: "python check.py"},
		{command: "python3 check.py"},
		{command: "python ../check.py"},
		{command: "python check.py --verbose"},
	}

	for _, tc := range cases {
		if err := ValidateCommand(projectPath, tc.command, tc.workingDir); err == nil {
			t.Fatalf("expected %q to be blocked", tc.command)
		}
	}
}

func TestValidateCommandBlocksUnsupportedProjectCommands(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "check.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		"go test ./...",
		"go vet ./...",
		"npm run build",
		"python missing.py",
		".venv/bin/python check.py",
	}
	for _, command := range cases {
		if err := ValidateCommand(projectPath, command, ""); err == nil {
			t.Fatalf("expected %q to be blocked for unsupported project", command)
		}
	}
}

func TestDefaultSuggestionsUsesPythonProject(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "check.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "requirements.txt"), []byte("# standard library only\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := DefaultSuggestions(projectPath)
	if len(got) != 1 {
		t.Fatalf("expected one Python suggestion, got %#v", got)
	}
	if got[0].Command != ".venv/bin/python check.py" {
		t.Fatalf("expected .venv/bin/python check.py, got %#v", got[0])
	}
}

func TestFilterSupportedSuggestionsRemovesUnsupported(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "check.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "requirements.txt"), []byte("# standard library only\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := FilterSupportedSuggestions(projectPath, []Suggestion{
		{Command: "go test ./..."},
		{Command: "python3 check.py"},
	})
	if len(got) != 1 || got[0].Command != ".venv/bin/python check.py" {
		t.Fatalf("expected only supported Python suggestion, got %#v", got)
	}
}

func TestDefaultSuggestionsUsesPytestWhenProjectHasTests(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectPath, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "requirements.txt"), []byte("pytest\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := DefaultSuggestions(projectPath)
	if len(got) != 1 || got[0].Command != ".venv/bin/python -m pytest" {
		t.Fatalf("expected pytest suggestion, got %#v", got)
	}
}

func TestDefaultSuggestionsUsesPyCompileFallback(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectPath, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "requirements.txt"), []byte("# standard library only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "src", "tool.py"), []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := DefaultSuggestions(projectPath)
	if len(got) != 1 || got[0].Command != ".venv/bin/python -m py_compile src/tool.py" {
		t.Fatalf("expected py_compile suggestion, got %#v", got)
	}
}

func TestRunPythonCreatesAndUsesVirtualenv(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not available in PATH")
	}
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "requirements.txt"), []byte("# standard library only\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "check.py"), []byte("import sys\nprint(sys.prefix)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := Run(context.Background(), projectPath, ".venv/bin/python check.py", "")
	if result.Status != StatusPassed {
		t.Fatalf("expected passed venv run, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(projectPath, ".venv", "bin", "python")); err != nil {
		t.Fatalf("expected venv python to be created: %v", err)
	}
	if !strings.Contains(result.Stdout, ".venv") {
		t.Fatalf("expected script to run inside venv, stdout: %q", result.Stdout)
	}
}

func TestExtractSuggestionsParsesJSON(t *testing.T) {
	got := ExtractSuggestions(`{
		"summary": "ok",
		"commands": [
			{"command":"go test ./...","reason":"backend"},
			{"command":"npm run build","working_dir":"frontend","reason":"ui"}
		]
	}`)
	if len(got) != 2 {
		t.Fatalf("expected 2 suggestions, got %#v", got)
	}
	if got[1].WorkingDir != "frontend" {
		t.Fatalf("expected frontend working dir, got %#v", got[1])
	}
}

func TestRunBlocksUnsafeCommand(t *testing.T) {
	result := Run(context.Background(), t.TempDir(), "rm -rf .", "")
	if result.Status != StatusBlocked {
		t.Fatalf("expected blocked, got %#v", result)
	}
}

func TestNormalizeSuggestionsForcesPythonVenv(t *testing.T) {
	got := ExtractSuggestions(`{
		"summary": "python",
		"commands": [
			{"command":"python3 check.py","reason":"script"},
			{"command":"python -m pytest","reason":"tests"},
			{"command":"python -m py_compile src/tool.py","reason":"syntax"}
		]
	}`)
	want := []string{
		".venv/bin/python check.py",
		".venv/bin/python -m pytest",
		".venv/bin/python -m py_compile src/tool.py",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d suggestions, got %#v", len(want), got)
	}
	for index, command := range want {
		if got[index].Command != command {
			t.Fatalf("suggestion %d = %q, want %q", index, got[index].Command, command)
		}
	}
}
