package devworkspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
)

func TestNormalizeBlueprintAddsGoWorkspaceContract(t *testing.T) {
	projectPath := t.TempDir()
	got := NormalizeBlueprint(blueprint.Blueprint{
		Stack:       blueprint.StackGo,
		Runtime:     "Go 1.21+",
		ProjectType: "cli_app",
		Entrypoints: []string{"main.go"},
		ExpectedFiles: []blueprint.ExpectedFile{
			{Path: "main.go", Action: changes.ActionCreate, Purpose: "CLI entrypoint"},
		},
		ForbiddenFiles: []string{"go.mod"},
		Dependencies:   blueprint.DependencyPolicy{Policy: "stdlib"},
	}, projectPath)

	if got.Runtime != "Go 1.25+" {
		t.Fatalf("expected Go 1.25+, got %q", got.Runtime)
	}
	for _, path := range []string{"go.mod", ".gitignore", "Makefile", "README.md", ciPath} {
		if !hasExpectedFile(got.ExpectedFiles, path) {
			t.Fatalf("expected %s in workspace contract: %#v", path, got.ExpectedFiles)
		}
	}
	if !hasTestCommand(got.TestCommands, "go test ./...") || !hasTestCommand(got.TestCommands, "go vet ./...") {
		t.Fatalf("expected Go checks, got %#v", got.TestCommands)
	}
	if strings.Contains(got.RawJSON, "Go 1.21+") || !strings.Contains(got.RawJSON, "go vet ./...") {
		t.Fatalf("expected refreshed raw json, got %s", got.RawJSON)
	}
}

func TestEnsureDraftsAddsPythonWorkspaceFiles(t *testing.T) {
	projectPath := t.TempDir()
	taskBlueprint := NormalizeBlueprint(blueprint.Blueprint{
		Stack:       blueprint.StackPython,
		Runtime:     "Python 3",
		ProjectType: "cli_app",
		Entrypoints: []string{"main.py"},
		ExpectedFiles: []blueprint.ExpectedFile{
			{Path: "main.py", Action: changes.ActionCreate, Purpose: "CLI entrypoint"},
		},
		Dependencies: blueprint.DependencyPolicy{Policy: "external", Items: []string{"requests", "pytest"}},
	}, projectPath)

	got := EnsureDrafts(projectPath, taskBlueprint, []changes.Draft{
		{FilePath: "main.py", Action: changes.ActionCreate, Content: "print('ok')\n"},
	})

	for _, path := range []string{"requirements.txt", ".gitignore", "Makefile", "README.md", ciPath} {
		if !hasDraft(got, path) {
			t.Fatalf("expected %s draft, got %#v", path, got)
		}
	}
	requirements := draftContent(got, "requirements.txt")
	if !strings.Contains(requirements, "pytest") || !strings.Contains(requirements, "requests") {
		t.Fatalf("expected requirements from dependencies, got %q", requirements)
	}
	if !strings.Contains(draftContent(got, "Makefile"), ".venv/bin/python -m py_compile main.py") {
		t.Fatalf("expected venv Makefile, got %s", draftContent(got, "Makefile"))
	}
}

func TestEnsureDraftsMergesExistingRequirements(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "requirements.txt"), []byte("click\n# local note\nrequests\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	taskBlueprint := NormalizeBlueprint(blueprint.Blueprint{
		Stack:        blueprint.StackPython,
		Entrypoints:  []string{"main.py"},
		Dependencies: blueprint.DependencyPolicy{Policy: "external", Items: []string{"python-telegram-bot", "requests"}},
	}, projectPath)

	got := EnsureDrafts(projectPath, taskBlueprint, nil)
	requirements := draftContent(got, "requirements.txt")
	for _, want := range []string{"click", "python-telegram-bot", "requests"} {
		if !strings.Contains(requirements, want) {
			t.Fatalf("expected merged dependency %q, got %q", want, requirements)
		}
	}
}

func TestEnsureDraftsUpgradesExistingGoModWithoutOverwritingReadme(t *testing.T) {
	projectPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectPath, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "README.md"), []byte("# Existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	taskBlueprint := NormalizeBlueprint(blueprint.Blueprint{
		Stack:       blueprint.StackGo,
		Entrypoints: []string{"main.go"},
	}, projectPath)

	got := EnsureDrafts(projectPath, taskBlueprint, nil)
	if !hasDraft(got, "go.mod") || !strings.Contains(draftContent(got, "go.mod"), "go 1.25") {
		t.Fatalf("expected upgraded go.mod draft, got %#v", got)
	}
	if hasDraft(got, "README.md") {
		t.Fatalf("existing README should not be overwritten: %#v", got)
	}
}

func hasExpectedFile(items []blueprint.ExpectedFile, path string) bool {
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}

func hasTestCommand(items []blueprint.TestCommand, command string) bool {
	for _, item := range items {
		if item.Command == command {
			return true
		}
	}
	return false
}

func hasDraft(items []changes.Draft, path string) bool {
	for _, item := range items {
		if item.FilePath == path {
			return true
		}
	}
	return false
}

func draftContent(items []changes.Draft, path string) string {
	for _, item := range items {
		if item.FilePath == path {
			return item.Content
		}
	}
	return ""
}
