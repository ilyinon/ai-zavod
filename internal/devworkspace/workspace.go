package devworkspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
)

const (
	ciPath = ".github/workflows/ci.yml"
)

type Profile struct {
	Stack      string
	HasGo      bool
	HasPython  bool
	Entrypoint string
}

func NormalizeBlueprint(value blueprint.Blueprint, projectPath string) blueprint.Blueprint {
	profile := ProfileFromBlueprint(value)
	if profile.HasGo {
		value.Runtime = "Go 1.25+"
		value.ExpectedFiles = ensureExpectedFile(value.ExpectedFiles, "go.mod", actionForProjectFile(projectPath, "go.mod"), "Go module with go 1.25+")
		value.TestCommands = ensureTestCommand(value.TestCommands, "go test ./...", "", "проверяет Go-проект")
		value.TestCommands = ensureTestCommand(value.TestCommands, "go vet ./...", "", "проверяет Go-код статически")
		value.ForbiddenFiles = removePath(value.ForbiddenFiles, "go.mod")
	}
	if profile.HasPython {
		value.Runtime = "Python 3 + .venv"
		value.ExpectedFiles = ensureExpectedFile(value.ExpectedFiles, "requirements.txt", actionForProjectFile(projectPath, "requirements.txt"), "Python dependencies for project virtualenv")
		value.TestCommands = ensurePythonCommands(value.TestCommands, value.Entrypoints, value.Dependencies.Items)
		value.ForbiddenFiles = removePath(value.ForbiddenFiles, "requirements.txt")
	}
	if profile.HasGo || profile.HasPython {
		value.ExpectedFiles = ensureExpectedFile(value.ExpectedFiles, ".gitignore", actionForProjectFile(projectPath, ".gitignore"), "Ignore build artifacts, caches and local env")
		value.ExpectedFiles = ensureExpectedFile(value.ExpectedFiles, "Makefile", actionForProjectFile(projectPath, "Makefile"), "Local dev commands for build/test/run")
		value.ExpectedFiles = ensureExpectedFile(value.ExpectedFiles, "README.md", actionForProjectFile(projectPath, "README.md"), "Project setup and development guide")
		value.ExpectedFiles = ensureExpectedFile(value.ExpectedFiles, ciPath, actionForProjectFile(projectPath, ciPath), "GitHub Actions CI for build and tests")
	}
	return blueprint.RefreshRawJSON(value)
}

func EnsureDrafts(projectPath string, taskBlueprint blueprint.Blueprint, drafts []changes.Draft) []changes.Draft {
	taskBlueprint = NormalizeBlueprint(taskBlueprint, projectPath)
	profile := ProfileFromBlueprint(taskBlueprint)
	out := append([]changes.Draft{}, drafts...)

	if profile.HasGo {
		out = ensureGoModDraft(projectPath, out)
	}
	if profile.HasPython {
		out = ensureRequirementsDraft(projectPath, out, taskBlueprint.Dependencies.Items)
	}
	if profile.HasGo || profile.HasPython {
		out = ensureFileDraft(projectPath, out, ".gitignore", gitignoreContent(profile), "dev workspace ignore rules")
		out = ensureFileDraft(projectPath, out, "Makefile", makefileContent(profile), "dev workspace commands")
		out = ensureFileDraft(projectPath, out, "README.md", readmeContent(profile), "project setup and development guide")
		out = ensureFileDraft(projectPath, out, ciPath, ciContent(profile), "GitHub Actions CI for dev workspace")
	}
	return out
}

func ProfileFromBlueprint(value blueprint.Blueprint) Profile {
	profile := Profile{Stack: value.Stack}
	switch value.Stack {
	case blueprint.StackGo:
		profile.HasGo = true
	case blueprint.StackPython:
		profile.HasPython = true
	case blueprint.StackMixed:
		profile.HasGo = hasGoSignals(value)
		profile.HasPython = hasPythonSignals(value)
	}
	for _, entrypoint := range value.Entrypoints {
		if profile.Entrypoint == "" {
			profile.Entrypoint = entrypoint
		}
		if strings.HasSuffix(entrypoint, ".go") {
			profile.HasGo = true
		}
		if strings.HasSuffix(entrypoint, ".py") {
			profile.HasPython = true
			if profile.Entrypoint == "" || !strings.HasSuffix(profile.Entrypoint, ".py") {
				profile.Entrypoint = entrypoint
			}
		}
	}
	for _, file := range value.ExpectedFiles {
		if strings.HasSuffix(file.Path, ".go") || filepath.Base(file.Path) == "go.mod" {
			profile.HasGo = true
		}
		if strings.HasSuffix(file.Path, ".py") || strings.HasPrefix(filepath.Base(file.Path), "requirements") {
			profile.HasPython = true
		}
	}
	return profile
}

func ensureGoModDraft(projectPath string, drafts []changes.Draft) []changes.Draft {
	if index, ok := draftIndexByPath(drafts, "go.mod"); ok {
		drafts[index].Content = EnsureGoModVersion(drafts[index].Content)
		return drafts
	}
	content := readProjectFile(projectPath, "go.mod")
	if strings.TrimSpace(content) == "" {
		content = fmt.Sprintf("module %s\n\ngo 1.25\n", moduleNameFromProject(projectPath))
	} else {
		next := EnsureGoModVersion(content)
		if next == content {
			return drafts
		}
		content = next
	}
	return append(drafts, changes.Draft{
		FilePath: "go.mod",
		Action:   actionForProjectFile(projectPath, "go.mod"),
		Content:  content,
		Reason:   "Go 1.25+ module for dev workspace",
	})
}

func ensureRequirementsDraft(projectPath string, drafts []changes.Draft, dependencies []string) []changes.Draft {
	requirements := append(requirementsFromContent(readProjectFile(projectPath, "requirements.txt")), dependencies...)
	content := PythonRequirementsContent(requirements)
	if index, ok := draftIndexByPath(drafts, "requirements.txt"); ok {
		drafts[index].Content = content
		if strings.TrimSpace(drafts[index].Reason) == "" {
			drafts[index].Reason = "Python dependencies for project virtualenv"
		}
		return drafts
	}
	return append(drafts, changes.Draft{
		FilePath: "requirements.txt",
		Action:   actionForProjectFile(projectPath, "requirements.txt"),
		Content:  content,
		Reason:   "Python dependencies for project virtualenv",
	})
}

func ensureFileDraft(projectPath string, drafts []changes.Draft, relativePath string, content string, reason string) []changes.Draft {
	if hasDraftPath(drafts, relativePath) || fileExists(filepath.Join(projectPath, relativePath)) {
		return drafts
	}
	return append(drafts, changes.Draft{
		FilePath: relativePath,
		Action:   changes.ActionCreate,
		Content:  content,
		Reason:   reason,
	})
}

func EnsureGoModVersion(content string) string {
	lines := strings.Split(content, "\n")
	for index, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			if !goVersionAtLeast125(fields[1]) {
				lines[index] = "go 1.25"
			}
			return strings.Join(lines, "\n")
		}
	}
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return "go 1.25\n"
	}
	return content + "\n\ngo 1.25\n"
}

func PythonRequirementsContent(items []string) string {
	var lines []string
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		lines = append(lines, item)
	}
	if len(lines) == 0 {
		return "# standard library only\n"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func ensurePythonCommands(items []blueprint.TestCommand, entrypoints []string, dependencies []string) []blueprint.TestCommand {
	out := items
	for _, entrypoint := range entrypoints {
		if strings.HasSuffix(entrypoint, ".py") {
			out = ensureTestCommand(out, ".venv/bin/python -m py_compile "+entrypoint, "", "проверяет синтаксис Python entrypoint внутри virtualenv")
			break
		}
	}
	if hasDependency(dependencies, "pytest") {
		out = ensureTestCommand(out, ".venv/bin/python -m pytest", "", "запускает Python-тесты внутри virtualenv")
	}
	return out
}

func ensureTestCommand(items []blueprint.TestCommand, command string, workingDir string, reason string) []blueprint.TestCommand {
	for _, item := range items {
		if strings.TrimSpace(item.Command) == command && strings.TrimSpace(item.WorkingDir) == workingDir {
			return items
		}
	}
	return append(items, blueprint.TestCommand{Command: command, WorkingDir: workingDir, Reason: reason})
}

func ensureExpectedFile(items []blueprint.ExpectedFile, path string, action string, purpose string) []blueprint.ExpectedFile {
	path = filepath.ToSlash(strings.Trim(path, "/"))
	for index, item := range items {
		if filepath.ToSlash(strings.Trim(item.Path, "/")) != path {
			continue
		}
		if strings.TrimSpace(items[index].Action) == "" {
			items[index].Action = action
		}
		if strings.TrimSpace(items[index].Purpose) == "" {
			items[index].Purpose = purpose
		}
		return items
	}
	return append(items, blueprint.ExpectedFile{Path: path, Action: action, Purpose: purpose})
}

func gitignoreContent(profile Profile) string {
	lines := []string{".zavod/backups/", ".DS_Store"}
	if profile.HasGo {
		lines = append(lines, "bin/", "*.test", "coverage.out")
	}
	if profile.HasPython {
		lines = append(lines, ".venv/", "__pycache__/", "*.py[cod]", ".pytest_cache/")
	}
	return strings.Join(lines, "\n") + "\n"
}

func makefileContent(profile Profile) string {
	var builder strings.Builder
	builder.WriteString("SHELL := /bin/sh\n\n")
	if profile.HasGo {
		builder.WriteString(".PHONY: build test lint run\n\n")
		builder.WriteString("build:\n\tgo build ./...\n\n")
		builder.WriteString("test:\n\tgo test ./...\n\n")
		builder.WriteString("lint:\n\tgo vet ./...\n\n")
		builder.WriteString("run:\n\tgo run .\n")
		return builder.String()
	}
	if profile.HasPython {
		entrypoint := profile.Entrypoint
		if entrypoint == "" || !strings.HasSuffix(entrypoint, ".py") {
			entrypoint = "main.py"
		}
		builder.WriteString(".PHONY: install test run clean\n\n")
		builder.WriteString(".venv/bin/python:\n\tpython3 -m venv .venv\n\n")
		builder.WriteString("install: .venv/bin/python\n\t.venv/bin/python -m pip install -r requirements.txt\n\n")
		builder.WriteString("test: install\n\t.venv/bin/python -m py_compile " + entrypoint + "\n\n")
		builder.WriteString("run: install\n\t.venv/bin/python " + entrypoint + "\n\n")
		builder.WriteString("clean:\n\trm -rf .venv .pytest_cache __pycache__\n")
		return builder.String()
	}
	builder.WriteString(".PHONY: test\n\ntest:\n\t@echo \"No test command configured\"\n")
	return builder.String()
}

func readmeContent(profile Profile) string {
	var builder strings.Builder
	builder.WriteString("# Project\n\n")
	builder.WriteString("## Development\n\n")
	if profile.HasGo {
		builder.WriteString("- Runtime: Go 1.25+\n")
		builder.WriteString("- Build: `make build`\n")
		builder.WriteString("- Test: `make test`\n")
		builder.WriteString("- Lint: `make lint`\n")
	}
	if profile.HasPython {
		builder.WriteString("- Runtime: Python 3 with project-local `.venv`\n")
		builder.WriteString("- Install: `make install`\n")
		builder.WriteString("- Test: `make test`\n")
		builder.WriteString("- Run: `make run`\n")
	}
	builder.WriteString("\n## CI\n\nGitHub Actions workflow is stored in `.github/workflows/ci.yml`.\n")
	return builder.String()
}

func ciContent(profile Profile) string {
	var builder strings.Builder
	builder.WriteString("name: CI\n\n")
	builder.WriteString("on:\n  push:\n  pull_request:\n  workflow_dispatch:\n\n")
	builder.WriteString("jobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n")
	builder.WriteString("      - uses: actions/checkout@v6\n")
	if profile.HasGo {
		builder.WriteString("      - uses: actions/setup-go@v7\n        with:\n          go-version: '1.25'\n          cache: true\n")
		builder.WriteString("      - run: go test ./...\n")
		builder.WriteString("      - run: go vet ./...\n")
	}
	if profile.HasPython {
		builder.WriteString("      - uses: actions/setup-python@v6\n        with:\n          python-version: '3.x'\n")
		builder.WriteString("      - run: python -m venv .venv\n")
		builder.WriteString("      - run: .venv/bin/python -m pip install -r requirements.txt\n")
		if profile.Entrypoint != "" && strings.HasSuffix(profile.Entrypoint, ".py") {
			builder.WriteString("      - run: .venv/bin/python -m py_compile " + profile.Entrypoint + "\n")
		}
	}
	return builder.String()
}

func removePath(items []string, path string) []string {
	out := make([]string, 0, len(items))
	path = filepath.ToSlash(strings.Trim(path, "/"))
	for _, item := range items {
		if filepath.ToSlash(strings.Trim(item, "/")) == path {
			continue
		}
		out = append(out, item)
	}
	return out
}

func hasGoSignals(value blueprint.Blueprint) bool {
	for _, file := range value.ExpectedFiles {
		if strings.HasSuffix(file.Path, ".go") || filepath.Base(file.Path) == "go.mod" {
			return true
		}
	}
	for _, entrypoint := range value.Entrypoints {
		if strings.HasSuffix(entrypoint, ".go") {
			return true
		}
	}
	return false
}

func hasPythonSignals(value blueprint.Blueprint) bool {
	for _, file := range value.ExpectedFiles {
		if strings.HasSuffix(file.Path, ".py") || strings.HasPrefix(filepath.Base(file.Path), "requirements") {
			return true
		}
	}
	for _, entrypoint := range value.Entrypoints {
		if strings.HasSuffix(entrypoint, ".py") {
			return true
		}
	}
	return false
}

func hasDependency(items []string, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, item := range items {
		if strings.ToLower(strings.TrimSpace(item)) == name {
			return true
		}
	}
	return false
}

func goVersionAtLeast125(value string) bool {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > 1 || major == 1 && minor >= 25
}

func draftIndexByPath(drafts []changes.Draft, path string) (int, bool) {
	path = filepath.ToSlash(strings.Trim(path, "/"))
	for index, draft := range drafts {
		if filepath.ToSlash(strings.Trim(draft.FilePath, "/")) == path {
			return index, true
		}
	}
	return -1, false
}

func hasDraftPath(drafts []changes.Draft, path string) bool {
	_, ok := draftIndexByPath(drafts, path)
	return ok
}

func actionForProjectFile(projectPath string, relativePath string) string {
	if fileExists(filepath.Join(projectPath, relativePath)) {
		return changes.ActionReplace
	}
	return changes.ActionCreate
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readProjectFile(projectPath string, relativePath string) string {
	content, err := os.ReadFile(filepath.Join(projectPath, relativePath))
	if err != nil {
		return ""
	}
	return string(content)
}

func requirementsFromContent(content string) []string {
	var items []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		items = append(items, line)
	}
	return items
}

func moduleNameFromProject(projectPath string) string {
	name := strings.ToLower(filepath.Base(projectPath))
	var builder strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			continue
		}
		if r == '-' || r == '_' {
			builder.WriteRune('-')
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		slug = "app"
	}
	return "example.com/" + slug
}
