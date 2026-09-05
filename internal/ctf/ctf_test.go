package ctf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyCategories(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{name: "web", message: "CTF web challenge with cookies", want: CategoryWeb},
		{name: "lfi", message: "LFI path traversal ../etc/passwd", want: CategoryLFI},
		{name: "rce", message: "RCE command injection lab", want: CategoryRCE},
		{name: "sqli", message: "blind SQLi union select", want: CategorySQLi},
		{name: "pwn", message: "pwn ret2win buffer overflow", want: CategoryPwn},
		{name: "crypto", message: "crypto RSA oracle", want: CategoryCrypto},
		{name: "reverse", message: "reverse with ghidra", want: CategoryReverse},
		{name: "forensics", message: "forensics pcap wireshark", want: CategoryForensics},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.message); got != tt.want {
				t.Fatalf("Classify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScopeStatusRequiresScopeForExternalNetworkTarget(t *testing.T) {
	status, requiresScope, allowed := ScopeStatus("check RCE on https://example.com", CategoryRCE)
	if status != "needs_scope" || !requiresScope {
		t.Fatalf("ScopeStatus() = %q, %t; want needs_scope, true", status, requiresScope)
	}
	if len(allowed) == 0 {
		t.Fatal("expected allowed passive actions")
	}
}

func TestScopeStatusAllowsExplicitCTF(t *testing.T) {
	status, requiresScope, _ := ScopeStatus("solve HackTheBox SQLi challenge at http://box.htb", CategorySQLi)
	if status != "ctf_or_lab_scope" || requiresScope {
		t.Fatalf("ScopeStatus() = %q, %t; want ctf_or_lab_scope, false", status, requiresScope)
	}
}

func TestToolProfileIDByCategory(t *testing.T) {
	tests := map[string]string{
		CategoryWeb:       "tool_ctf_web",
		CategoryLFI:       "tool_ctf_lfi",
		CategoryRCE:       "tool_ctf_rce",
		CategorySQLi:      "tool_ctf_sqli",
		CategoryPwn:       "tool_ctf_pwn",
		CategoryCrypto:    "tool_ctf_crypto",
		CategoryReverse:   "tool_ctf_reverse",
		CategoryForensics: "tool_ctf_forensics",
	}
	for category, want := range tests {
		if got := ToolProfileID(category); got != want {
			t.Fatalf("ToolProfileID(%q) = %q, want %q", category, got, want)
		}
	}
}

func TestPrepareWorkspaceCreatesCTFFiles(t *testing.T) {
	root := t.TempDir()
	workspace, err := PrepareWorkspace(root, "Baby SQLi", "solve SQLi CTF", CategorySQLi, time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Category != CategorySQLi {
		t.Fatalf("category = %q, want %q", workspace.Category, CategorySQLi)
	}
	for _, relativePath := range []string{
		workspace.ChallengeYAML,
		workspace.ScopeMD,
		workspace.NotesMD,
		workspace.EvidenceIndex,
		workspace.EvidenceEvents,
		filepath.Join(workspace.SolveDir, "README.md"),
		workspace.WriteupMD,
	} {
		if _, err := os.Stat(filepath.Join(root, relativePath)); err != nil {
			t.Fatalf("expected %s: %v", relativePath, err)
		}
	}
}

func TestRecordEvidenceWritesEntryIndexAndJSONL(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 5, 12, 34, 56, 0, time.UTC)
	workspace, err := PrepareWorkspace(root, "Baby pwn", "solve pwn CTF", CategoryPwn, now)
	if err != nil {
		t.Fatal(err)
	}

	entry, err := RecordEvidence(root, workspace, EvidenceEntry{
		Kind:    "solver_output",
		Title:   "Pwntools solver result",
		AgentID: "ctf_pwn",
		StepKey: "category_solver",
		Content: "flag{demo}\n",
		Metadata: map[string]string{
			"tool_profile": "tool_ctf_pwn",
		},
	}, now)
	if err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	if entry.RelativePath == "" || !strings.HasPrefix(entry.RelativePath, filepath.ToSlash(workspace.EvidenceDir)+"/") {
		t.Fatalf("unexpected evidence path: %#v", entry)
	}
	content, err := os.ReadFile(filepath.Join(root, entry.RelativePath))
	if err != nil {
		t.Fatalf("read evidence entry: %v", err)
	}
	if !strings.Contains(string(content), "flag{demo}") || !strings.Contains(string(content), "tool_ctf_pwn") {
		t.Fatalf("expected evidence content and metadata, got %s", string(content))
	}
	index, err := os.ReadFile(filepath.Join(root, workspace.EvidenceIndex))
	if err != nil {
		t.Fatalf("read evidence index: %v", err)
	}
	if !strings.Contains(string(index), "Pwntools solver result") || !strings.Contains(string(index), filepath.Base(entry.RelativePath)) {
		t.Fatalf("expected index link, got %s", string(index))
	}
	events, err := os.ReadFile(filepath.Join(root, workspace.EvidenceEvents))
	if err != nil {
		t.Fatalf("read evidence events: %v", err)
	}
	if !strings.Contains(string(events), `"kind":"solver_output"`) || !strings.Contains(string(events), `"relativePath"`) {
		t.Fatalf("expected jsonl event, got %s", string(events))
	}
}
