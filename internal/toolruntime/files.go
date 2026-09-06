package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"zavod_ai/internal/checks"
	"zavod_ai/internal/llm"
)

type workspace struct {
	*os.Root
	scope Scope
}

func openWorkspace(scope Scope) (*workspace, error) {
	if !filepath.IsAbs(scope.WorkingDir) {
		return nil, errors.New("working directory must be absolute")
	}
	root, err := os.OpenRoot(scope.WorkingDir)
	if err != nil {
		return nil, err
	}
	return &workspace{Root: root, scope: scope}, nil
}

type args struct {
	Path    string `json:"path"`
	Query   string `json:"query"`
	Command string `json:"command"`
	Offset  int    `json:"offset"`
	Limit   int    `json:"limit"`
}

func (w *workspace) execute(ctx context.Context, call llm.ToolCall, limit int) Result {
	blocked := func(err error) Result { return Result{Status: "blocked", Error: err.Error()} }
	if err := ctx.Err(); err != nil {
		return blocked(err)
	}
	name := call.Function.Name
	if !contains(w.scope.AllowedTools, name) {
		return blocked(errors.New("tool not allowed for this agent"))
	}
	// Validate against each tool's own schema, not the union of fields.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(call.Function.Arguments), &raw); err != nil || raw == nil {
		return blocked(errors.New("arguments must be a JSON object"))
	}
	allowed := map[string][]string{"list_files": {"path"}, "read_file": {"path", "offset", "limit"}, "search_files": {"path", "query"}, "run_check": {"path", "command"}}
	for key, value := range raw {
		if !contains(allowed[name], key) || string(value) == "null" {
			return blocked(errors.New("unknown or null argument: " + key))
		}
	}
	var a args
	if err := json.Unmarshal([]byte(call.Function.Arguments), &a); err != nil {
		return blocked(err)
	}
	if name == "read_file" && a.Path == "" {
		return blocked(errors.New("path is required"))
	}
	if a.Path == "" {
		a.Path = "."
	}
	if err := w.safePath(a.Path); err != nil {
		return blocked(err)
	}
	switch name {
	case "run_check":
		if !contains([]string{"go test ./...", "go vet ./...", "python3 -m pytest", ".venv/bin/python -m pytest"}, a.Command) {
			return blocked(errors.New("command is not a supported check; arbitrary shell and approval execution are unavailable"))
		}
		if w.scope.ToolProfileID == "" {
			return blocked(errors.New("no execution policy profile"))
		}
		if err := checks.ValidateCommandWithToolProfile(w.scope.WorkingDir, w.scope.ToolProfileID, a.Command, a.Path); err != nil {
			return blocked(err)
		}
		result := checks.RunWithToolProfile(ctx, w.scope.WorkingDir, w.scope.ToolProfileID, a.Command, a.Path)
		return Result{Status: result.Status, Output: result.Stdout + "\n" + result.Stderr, Error: result.Error, ExitCode: &result.ExitCode}
	case "read_file":
		if !permitted(a.Path, w.scope.ReadPaths) {
			return blocked(errors.New("path is outside agent read permissions"))
		}
		if a.Offset == 0 {
			a.Offset = 1
		}
		if a.Limit == 0 {
			a.Limit = 160
		}
		if a.Offset < 1 || a.Offset > 1000000 || a.Limit < 1 || a.Limit > 400 {
			return blocked(errors.New("invalid line range"))
		}
		content, err := w.read(a.Path)
		if err != nil {
			return blocked(err)
		}
		lines := strings.Split(content, "\n")
		var out strings.Builder
		end := min(len(lines), a.Offset-1+a.Limit)
		for i := a.Offset - 1; i < end; i++ {
			fmt.Fprintf(&out, "%d: %s\n", i+1, lines[i])
			if out.Len() > limit {
				break
			}
		}
		return Result{Status: "passed", Output: out.String(), Truncated: end < len(lines) || out.Len() > limit}
	case "list_files", "search_files":
		if name == "search_files" && (a.Query == "" || len(a.Query) > 256) {
			return blocked(errors.New("query must contain 1..256 bytes"))
		}
		info, err := w.Stat(a.Path)
		if err != nil || !info.IsDir() {
			return blocked(errors.New("path must be an existing directory"))
		}
		var out strings.Builder
		visited, scanned := 0, 0
		truncated := false
		err = fs.WalkDir(w.FS(), path.Clean(a.Path), func(p string, d fs.DirEntry, walkErr error) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			visited++
			if visited > 3000 || scanned > 8*1024*1024 || out.Len() > limit {
				truncated = true
				return fs.SkipAll
			}
			if walkErr != nil {
				truncated = true
				return nil
			}
			if excluded(p) || d.Type()&os.ModeSymlink != 0 {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() || !permitted(p, w.scope.ReadPaths) {
				return nil
			}
			if name == "list_files" {
				out.WriteString(p + "\n")
				return nil
			}
			text, err := w.read(p)
			if err != nil {
				truncated = true
				return nil
			}
			scanned += len(text)
			for i, line := range strings.Split(text, "\n") {
				if strings.Contains(line, a.Query) {
					fmt.Fprintf(&out, "%s:%d: %s\n", p, i+1, line)
				}
				if out.Len() > limit {
					truncated = true
					return fs.SkipAll
				}
			}
			return nil
		})
		if err != nil {
			return blocked(err)
		}
		return Result{Status: "passed", Output: out.String(), Truncated: truncated}
	}
	return blocked(errors.New("unknown tool"))
}

func (w *workspace) safePath(p string) error {
	if !fs.ValidPath(strings.TrimPrefix(p, "./")) && p != "." {
		return errors.New("use a project-relative path without traversal")
	}
	if excluded(p) {
		return errors.New("private or generated path is excluded")
	}
	current := "."
	for _, part := range strings.Split(p, "/") {
		current = path.Join(current, part)
		info, err := w.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlinks are not permitted")
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return errors.New("special files are not permitted")
		}
	}
	return nil
}

func (w *workspace) read(p string) (string, error) {
	if err := w.safePath(p); err != nil {
		return "", err
	}
	file, err := w.Open(p)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return "", errors.New("not a regular text file or exceeds 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024+1))
	if err != nil {
		return "", err
	}
	if len(data) > 1024*1024 || !utf8.Valid(data) || strings.IndexByte(string(data), 0) >= 0 {
		return "", errors.New("binary or oversized file")
	}
	return string(data), nil
}

func excluded(p string) bool {
	for _, part := range strings.Split(p, "/") {
		part = strings.ToLower(part)
		if contains([]string{".git", ".zavod", ".venv", "venv", "node_modules", "dist", "build", ".ssh", ".aws", ".cache", "__pycache__"}, part) || strings.HasPrefix(part, ".env") || strings.HasSuffix(part, ".pem") || strings.HasSuffix(part, ".key") || strings.HasPrefix(part, "id_rsa") || strings.HasPrefix(part, "id_ed25519") {
			return true
		}
	}
	return false
}

func permitted(p string, patterns []string) bool {
	p = strings.TrimPrefix(p, "./")
	allow := false
	for _, pattern := range patterns {
		deny := strings.HasPrefix(pattern, "!")
		pattern = strings.TrimPrefix(strings.TrimPrefix(pattern, "!"), "./")
		if glob(strings.Split(pattern, "/"), strings.Split(p, "/")) {
			if deny {
				return false
			}
			allow = true
		}
	}
	return allow
}

func glob(pattern, parts []string) bool {
	if len(pattern) == 0 {
		return len(parts) == 0
	}
	if pattern[0] == "**" {
		return glob(pattern[1:], parts) || len(parts) > 0 && glob(pattern, parts[1:])
	}
	if len(parts) == 0 {
		return false
	}
	ok, _ := path.Match(pattern[0], parts[0])
	return ok && glob(pattern[1:], parts[1:])
}
