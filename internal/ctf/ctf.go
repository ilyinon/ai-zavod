package ctf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	EvidenceIndex  string
	EvidenceEvents string
	SolveDir       string
	ScopeStatus    string
	RequiresScope  bool
	AllowedActions []string
}

type EvidenceEntry struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Title        string            `json:"title"`
	AgentID      string            `json:"agentId,omitempty"`
	StepKey      string            `json:"stepKey,omitempty"`
	Source       string            `json:"source,omitempty"`
	RelativePath string            `json:"relativePath"`
	Summary      string            `json:"summary,omitempty"`
	Content      string            `json:"content,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	CreatedAt    string            `json:"createdAt"`
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

func ToolProfileID(category string) string {
	switch category {
	case CategoryLFI:
		return "tool_ctf_lfi"
	case CategoryRCE:
		return "tool_ctf_rce"
	case CategorySQLi:
		return "tool_ctf_sqli"
	case CategoryPwn:
		return "tool_ctf_pwn"
	case CategoryCrypto:
		return "tool_ctf_crypto"
	case CategoryReverse:
		return "tool_ctf_reverse"
	case CategoryForensics:
		return "tool_ctf_forensics"
	default:
		return "tool_ctf_web"
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
		Title:          title,
		Slug:           slug,
		Category:       category,
		RelativeRoot:   root,
		ChallengeYAML:  filepath.Join(root, "challenge.yml"),
		ScopeMD:        filepath.Join(root, "scope.md"),
		NotesMD:        filepath.Join(root, "notes.md"),
		WriteupMD:      filepath.Join(root, "writeup.md"),
		ArtifactsDir:   filepath.Join(root, "artifacts"),
		EvidenceDir:    filepath.Join(root, "evidence"),
		EvidenceIndex:  filepath.Join(root, "evidence", "index.md"),
		EvidenceEvents: filepath.Join(root, "evidence", "events.jsonl"),
		SolveDir:       filepath.Join(root, "solve"),
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
		workspace.EvidenceIndex:                        evidenceIndexMarkdown(workspace),
		workspace.EvidenceEvents:                       "",
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

func RecordEvidence(projectPath string, workspace Workspace, entry EvidenceEntry, now time.Time) (EvidenceEntry, error) {
	if strings.TrimSpace(projectPath) == "" {
		return EvidenceEntry{}, fmt.Errorf("project path is required")
	}
	if strings.TrimSpace(workspace.EvidenceDir) == "" {
		workspace.EvidenceDir = filepath.Join(workspace.RelativeRoot, "evidence")
	}
	if strings.TrimSpace(workspace.EvidenceIndex) == "" {
		workspace.EvidenceIndex = filepath.Join(workspace.EvidenceDir, "index.md")
	}
	if strings.TrimSpace(workspace.EvidenceEvents) == "" {
		workspace.EvidenceEvents = filepath.Join(workspace.EvidenceDir, "events.jsonl")
	}
	entry.Kind = normalizeEvidenceKind(entry.Kind)
	entry.Title = strings.TrimSpace(entry.Title)
	if entry.Title == "" {
		entry.Title = entry.Kind
	}
	entry.Content = strings.TrimSpace(entry.Content)
	entry.Summary = strings.TrimSpace(entry.Summary)
	if entry.Summary == "" {
		entry.Summary = firstNonEmptyLine(entry.Content)
	}
	if entry.Summary == "" {
		entry.Summary = entry.Title
	}
	if now.IsZero() {
		now = time.Now()
	}
	if entry.ID == "" {
		entry.ID = evidenceID(entry, now)
	}
	entry.CreatedAt = now.UTC().Format(time.RFC3339)
	entry.RelativePath = filepath.ToSlash(filepath.Join(workspace.EvidenceDir, entry.ID+".md"))

	if err := os.MkdirAll(filepath.Join(projectPath, workspace.EvidenceDir), 0o755); err != nil {
		return EvidenceEntry{}, err
	}
	if err := writeProjectRelative(projectPath, entry.RelativePath, evidenceEntryMarkdown(entry)); err != nil {
		return EvidenceEntry{}, err
	}
	if err := appendEvidenceIndex(projectPath, workspace, entry); err != nil {
		return EvidenceEntry{}, err
	}
	if err := appendEvidenceEvent(projectPath, workspace, entry); err != nil {
		return EvidenceEntry{}, err
	}
	return entry, nil
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

func evidenceIndexMarkdown(workspace Workspace) string {
	return fmt.Sprintf(`# Evidence Store

Challenge: %s
Category: %s

This directory stores command outputs, discovered files, payload notes, screenshots, pcap analysis, solver outputs and validation evidence outside chat.

## Entries
`, workspace.Title, workspace.Category)
}

func evidenceEntryMarkdown(entry EvidenceEntry) string {
	var builder strings.Builder
	builder.WriteString("# ")
	builder.WriteString(entry.Title)
	builder.WriteString("\n\n")
	builder.WriteString("- Kind: ")
	builder.WriteString(entry.Kind)
	builder.WriteString("\n")
	if entry.StepKey != "" {
		builder.WriteString("- Step: ")
		builder.WriteString(entry.StepKey)
		builder.WriteString("\n")
	}
	if entry.AgentID != "" {
		builder.WriteString("- Agent: ")
		builder.WriteString(entry.AgentID)
		builder.WriteString("\n")
	}
	if entry.Source != "" {
		builder.WriteString("- Source: ")
		builder.WriteString(entry.Source)
		builder.WriteString("\n")
	}
	if entry.CreatedAt != "" {
		builder.WriteString("- Created: ")
		builder.WriteString(entry.CreatedAt)
		builder.WriteString("\n")
	}
	if len(entry.Metadata) > 0 {
		builder.WriteString("\n## Metadata\n\n")
		keys := make([]string, 0, len(entry.Metadata))
		for key := range entry.Metadata {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			builder.WriteString("- ")
			builder.WriteString(key)
			builder.WriteString(": ")
			builder.WriteString(entry.Metadata[key])
			builder.WriteString("\n")
		}
	}
	if entry.Summary != "" {
		builder.WriteString("\n## Summary\n\n")
		builder.WriteString(entry.Summary)
		builder.WriteString("\n")
	}
	if entry.Content != "" {
		builder.WriteString("\n## Content\n\n")
		builder.WriteString(entry.Content)
		builder.WriteString("\n")
	}
	return builder.String()
}

func appendEvidenceIndex(projectPath string, workspace Workspace, entry EvidenceEntry) error {
	target := filepath.Join(projectPath, workspace.EvidenceIndex)
	if _, err := os.Stat(target); os.IsNotExist(err) {
		if err := writeProjectRelative(projectPath, workspace.EvidenceIndex, evidenceIndexMarkdown(workspace)); err != nil {
			return err
		}
	}
	line := fmt.Sprintf("- [%s](%s) · `%s` · %s\n", entry.Title, filepath.Base(entry.RelativePath), entry.Kind, entry.Summary)
	return appendFile(projectPath, workspace.EvidenceIndex, line)
}

func appendEvidenceEvent(projectPath string, workspace Workspace, entry EvidenceEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return appendFile(projectPath, workspace.EvidenceEvents, string(data)+"\n")
}

func writeProjectRelative(projectPath string, relativePath string, content string) error {
	target := filepath.Clean(filepath.Join(projectPath, relativePath))
	root := filepath.Clean(projectPath)
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return fmt.Errorf("relative path escapes project: %s", relativePath)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, []byte(content), 0o644)
}

func normalizeEvidenceKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "command_output", "found_file", "payload_note", "screenshot", "pcap_analysis", "solver_output", "agent_output", "validation":
		return kind
	default:
		return "note"
	}
}

func evidenceID(entry EvidenceEntry, now time.Time) string {
	parts := []string{now.UTC().Format("20060102-150405")}
	if entry.StepKey != "" {
		parts = append(parts, entry.StepKey)
	}
	if entry.Kind != "" {
		parts = append(parts, entry.Kind)
	}
	if entry.Title != "" {
		parts = append(parts, entry.Title)
	}
	id := slugify(strings.Join(parts, "-"))
	if id == "" {
		return now.UTC().Format("20060102-150405")
	}
	return id
}

func firstNonEmptyLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "#*-` "))
		if line != "" {
			if len(line) > 180 {
				return line[:180] + "..."
			}
			return line
		}
	}
	return ""
}

func appendFile(projectPath string, relativePath string, content string) error {
	target := filepath.Clean(filepath.Join(projectPath, relativePath))
	root := filepath.Clean(projectPath)
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return fmt.Errorf("relative path escapes project: %s", relativePath)
	}
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
