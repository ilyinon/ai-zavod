package ctf

import (
	"os"
	"path/filepath"
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
		filepath.Join(workspace.SolveDir, "README.md"),
		workspace.WriteupMD,
	} {
		if _, err := os.Stat(filepath.Join(root, relativePath)); err != nil {
			t.Fatalf("expected %s: %v", relativePath, err)
		}
	}
}
