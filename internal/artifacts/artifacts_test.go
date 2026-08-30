package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteWorkflowOutputCreatesTaskSpecAndRunFiles(t *testing.T) {
	projectPath := t.TempDir()
	files, err := WriteWorkflowOutput(WorkflowOutput{
		ProjectPath:   projectPath,
		TaskTitle:     "Проверить локальную LLM",
		WorkflowRunID: "workflow_test",
		Intake:        `{"summary":"Проверить локальную LLM","goal":"Создать проверку доступности локальной модели","constraints":["Go","без внешних зависимостей"]}`,
		Product:       "## Функциональные требования\n\n- Проверить сетевую доступность.\n- Вывести результат в консоль.\n\n## Критерии готовности\n\n- Проверка запускается командой `go test ./...`.",
		Blueprint:     `{"stack":"go","runtime":"Go 1.21+","project_type":"cli_app","scaffold_required":false,"entrypoints":["main.go"],"expected_files":[{"path":"check.go","action":"replace","purpose":"логика проверки"}],"forbidden_files":[".zavod/**"],"dependencies":{"policy":"stdlib","items":[]},"test_commands":[{"command":"go test ./...","working_dir":".","reason":"проверка Go-кода"}],"confidence":"high"}`,
		Architect:     "Architecture",
		Developer:     "Developer plan",
		Final:         "Summary",
		CreatedAt:     time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("write workflow output: %v", err)
	}
	if len(files) != 7 {
		t.Fatalf("expected 7 files, got %d", len(files))
	}

	specPath := filepath.Join(projectPath, "docs", "task-spec.md")
	content, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read task spec: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "# Проверить локальную LLM") ||
		!strings.Contains(text, "## Технический контракт") ||
		!strings.Contains(text, "`check.go`") ||
		strings.Contains(text, "## План разработки") ||
		strings.Contains(text, "{\"summary\"") {
		t.Fatalf("unexpected task spec content: %s", text)
	}

	runPath := filepath.Join(projectPath, ".zavod", "runs", "workflow_test", "05-developer-plan.md")
	if _, err := os.Stat(runPath); err != nil {
		t.Fatalf("expected run artifact: %v", err)
	}
}

func TestWriteWorkflowOutputRejectsPathTraversal(t *testing.T) {
	_, err := safeJoin(t.TempDir(), filepath.Join("..", "outside.md"))
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}
}
