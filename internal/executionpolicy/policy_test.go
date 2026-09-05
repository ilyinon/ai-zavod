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

func TestCTFToolProfilesUseCategoryAllowlists(t *testing.T) {
	cases := []struct {
		profile string
		command string
		want    string
	}{
		{ToolCTFWeb, ".venv/bin/python solve/check.py", DecisionAuto},
		{ToolCTFWeb, "curl http://127.0.0.1:8080", DecisionConfirm},
		{ToolCTFWeb, "sqlmap -u http://127.0.0.1:8080", DecisionDeny},
		{ToolCTFSQLi, "sqlmap -u http://127.0.0.1:8080", DecisionConfirm},
		{ToolCTFPwn, "checksec chall", DecisionAuto},
		{ToolCTFPwn, "readelf -h chall", DecisionAuto},
		{ToolCTFPwn, "gdb chall", DecisionConfirm},
		{ToolCTFPwn, "binwalk firmware.bin", DecisionDeny},
		{ToolCTFCrypto, ".venv/bin/python solve/crypto_solver.py", DecisionAuto},
		{ToolCTFCrypto, "sage solve.sage", DecisionConfirm},
		{ToolCTFReverse, "objdump -d chall", DecisionAuto},
		{ToolCTFReverse, "radare2 chall", DecisionConfirm},
		{ToolCTFForensics, "exiftool image.jpg", DecisionAuto},
		{ToolCTFForensics, "binwalk firmware.bin", DecisionAuto},
		{ToolCTFForensics, "binwalk -e firmware.bin", DecisionConfirm},
	}
	for _, tt := range cases {
		got := EvaluateToolProfile(tt.profile, tt.command)
		if got.Decision != tt.want {
			t.Fatalf("EvaluateToolProfile(%q, %q) = %#v, want %s", tt.profile, tt.command, got, tt.want)
		}
		if got.ToolProfileID != tt.profile {
			t.Fatalf("expected tool profile id %q, got %#v", tt.profile, got)
		}
	}
}

func TestCTFToolProfilesExposeAllCategories(t *testing.T) {
	got := CTFToolProfiles()
	if len(got) != 8 {
		t.Fatalf("expected eight CTF tool profiles, got %#v", got)
	}
	seen := map[string]bool{}
	for _, profile := range got {
		seen[profile.Context] = len(profile.Auto) > 0 && len(profile.Deny) > 0
	}
	for _, profileID := range []string{ToolCTFWeb, ToolCTFLFI, ToolCTFRCE, ToolCTFSQLi, ToolCTFPwn, ToolCTFCrypto, ToolCTFReverse, ToolCTFForensics} {
		if !seen[profileID] {
			t.Fatalf("expected CTF profile %s, got %#v", profileID, got)
		}
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
