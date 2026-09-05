package ctf

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	CategoryWeb       = "web"
	CategoryLFI       = "LFI"
	CategoryRCE       = "RCE"
	CategorySQLi      = "SQLi"
	CategoryPwn       = "pwn"
	CategoryCrypto    = "crypto"
	CategoryReverse   = "reverse"
	CategoryForensics = "forensics"
)

var Categories = []string{
	CategoryWeb,
	CategoryLFI,
	CategoryRCE,
	CategorySQLi,
	CategoryPwn,
	CategoryCrypto,
	CategoryReverse,
	CategoryForensics,
}

type Workspace struct {
	Title          string
	Slug           string
	Category       string
	RelativeRoot   string
	ChallengeYAML  string
	ScopeMD        string
	NotesMD        string
	WriteupMD      string
	ArtifactsDir   string
	EvidenceDir    string
	SolveDir       string
	ScopeStatus    string
	RequiresScope  bool
	AllowedActions []string
}

func IsCTFRequest(message string) bool {
	text := normalized(message)
	return containsAny(text,
		"ctf", "capture the flag", "challenge", "таск", "таска", "райтап", "writeup",
		"hackthebox", "tryhackme", "picoctf", "ctftime", "root-me", "portswigger lab",
		"lfi", "rce", "sqli", "sql injection", "pwn", "pwning", "crypto", "reverse", "reversing", "forensics",
	)
}

func Classify(message string) string {
	text := normalized(message)
	switch {
	case containsAny(text, "lfi", "local file inclusion", "file inclusion", "path traversal", "../", "%2e%2e"):
		return CategoryLFI
	case containsAny(text, "rce", "remote code execution", "command injection", "cmd injection", "ssti", "template injection"):
		return CategoryRCE
	case containsAny(text, "sqli", "sql injection", "union select", "blind sql", "sqlite injection", "mysql injection"):
		return CategorySQLi
	case containsAny(text, "pwn", "pwning", "buffer overflow", "rop", "ret2", "heap", "format string", "binary exploitation"):
		return CategoryPwn
	case containsAny(text, "crypto", "rsa", "aes", "xor", "cipher", "oracle", "hash", "lattice"):
		return CategoryCrypto
	case containsAny(text, "reverse", "reversing", "rev", "ghidra", "ida", "radare", "objdump", "decompile"):
		return CategoryReverse
	case containsAny(text, "forensics", "pcap", "wireshark", "memory dump", "dump", "stego", "exif", "binwalk"):
		return CategoryForensics
	case containsAny(text, "web", "http", "url", "cookie", "xss", "csrf", "ssrf", "jwt", "auth"):
		return CategoryWeb
	default:
		return CategoryWeb
	}
}

func SolverAgentID(category string) string {
	switch category {
	case CategoryLFI:
		return "ctf_lfi"
	case CategoryRCE:
		return "ctf_rce"
	case CategorySQLi:
		return "ctf_sqli"
	case CategoryPwn:
		return "ctf_pwn"
	case CategoryCrypto:
		return "ctf_crypto"
	case CategoryReverse:
		return "ctf_reverse"
	case CategoryForensics:
		return "ctf_forensics"
	default:
		return "ctf_web"
	}
}

func ScopeStatus(message string, category string) (status string, requiresScope bool, allowed []string) {
	text := normalized(message)
	localOrCTF := containsAny(text,
		"ctf", "capture the flag", "challenge", "таск", "таска", "райтап", "writeup",
		"hackthebox", "tryhackme", "picoctf", "ctftime", "root-me", "portswigger lab",
		"localhost", "127.0.0.1", "0.0.0.0", "docker", "container", "локаль", "локальный",
		"файл", "provided file", "attached file", "challenge file", "lab", "лаба", "учебн",
	)
	activeNetwork := category == CategoryWeb || category == CategoryLFI || category == CategoryRCE || category == CategorySQLi
	hasExternalTarget := regexp.MustCompile(`(?i)\bhttps?://|(?:\b\d{1,3}\.){3}\d{1,3}\b|\b[a-z0-9][a-z0-9-]+\.[a-z]{2,}\b`).FindString(message) != ""
	if activeNetwork && hasExternalTarget && !localOrCTF && !containsAny(text, "разреш", "scope", "authorized", "permission", "имею право") {
		return "needs_scope", true, []string{"passive_notes", "scope_request"}
	}
	if activeNetwork {
		return "ctf_or_lab_scope", false, []string{"manual_http_notes", "local_payload_lab", "rate_limited_checks"}
	}
	return "local_artifact_scope", false, []string{"local_file_analysis", "solver_scripts", "writeup"}
}

func PrepareWorkspace(projectPath string, taskTitle string, request string, category string, now time.Time) (Workspace, error) {
	if strings.TrimSpace(projectPath) == "" {
		return Workspace{}, fmt.Errorf("project path is required")
	}
	title := strings.TrimSpace(taskTitle)
	if title == "" {
		title = titleFromRequest(request)
	}
	if title == "" {
		title = "ctf challenge"
	}
	if category == "" {
		category = Classify(request)
	}
	slug := slugify(title)
	if slug == "" {
		slug = "challenge"
	}
	root := filepath.Join("ctf", slug)
	workspace := Workspace{
		Title:         title,
		Slug:          slug,
		Category:      category,
		RelativeRoot:  root,
		ChallengeYAML: filepath.Join(root, "challenge.yml"),
		ScopeMD:       filepath.Join(root, "scope.md"),
		NotesMD:       filepath.Join(root, "notes.md"),
		WriteupMD:     filepath.Join(root, "writeup.md"),
		ArtifactsDir:  filepath.Join(root, "artifacts"),
		EvidenceDir:   filepath.Join(root, "evidence"),
		SolveDir:      filepath.Join(root, "solve"),
	}
	workspace.ScopeStatus, workspace.RequiresScope, workspace.AllowedActions = ScopeStatus(request, category)

	for _, dir := range []string{workspace.ArtifactsDir, workspace.EvidenceDir, workspace.SolveDir} {
		if err := os.MkdirAll(filepath.Join(projectPath, dir), 0o755); err != nil {
			return Workspace{}, err
		}
	}
	files := map[string]string{
		workspace.ChallengeYAML:                        challengeYAML(workspace, now),
		workspace.ScopeMD:                              scopeMarkdown(workspace, request),
		workspace.NotesMD:                              notesMarkdown(workspace, request),
		filepath.Join(workspace.SolveDir, "README.md"): solveReadme(workspace),
		workspace.WriteupMD:                            writeupMarkdown(workspace),
	}
	for relativePath, content := range files {
		target := filepath.Join(projectPath, relativePath)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return Workspace{}, err
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return Workspace{}, err
		}
	}
	return workspace, nil
}

func AppendNotes(projectPath string, workspace Workspace, sections map[string]string) error {
	var builder strings.Builder
	for title, content := range sections {
		content = strings.TrimSpace(content)
		if content == "" {
			continue
		}
		builder.WriteString("\n\n## ")
		builder.WriteString(title)
		builder.WriteString("\n\n")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	if builder.Len() == 0 {
		return nil
	}
	return appendFile(projectPath, workspace.NotesMD, builder.String())
}

func WriteWriteup(projectPath string, workspace Workspace, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	return os.WriteFile(filepath.Join(projectPath, workspace.WriteupMD), []byte(content+"\n"), 0o644)
}

func appendFile(projectPath string, relativePath string, content string) error {
	target := filepath.Join(projectPath, relativePath)
	file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(content)
	return err
}

func titleFromRequest(request string) string {
	words := strings.Fields(strings.TrimSpace(request))
	if len(words) > 7 {
		words = words[:7]
	}
	return strings.Join(words, " ")
}

func challengeYAML(workspace Workspace, now time.Time) string {
	stamp := now.UTC().Format(time.RFC3339)
	return fmt.Sprintf(`title: %q
event: ""
category: %q
difficulty: ""
provided_files: []
target: ""
flag_format: ""
status: "in_progress"
scope_status: %q
created_at: %q
updated_at: %q
`, workspace.Title, workspace.Category, workspace.ScopeStatus, stamp, stamp)
}

func scopeMarkdown(workspace Workspace, request string) string {
	return fmt.Sprintf(`# Scope

## Target

Заполнить явно, если задача требует активных сетевых действий.

## Authorization

Status: %s

## Allowed Actions

%s

## Forbidden Actions

- атаки на реальные сторонние системы без разрешения
- persistence, stealth, credential harvesting
- destructive actions

## Rate Limits

Manual / challenge-safe by default.

## Time Window

Current CTF/lab session.

## Evidence Directory

%s

## Original Request

%s
`, workspace.ScopeStatus, markdownList(workspace.AllowedActions), workspace.EvidenceDir, strings.TrimSpace(request))
}

func notesMarkdown(workspace Workspace, request string) string {
	return fmt.Sprintf(`# Notes

## Challenge

- Title: %s
- Category: %s
- Scope: %s

## Observations

Original request:

%s

## Hypotheses

## Attempts

## Dead Ends

## Evidence

## Next Steps
`, workspace.Title, workspace.Category, workspace.ScopeStatus, strings.TrimSpace(request))
}

func solveReadme(workspace Workspace) string {
	return fmt.Sprintf(`# Solve

Category: %s

Keep local solver scripts and reproducible notes here.
`, workspace.Category)
}

func writeupMarkdown(workspace Workspace) string {
	return fmt.Sprintf(`# Writeup

## Challenge

%s

## Category

%s

## Summary

## Approach

## Exploit or Solution

## Flag

## Lessons Learned
`, workspace.Title, workspace.Category)
}

func markdownList(items []string) string {
	if len(items) == 0 {
		return "- passive_notes"
	}
	var builder strings.Builder
	for _, item := range items {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func normalized(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
