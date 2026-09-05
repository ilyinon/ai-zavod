package blueprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	StackPython  = "python"
	StackGo      = "go"
	StackNode    = "node"
	StackMixed   = "mixed"
	StackUnknown = "unknown"
)

type ExpectedFile struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Purpose string `json:"purpose"`
}

type DependencyPolicy struct {
	Policy string   `json:"policy"`
	Items  []string `json:"items"`
}

type TestCommand struct {
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir"`
	Reason     string `json:"reason"`
}

type Blueprint struct {
	ID               string           `json:"id"`
	ProjectID        string           `json:"projectId"`
	TaskID           string           `json:"taskId"`
	WorkflowRunID    string           `json:"workflowRunId"`
	Stack            string           `json:"stack"`
	Runtime          string           `json:"runtime"`
	ProjectType      string           `json:"projectType"`
	ScaffoldRequired bool             `json:"scaffoldRequired"`
	Entrypoints      []string         `json:"entrypoints"`
	ExpectedFiles    []ExpectedFile   `json:"expectedFiles"`
	ForbiddenFiles   []string         `json:"forbiddenFiles"`
	Dependencies     DependencyPolicy `json:"dependencies"`
	TestCommands     []TestCommand    `json:"testCommands"`
	OpenQuestions    []string         `json:"openQuestions"`
	Confidence       string           `json:"confidence"`
	RawJSON          string           `json:"rawJson"`
	CreatedAt        string           `json:"createdAt"`
}

type parsedBlueprint struct {
	Stack            string           `json:"stack"`
	Runtime          string           `json:"runtime"`
	ProjectType      string           `json:"project_type"`
	ScaffoldRequired bool             `json:"scaffold_required"`
	Entrypoints      []string         `json:"entrypoints"`
	ExpectedFiles    []ExpectedFile   `json:"expected_files"`
	ForbiddenFiles   []string         `json:"forbidden_files"`
	Dependencies     DependencyPolicy `json:"dependencies"`
	TestCommands     []TestCommand    `json:"test_commands"`
	OpenQuestions    []string         `json:"open_questions"`
	Confidence       string           `json:"confidence"`
}

func Parse(output string) (Blueprint, error) {
	trimmed := stripCodeFence(output)
	if trimmed == "" {
		return Blueprint{}, fmt.Errorf("blueprint пустой")
	}
	var parsed parsedBlueprint
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return Blueprint{}, fmt.Errorf("blueprint не похож на JSON: %w", err)
	}
	result := Blueprint{
		Stack:            normalizeStack(parsed.Stack),
		Runtime:          normalizeRuntime(normalizeStack(parsed.Stack), parsed.Runtime),
		ProjectType:      strings.TrimSpace(parsed.ProjectType),
		ScaffoldRequired: parsed.ScaffoldRequired,
		Entrypoints:      normalizePaths(parsed.Entrypoints),
		ExpectedFiles:    normalizeExpectedFiles(parsed.ExpectedFiles),
		ForbiddenFiles:   normalizePaths(parsed.ForbiddenFiles),
		Dependencies: DependencyPolicy{
			Policy: strings.TrimSpace(parsed.Dependencies.Policy),
			Items:  normalizeStrings(parsed.Dependencies.Items),
		},
		TestCommands:  normalizeTestCommands(parsed.TestCommands),
		OpenQuestions: normalizeStrings(parsed.OpenQuestions),
		Confidence:    normalizeConfidence(parsed.Confidence),
		RawJSON:       trimmed,
	}
	if result.ProjectType == "" {
		result.ProjectType = "unknown"
	}
	return result, nil
}

func NormalizeForProject(value Blueprint, projectPath string) Blueprint {
	switch value.Stack {
	case StackGo:
		value.Runtime = normalizeRuntime(value.Stack, value.Runtime)
		value.ExpectedFiles = ensureGoModuleFile(value.ExpectedFiles, projectPath)
		value.ForbiddenFiles = removePath(value.ForbiddenFiles, "go.mod")
		value.TestCommands = ensureGoTestCommand(value.TestCommands)
		value.RawJSON = toRawJSON(value)
		return value
	case StackPython:
		value.Runtime = normalizeRuntime(value.Stack, value.Runtime)
		value.ExpectedFiles = ensurePythonRequirementsFile(value.ExpectedFiles, projectPath)
		value.ForbiddenFiles = removePath(value.ForbiddenFiles, "requirements.txt")
		value.TestCommands = ensurePythonVenvTestCommands(value.TestCommands, value.Entrypoints)
		value.RawJSON = toRawJSON(value)
		return value
	default:
		return value
	}
}

func ToPrompt(value *Blueprint) string {
	if value == nil || value.RawJSON == "" {
		return "Task blueprint еще не создан."
	}
	return value.RawJSON
}

func RefreshRawJSON(value Blueprint) Blueprint {
	value.RawJSON = toRawJSON(value)
	return value
}

func TestCommandsToSuggestions(value *Blueprint) []struct {
	Command    string
	WorkingDir string
	Reason     string
} {
	if value == nil {
		return nil
	}
	out := make([]struct {
		Command    string
		WorkingDir string
		Reason     string
	}, 0, len(value.TestCommands))
	for _, item := range value.TestCommands {
		out = append(out, struct {
			Command    string
			WorkingDir string
			Reason     string
		}{
			Command:    item.Command,
			WorkingDir: item.WorkingDir,
			Reason:     item.Reason,
		})
	}
	return out
}

func stripCodeFence(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

func normalizeStack(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StackPython:
		return StackPython
	case StackGo:
		return StackGo
	case StackNode:
		return StackNode
	case StackMixed:
		return StackMixed
	default:
		return StackUnknown
	}
}

func runtimeForStack(stack string) string {
	switch stack {
	case StackPython:
		return "Python 3"
	case StackGo:
		return "Go 1.25+"
	case StackNode:
		return "Node.js"
	default:
		return "unknown"
	}
}

func normalizeRuntime(stack string, runtime string) string {
	trimmed := strings.TrimSpace(runtime)
	if stack == StackGo {
		return "Go 1.25+"
	}
	if stack == StackPython {
		return "Python 3 + venv"
	}
	if trimmed != "" {
		return trimmed
	}
	return runtimeForStack(stack)
}

func ensurePythonRequirementsFile(items []ExpectedFile, projectPath string) []ExpectedFile {
	for i, item := range items {
		if filepath.ToSlash(strings.Trim(item.Path, "/")) != "requirements.txt" {
			continue
		}
		if item.Action == "" {
			items[i].Action = pythonRequirementsAction(projectPath)
		}
		if strings.TrimSpace(item.Purpose) == "" {
			items[i].Purpose = "Python dependencies for project virtualenv"
		}
		return items
	}
	return append(items, ExpectedFile{
		Path:    "requirements.txt",
		Action:  pythonRequirementsAction(projectPath),
		Purpose: "Python dependencies for project virtualenv",
	})
}

func ensureGoModuleFile(items []ExpectedFile, projectPath string) []ExpectedFile {
	if strings.TrimSpace(projectPath) != "" {
		if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
			return items
		}
	}
	for _, item := range items {
		if filepath.ToSlash(strings.Trim(item.Path, "/")) == "go.mod" {
			return items
		}
	}
	return append([]ExpectedFile{{
		Path:    "go.mod",
		Action:  "create",
		Purpose: "Go module scaffold with go 1.25",
	}}, items...)
}

func ensureGoTestCommand(items []TestCommand) []TestCommand {
	normalized := normalizeTestCommands(items)
	for _, item := range normalized {
		if strings.TrimSpace(item.Command) == "go test ./..." {
			return normalized
		}
	}
	return append(normalized, TestCommand{
		Command: "go test ./...",
		Reason:  "проверяет Go-проект",
	})
}

func pythonRequirementsAction(projectPath string) string {
	if strings.TrimSpace(projectPath) == "" {
		return "create"
	}
	if _, err := os.Stat(filepath.Join(projectPath, "requirements.txt")); err == nil {
		return "replace"
	}
	return "create"
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

func ensurePythonVenvTestCommands(items []TestCommand, entrypoints []string) []TestCommand {
	normalized := normalizeTestCommands(items)
	hasPython := false
	for _, item := range normalized {
		args := strings.Fields(item.Command)
		if len(args) > 0 && isPythonExecutable(args[0]) {
			hasPython = true
			break
		}
	}
	if hasPython {
		return normalized
	}
	for _, entrypoint := range entrypoints {
		if strings.HasSuffix(entrypoint, ".py") {
			return append(normalized, TestCommand{
				Command: ".venv/bin/python " + entrypoint,
				Reason:  "запускает Python entrypoint внутри virtualenv",
			})
		}
	}
	return normalized
}

func normalizeConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "medium"
	}
}

func normalizePaths(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item = filepath.ToSlash(strings.TrimSpace(item))
		item = strings.Trim(item, "/")
		if item == "" || strings.Contains(item, "..") {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeExpectedFiles(items []ExpectedFile) []ExpectedFile {
	out := make([]ExpectedFile, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item.Path = filepath.ToSlash(strings.Trim(strings.TrimSpace(item.Path), "/"))
		item.Action = strings.ToLower(strings.TrimSpace(item.Action))
		item.Purpose = strings.TrimSpace(item.Purpose)
		if item.Path == "" || strings.Contains(item.Path, "..") {
			continue
		}
		if item.Action == "" {
			item.Action = "create"
		}
		if _, ok := seen[item.Path]; ok {
			continue
		}
		seen[item.Path] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizeTestCommands(items []TestCommand) []TestCommand {
	out := make([]TestCommand, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item.Command = strings.TrimSpace(item.Command)
		item.WorkingDir = strings.Trim(strings.TrimSpace(item.WorkingDir), "/")
		item.Reason = strings.TrimSpace(item.Reason)
		if item.WorkingDir == "." {
			item.WorkingDir = ""
		}
		if item.Command == "" {
			continue
		}
		item.Command = normalizePythonCommand(item.Command)
		key := item.WorkingDir + "\x00" + item.Command
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizePythonCommand(command string) string {
	args := strings.Fields(strings.TrimSpace(command))
	if len(args) == 3 && isPythonExecutable(args[0]) && args[1] == "-m" && args[2] == "pytest" {
		return ".venv/bin/python -m pytest"
	}
	if len(args) == 4 && isPythonExecutable(args[0]) && args[1] == "-m" && args[2] == "py_compile" && strings.HasSuffix(args[3], ".py") {
		return ".venv/bin/python -m py_compile " + args[3]
	}
	if len(args) != 2 || !isPythonExecutable(args[0]) || !strings.HasSuffix(args[1], ".py") {
		return command
	}
	return ".venv/bin/python " + args[1]
}

func isPythonExecutable(value string) bool {
	switch strings.TrimSpace(value) {
	case "python", "python3", ".venv/bin/python", ".venv/bin/python3":
		return true
	default:
		return false
	}
}

func toRawJSON(value Blueprint) string {
	payload := parsedBlueprint{
		Stack:            value.Stack,
		Runtime:          value.Runtime,
		ProjectType:      value.ProjectType,
		ScaffoldRequired: value.ScaffoldRequired,
		Entrypoints:      value.Entrypoints,
		ExpectedFiles:    value.ExpectedFiles,
		ForbiddenFiles:   value.ForbiddenFiles,
		Dependencies:     value.Dependencies,
		TestCommands:     value.TestCommands,
		OpenQuestions:    value.OpenQuestions,
		Confidence:       value.Confidence,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return value.RawJSON
	}
	return string(data)
}

func normalizeStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}
