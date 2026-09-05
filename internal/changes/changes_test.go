package changes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDraftsFromJSONFence(t *testing.T) {
	text := "## Proposed changes\n```json\n[{\"file_path\":\"check_llm.py\",\"action\":\"create\",\"reason\":\"test\",\"content\":\"print(1)\\n\"}]\n```"
	drafts := ExtractDrafts(text)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(drafts))
	}
	if drafts[0].FilePath != "check_llm.py" || drafts[0].Action != ActionCreate {
		t.Fatalf("unexpected draft: %#v", drafts[0])
	}
}

func TestExtractDraftsNormalizesDoubleEscapedContent(t *testing.T) {
	text := `## Proposed changes
[
  {
    "file_path": "password_generator.py",
    "action": "create",
    "content": "import argparse\\nimport secrets\\n\\ndef main():\\n    print('ok')\\n"
  }
]`
	drafts := ExtractDrafts(text)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %#v", drafts)
	}
	expected := "import argparse\nimport secrets\n\ndef main():\n    print('ok')\n"
	if drafts[0].Content != expected {
		t.Fatalf("expected content to be unescaped:\n%q", drafts[0].Content)
	}
}

func TestExtractDraftsConvertsUnifiedDiffContent(t *testing.T) {
	text := `## Proposed changes
[
  {
    "file_path": "password_generator.py",
    "action": "create",
    "content": "--- /dev/null\n+++ password_generator.py\n@@\n+import argparse\\nimport secrets\\n\\ndef main():\\n    print('ok')\\n"
  }
]`
	drafts := ExtractDrafts(text)
	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %#v", drafts)
	}
	expected := "import argparse\nimport secrets\n\ndef main():\n    print('ok')\n"
	if drafts[0].Content != expected {
		t.Fatalf("expected diff content to become file content:\n%q", drafts[0].Content)
	}
}

func TestExtractDraftsWithErrorReportsMalformedProposedChanges(t *testing.T) {
	text := "## Proposed changes\n[{\"file_path\":\"main.go\",\"action\":\"create\",\"content\":\"if value == \"\" {\"}]"
	drafts, err := ExtractDraftsWithError(text)
	if err == nil {
		t.Fatal("expected malformed proposed changes error")
	}
	if len(drafts) != 0 {
		t.Fatalf("expected no drafts, got %#v", drafts)
	}
	if !strings.Contains(err.Error(), "не удалось разобрать ## Proposed changes") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractDraftsWithErrorAllowsNoProposedChangesSection(t *testing.T) {
	drafts, err := ExtractDraftsWithError("## Developer summary\nНет применимых изменений.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(drafts) != 0 {
		t.Fatalf("expected no drafts, got %#v", drafts)
	}
}

func TestValidateRelativePathRejectsUnsafePaths(t *testing.T) {
	paths := []string{
		"/tmp/file.txt",
		"../file.txt",
		filepath.Join(".git", "config"),
		filepath.Join(".zavod", "runs", "x.md"),
		"zavod.db",
	}
	for _, path := range paths {
		if _, err := ValidateRelativePath(path); err == nil {
			t.Fatalf("expected %s to be rejected", path)
		}
	}
}

func TestApplyCreateAndReplace(t *testing.T) {
	projectPath := t.TempDir()
	create := ProposedChange{
		ID:       "change_create",
		FilePath: "cmd/check_llm.py",
		Action:   ActionCreate,
		Content:  "print('ok')\n",
	}
	createResult, err := Apply(projectPath, create)
	if err != nil {
		t.Fatalf("apply create: %v", err)
	}
	if createResult.BeforeContent != "" || createResult.AfterContent != create.Content {
		t.Fatalf("unexpected create result: %#v", createResult)
	}
	if !strings.Contains(createResult.DiffText, "--- /dev/null") || !strings.Contains(createResult.DiffText, "+print('ok')") {
		t.Fatalf("unexpected create diff: %s", createResult.DiffText)
	}
	createdPath := filepath.Join(projectPath, "cmd", "check_llm.py")
	content, err := os.ReadFile(createdPath)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if string(content) != create.Content {
		t.Fatalf("unexpected created content: %s", content)
	}
	noopResult, err := Apply(projectPath, create)
	if err != nil {
		t.Fatalf("apply identical create as no-op: %v", err)
	}
	if noopResult.BeforeContent != create.Content || noopResult.AfterContent != create.Content || noopResult.DiffText != "" {
		t.Fatalf("unexpected noop result: %#v", noopResult)
	}
	conflictingCreate := create
	conflictingCreate.Content = "print('conflict')\n"
	if _, err := Apply(projectPath, conflictingCreate); err == nil {
		t.Fatal("expected create to reject existing file with different content")
	}

	replace := ProposedChange{
		ID:       "change_replace",
		FilePath: "cmd/check_llm.py",
		Action:   ActionReplace,
		Content:  "print('new')\n",
	}
	replaceResult, err := Apply(projectPath, replace)
	if err != nil {
		t.Fatalf("apply replace: %v", err)
	}
	if !strings.Contains(replaceResult.BackupPath, filepath.Join(".zavod", "backups", "change_replace")) {
		t.Fatalf("unexpected backup path: %s", replaceResult.BackupPath)
	}
	if replaceResult.BeforeContent != create.Content || replaceResult.AfterContent != replace.Content {
		t.Fatalf("unexpected replace result: %#v", replaceResult)
	}
	if !strings.Contains(replaceResult.DiffText, "-print('ok')") || !strings.Contains(replaceResult.DiffText, "+print('new')") {
		t.Fatalf("unexpected replace diff: %s", replaceResult.DiffText)
	}
	replaced, err := os.ReadFile(createdPath)
	if err != nil {
		t.Fatalf("read replaced file: %v", err)
	}
	if string(replaced) != replace.Content {
		t.Fatalf("unexpected replaced content: %s", replaced)
	}
	backupContent, err := os.ReadFile(filepath.Join(projectPath, replaceResult.BackupPath))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupContent) != create.Content {
		t.Fatalf("unexpected backup content: %s", backupContent)
	}
}

func TestRollbackCreateAndReplace(t *testing.T) {
	projectPath := t.TempDir()
	createdPath := filepath.Join(projectPath, "tool.py")
	createChange := ProposedChange{
		ID:            "change_create",
		FilePath:      "tool.py",
		Action:        ActionCreate,
		Content:       "print('new')\n",
		Status:        StatusApplied,
		BeforeContent: "",
		AfterContent:  "print('new')\n",
	}
	if err := os.WriteFile(createdPath, []byte(createChange.AfterContent), 0o644); err != nil {
		t.Fatal(err)
	}
	createRollback, err := Rollback(projectPath, createChange)
	if err != nil {
		t.Fatalf("rollback create: %v", err)
	}
	if _, err := os.Stat(createdPath); !os.IsNotExist(err) {
		t.Fatalf("expected created file to be removed, err=%v", err)
	}
	if !strings.Contains(createRollback.DiffText, "-print('new')") {
		t.Fatalf("expected removal diff, got %s", createRollback.DiffText)
	}

	if err := os.WriteFile(createdPath, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	replaceChange := ProposedChange{
		ID:            "change_replace",
		FilePath:      "tool.py",
		Action:        ActionReplace,
		Status:        StatusApplied,
		BeforeContent: "before\n",
		AfterContent:  "after\n",
	}
	if err := os.WriteFile(createdPath, []byte(replaceChange.AfterContent), 0o644); err != nil {
		t.Fatal(err)
	}
	replaceRollback, err := Rollback(projectPath, replaceChange)
	if err != nil {
		t.Fatalf("rollback replace: %v", err)
	}
	restored, err := os.ReadFile(createdPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != replaceChange.BeforeContent {
		t.Fatalf("expected restored content, got %q", restored)
	}
	if !strings.Contains(replaceRollback.DiffText, "-after") || !strings.Contains(replaceRollback.DiffText, "+before") {
		t.Fatalf("expected rollback diff, got %s", replaceRollback.DiffText)
	}
}

func TestRollbackRejectsChangedFile(t *testing.T) {
	projectPath := t.TempDir()
	path := filepath.Join(projectPath, "main.go")
	change := ProposedChange{
		ID:            "change_replace",
		FilePath:      "main.go",
		Action:        ActionReplace,
		Status:        StatusApplied,
		BeforeContent: "package main\n",
		AfterContent:  "package main\n\nfunc main() {}\n",
	}
	if err := os.WriteFile(path, []byte("manual edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Rollback(projectPath, change); err == nil {
		t.Fatal("expected rollback to reject file changed after apply")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "manual edit\n" {
		t.Fatalf("manual edit should stay intact, got %q", content)
	}
}

func TestGenerateUnifiedDiffKeepsContextLines(t *testing.T) {
	diff := GenerateUnifiedDiff("app.txt", "one\ntwo\nthree\n", "one\nTWO\nthree\n")
	for _, expected := range []string{"--- a/app.txt", "+++ b/app.txt", " one", "-two", "+TWO", " three"} {
		if !strings.Contains(diff, expected) {
			t.Fatalf("expected diff to contain %q, got:\n%s", expected, diff)
		}
	}
}
