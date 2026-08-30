package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusBlocked = "blocked"

	maxOutputBytes = 50 * 1024
)

type TestRun struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	TaskID        string `json:"taskId"`
	WorkflowRunID string `json:"workflowRunId"`
	Command       string `json:"command"`
	WorkingDir    string `json:"workingDir"`
	Reason        string `json:"reason"`
	Status        string `json:"status"`
	ExitCode      int    `json:"exitCode"`
	Stdout        string `json:"stdout"`
	Stderr        string `json:"stderr"`
	Error         string `json:"error"`
	StartedAt     string `json:"startedAt"`
	FinishedAt    string `json:"finishedAt"`
	CreatedAt     string `json:"createdAt"`
}

type Suggestion struct {
	Command    string `json:"command"`
	WorkingDir string `json:"working_dir"`
	Reason     string `json:"reason"`
}

type RunResult struct {
	Status   string
	ExitCode int
	Stdout   string
	Stderr   string
	Error    string
}

type testerResponse struct {
	Summary  string       `json:"summary"`
	Commands []Suggestion `json:"commands"`
}

func ExtractSuggestions(output string) []Suggestion {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var response testerResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err == nil {
		return normalizeSuggestions(response.Commands)
	}

	var commands []Suggestion
	if err := json.Unmarshal([]byte(trimmed), &commands); err == nil {
		return normalizeSuggestions(commands)
	}
	return nil
}

func DefaultSuggestions(projectPath string) []Suggestion {
	var suggestions []Suggestion
	if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
		suggestions = append(suggestions, Suggestion{
			Command: "go test ./...",
			Reason:  "проверяет Go backend и бизнес-логику",
		})
	}
	if _, err := os.Stat(filepath.Join(projectPath, "frontend", "package.json")); err == nil {
		suggestions = append(suggestions, Suggestion{
			Command:    "npm run build",
			WorkingDir: "frontend",
			Reason:     "проверяет сборку frontend",
		})
	} else if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
		suggestions = append(suggestions, Suggestion{
			Command: "npm run build",
			Reason:  "проверяет сборку frontend",
		})
	}
	for _, script := range defaultPythonScripts(projectPath) {
		suggestions = append(suggestions, Suggestion{
			Command: ".venv/bin/python " + script,
			Reason:  "запускает Python-скрипт проверки",
		})
	}
	return suggestions
}

func FilterSupportedSuggestions(projectPath string, suggestions []Suggestion) []Suggestion {
	filtered := make([]Suggestion, 0, len(suggestions))
	for _, suggestion := range normalizeSuggestions(suggestions) {
		if ValidateCommand(projectPath, suggestion.Command, suggestion.WorkingDir) == nil {
			filtered = append(filtered, suggestion)
		}
	}
	return filtered
}

func ValidateCommand(projectPath string, command string, workingDir string) error {
	if _, _, err := Resolve(projectPath, command, workingDir); err != nil {
		return err
	}
	return nil
}

func Run(ctx context.Context, projectPath string, command string, workingDir string) RunResult {
	workDir, args, err := Resolve(projectPath, command, workingDir)
	if err != nil {
		return RunResult{Status: StatusBlocked, ExitCode: -1, Error: err.Error()}
	}
	args = resolveExecutableAlias(args)

	runCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()

	var prepStdout string
	var prepStderr string
	if isPythonArgs(args) {
		preparedArgs, stdout, stderr, err := preparePythonVirtualenv(runCtx, workDir, args)
		prepStdout = stdout
		prepStderr = stderr
		if err != nil {
			return RunResult{
				Status:   StatusFailed,
				ExitCode: -1,
				Stdout:   prepStdout,
				Stderr:   prepStderr,
				Error:    err.Error(),
			}
		}
		args = preparedArgs
	}

	cmd := exec.CommandContext(runCtx, args[0], args[1:]...)
	cmd.Dir = workDir

	var stdout limitedBuffer
	var stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	result := RunResult{
		Status:   StatusPassed,
		ExitCode: 0,
		Stdout:   prepStdout + stdout.String(),
		Stderr:   prepStderr + stderr.String(),
	}
	if err == nil {
		return result
	}

	result.Status = StatusFailed
	result.ExitCode = 1
	if runCtx.Err() != nil {
		result.Error = "команда остановлена по timeout 180 секунд"
		result.ExitCode = -1
		return result
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		result.Error = fmt.Sprintf("команда завершилась с exit code %d", result.ExitCode)
		return result
	}
	result.Error = err.Error()
	return result
}

func resolveExecutableAlias(args []string) []string {
	if len(args) == 0 || args[0] != "python" {
		return args
	}
	if _, err := exec.LookPath("python"); err == nil {
		return args
	}
	if _, err := exec.LookPath("python3"); err == nil {
		out := append([]string{}, args...)
		out[0] = "python3"
		return out
	}
	return args
}

func preparePythonVirtualenv(ctx context.Context, workDir string, args []string) ([]string, string, string, error) {
	venvPython := filepath.Join(workDir, ".venv", "bin", "python")
	var stdout strings.Builder
	var stderr strings.Builder

	if _, err := os.Stat(venvPython); err != nil {
		if !os.IsNotExist(err) {
			return nil, stdout.String(), stderr.String(), err
		}
		cmd := exec.CommandContext(ctx, "python3", "-m", "venv", ".venv")
		cmd.Dir = workDir
		cmd.Env = append(os.Environ(), "PYTHONNOUSERSITE=1")
		out, errOut, err := runPrepCommand(cmd)
		stdout.WriteString(out)
		stderr.WriteString(errOut)
		if err != nil {
			return nil, stdout.String(), stderr.String(), fmt.Errorf("не удалось создать virtualenv: %w", err)
		}
	}

	requirementsPath := filepath.Join(workDir, "requirements.txt")
	if _, err := os.Stat(requirementsPath); err == nil {
		cmd := exec.CommandContext(ctx, venvPython, "-m", "pip", "install", "-r", "requirements.txt")
		cmd.Dir = workDir
		cmd.Env = append(os.Environ(), "PYTHONNOUSERSITE=1")
		out, errOut, err := runPrepCommand(cmd)
		stdout.WriteString(out)
		stderr.WriteString(errOut)
		if err != nil {
			return nil, stdout.String(), stderr.String(), fmt.Errorf("не удалось установить requirements.txt в virtualenv: %w", err)
		}
	}

	prepared := append([]string{}, args...)
	prepared[0] = venvPython
	return prepared, stdout.String(), stderr.String(), nil
}

func runPrepCommand(cmd *exec.Cmd) (string, string, error) {
	var stdout limitedBuffer
	var stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func Resolve(projectPath string, command string, workingDir string) (string, []string, error) {
	projectRoot, err := filepath.Abs(projectPath)
	if err != nil {
		return "", nil, err
	}
	workDir, err := resolveWorkingDir(projectRoot, workingDir)
	if err != nil {
		return "", nil, err
	}
	if err := validateShellSafety(command); err != nil {
		return "", nil, err
	}
	args := strings.Fields(strings.TrimSpace(command))
	if len(args) == 0 {
		return "", nil, fmt.Errorf("команда пустая")
	}
	if !isAllowedArgs(args) {
		return "", nil, fmt.Errorf("команда не входит в allowlist проверок")
	}
	if err := validateProjectSupport(workDir, args); err != nil {
		return "", nil, err
	}
	return workDir, args, nil
}

func resolveWorkingDir(projectRoot string, workingDir string) (string, error) {
	workingDir = strings.TrimSpace(workingDir)
	if filepath.IsAbs(workingDir) {
		return "", fmt.Errorf("working_dir должен быть относительным")
	}
	target := projectRoot
	if workingDir != "" && workingDir != "." {
		target = filepath.Join(projectRoot, workingDir)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(projectRoot, absTarget)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("working_dir выходит за пределы проекта")
	}
	info, err := os.Stat(absTarget)
	if err != nil {
		return "", fmt.Errorf("working_dir недоступен: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working_dir должен быть каталогом")
	}
	return absTarget, nil
}

func validateShellSafety(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("команда пустая")
	}
	blocked := []string{"&&", "||", ";", "|", ">", "<", "$(", "`", "\n", "\r"}
	for _, token := range blocked {
		if strings.Contains(command, token) {
			return fmt.Errorf("команда содержит запрещенный shell-оператор: %s", token)
		}
	}
	return nil
}

func isAllowedArgs(args []string) bool {
	if len(args) < 2 {
		return false
	}
	switch args[0] {
	case "go":
		if len(args) != 3 {
			return false
		}
		if args[1] != "test" && args[1] != "vet" {
			return false
		}
		return args[2] == "./..." || isSafeGoPackage(args[2])
	case "npm":
		if len(args) == 2 && args[1] == "test" {
			return true
		}
		if len(args) == 3 && args[1] == "run" {
			return args[2] == "test" || args[2] == "build" || args[2] == "lint"
		}
	default:
		if isPythonExecutable(args[0]) {
			if len(args) != 2 {
				return false
			}
			return isSafePythonScript(args[1])
		}
	}
	return false
}

func isPythonArgs(args []string) bool {
	return len(args) >= 1 && isPythonExecutable(args[0])
}

func isPythonExecutable(value string) bool {
	switch strings.TrimSpace(filepath.ToSlash(value)) {
	case "python", "python3", ".venv/bin/python", ".venv/bin/python3":
		return true
	default:
		return false
	}
}

func isSafeGoPackage(value string) bool {
	if !strings.HasPrefix(value, "./") || strings.Contains(value, "..") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		if r == '/' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func isSafePythonScript(value string) bool {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "..") {
		return false
	}
	if strings.HasPrefix(value, "-") || !strings.HasSuffix(value, ".py") {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		if r == '/' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validateProjectSupport(workDir string, args []string) error {
	if isPythonExecutable(args[0]) {
		scriptPath := filepath.Join(workDir, args[1])
		info, err := os.Stat(scriptPath)
		if err != nil {
			return fmt.Errorf("Python-скрипт не найден: %s", args[1])
		}
		if info.IsDir() {
			return fmt.Errorf("Python-скрипт должен быть файлом: %s", args[1])
		}
		return nil
	}

	switch args[0] {
	case "go":
		if _, err := os.Stat(filepath.Join(workDir, "go.mod")); err != nil {
			return fmt.Errorf("Go-проверка недоступна: в рабочем каталоге нет go.mod")
		}
	case "npm":
		if _, err := os.Stat(filepath.Join(workDir, "package.json")); err != nil {
			return fmt.Errorf("npm-проверка недоступна: в рабочем каталоге нет package.json")
		}
	}
	return nil
}

func defaultPythonScripts(projectPath string) []string {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil
	}
	available := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}
		available[entry.Name()] = struct{}{}
	}
	preferred := []string{"check.py", "site_health_check.py", "main.py", "app.py"}
	for _, name := range preferred {
		if _, ok := available[name]; ok {
			return []string{name}
		}
	}
	for name := range available {
		return []string{name}
	}
	return nil
}

func normalizeSuggestions(items []Suggestion) []Suggestion {
	out := make([]Suggestion, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item.Command = strings.TrimSpace(item.Command)
		item.WorkingDir = strings.Trim(strings.TrimSpace(item.WorkingDir), "/")
		item.Reason = strings.TrimSpace(item.Reason)
		if item.Command == "" {
			continue
		}
		item.Command = normalizePythonSuggestionCommand(item.Command)
		key := item.WorkingDir + "\x00" + item.Command
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func normalizePythonSuggestionCommand(command string) string {
	args := strings.Fields(strings.TrimSpace(command))
	if len(args) != 2 || !isPythonExecutable(args[0]) || !isSafePythonScript(args[1]) {
		return command
	}
	return ".venv/bin/python " + args[1]
}

type limitedBuffer struct {
	data      bytes.Buffer
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := maxOutputBytes - b.data.Len()
	if remaining > 0 {
		if len(p) <= remaining {
			_, _ = b.data.Write(p)
		} else {
			_, _ = b.data.Write(p[:remaining])
			b.truncated = true
		}
	} else {
		b.truncated = true
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	value := b.data.String()
	if b.truncated {
		value += "\n\n[output truncated]"
	}
	return value
}
