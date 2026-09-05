package executionpolicy

import "testing"

func TestDevPolicyAllowsOnlySafeAutoChecks(t *testing.T) {
	for _, command := range []string{
		"go test ./...",
		"go test ./internal/app",
		"go vet ./...",
		"npm test",
		"npm run build",
		".venv/bin/python check.py",
		".venv/bin/python -m pytest",
		".venv/bin/python -m py_compile src/check.py",
	} {
		got := Evaluate(ContextDev, command)
		if got.Decision != DecisionAuto {
			t.Fatalf("expected dev auto for %q, got %#v", command, got)
		}
	}
}

func TestDevPolicyRequiresConfirmationForMutableOrRuntimeCommands(t *testing.T) {
	for _, command := range []string{
		"go mod tidy",
		"go get github.com/google/uuid",
		"go run .",
		"npm install",
		"make build",
		"wails build",
	} {
		got := Evaluate(ContextDev, command)
		if got.Decision != DecisionConfirm {
			t.Fatalf("expected dev confirm for %q, got %#v", command, got)
		}
	}
}

func TestPolicyDeniesShellOperatorsAndForbiddenCommands(t *testing.T) {
	for _, command := range []string{
		"go test ./... && rm -rf .",
		"rm -rf .",
		"bash scripts/test.sh",
		"docker ps",
		"nmap 127.0.0.1",
	} {
		got := Evaluate(ContextDev, command)
		if got.Decision != DecisionDeny {
			t.Fatalf("expected deny for %q, got %#v", command, got)
		}
	}
}

func TestCTFPolicySeparatesLocalAutoFromScopedConfirm(t *testing.T) {
	auto := Evaluate(ContextCTF, "strings artifact.bin")
	if auto.Decision != DecisionAuto {
		t.Fatalf("expected local CTF auto, got %#v", auto)
	}
	confirm := Evaluate(ContextCTF, "curl http://127.0.0.1:8080")
	if confirm.Decision != DecisionConfirm {
		t.Fatalf("expected CTF confirm, got %#v", confirm)
	}
}

func TestResearchPolicyDeniesShellCommands(t *testing.T) {
	got := Evaluate(ContextResearch, "go test ./...")
	if got.Decision != DecisionDeny {
		t.Fatalf("expected research shell deny, got %#v", got)
	}
}

func TestProfilesExposeDevCTFResearch(t *testing.T) {
	got := Profiles()
	if len(got) != 3 {
		t.Fatalf("expected three policy profiles, got %#v", got)
	}
	seen := map[string]bool{}
	for _, profile := range got {
		seen[profile.Context] = len(profile.Auto) > 0 && len(profile.Deny) > 0
	}
	for _, contextName := range []string{ContextDev, ContextCTF, ContextResearch} {
		if !seen[contextName] {
			t.Fatalf("expected profile %s, got %#v", contextName, got)
		}
	}
}
