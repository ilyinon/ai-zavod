package blueprint

import (
	"strings"
	"testing"
)

func TestParseNormalizesGoRuntime(t *testing.T) {
	got, err := Parse(`{
		"stack": "go",
		"runtime": "Go 1.21+",
		"project_type": "cli_app",
		"scaffold_required": true,
		"entrypoints": ["main.go"],
		"expected_files": [],
		"forbidden_files": [".zavod/**"],
		"dependencies": {"policy": "stdlib", "items": []},
		"test_commands": [],
		"open_questions": [],
		"confidence": "high"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != "Go 1.25+" {
		t.Fatalf("expected Go 1.25+ runtime, got %q", got.Runtime)
	}
}

func TestParseUsesGoRuntimeFallback(t *testing.T) {
	got, err := Parse(`{
		"stack": "go",
		"project_type": "cli_app",
		"scaffold_required": true,
		"entrypoints": ["main.go"],
		"expected_files": [],
		"forbidden_files": [".zavod/**"],
		"dependencies": {"policy": "stdlib", "items": []},
		"test_commands": [],
		"open_questions": [],
		"confidence": "high"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != "Go 1.25+" {
		t.Fatalf("expected Go 1.25+ runtime fallback, got %q", got.Runtime)
	}
}

func TestNormalizeForProjectAddsPythonRequirementsAndVenvCommand(t *testing.T) {
	got, err := Parse(`{
		"stack": "python",
		"runtime": "Python 3",
		"project_type": "telegram_bot",
		"scaffold_required": false,
		"entrypoints": ["bot.py"],
		"expected_files": [
			{"path": "bot.py", "action": "create", "purpose": "Telegram bot entrypoint"}
		],
		"forbidden_files": ["go.mod", "requirements.txt"],
		"dependencies": {"policy": "external", "items": ["python-telegram-bot"]},
		"test_commands": [
			{"command": "python3 bot.py", "reason": "smoke"}
		],
		"open_questions": [],
		"confidence": "high"
	}`)
	if err != nil {
		t.Fatal(err)
	}

	got = NormalizeForProject(got, t.TempDir())
	if got.Runtime != "Python 3 + venv" {
		t.Fatalf("expected Python venv runtime, got %q", got.Runtime)
	}
	if !hasExpectedFile(got.ExpectedFiles, "requirements.txt") {
		t.Fatalf("expected requirements.txt in expected files: %#v", got.ExpectedFiles)
	}
	if hasForbiddenFile(got.ForbiddenFiles, "requirements.txt") {
		t.Fatalf("requirements.txt should not remain forbidden: %#v", got.ForbiddenFiles)
	}
	if len(got.TestCommands) != 1 || got.TestCommands[0].Command != ".venv/bin/python bot.py" {
		t.Fatalf("expected venv Python command, got %#v", got.TestCommands)
	}
	if !strings.Contains(got.RawJSON, "requirements.txt") || !strings.Contains(got.RawJSON, ".venv/bin/python bot.py") {
		t.Fatalf("expected normalized raw json, got %s", got.RawJSON)
	}
}

func hasExpectedFile(items []ExpectedFile, path string) bool {
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}

func hasForbiddenFile(items []string, path string) bool {
	for _, item := range items {
		if item == path {
			return true
		}
	}
	return false
}
