package reviewgate

import (
	"strings"
	"testing"

	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/reviews"
	"zavod_ai/internal/taskspec"
)

func TestBuildFindsExtraFileOutsideBlueprint(t *testing.T) {
	report := Build(Input{
		TaskSpec: &taskspec.Spec{Goal: "CLI", Requirements: []string{"Go"}},
		Blueprint: &blueprint.Blueprint{ExpectedFiles: []blueprint.ExpectedFile{
			{Path: "main.go"},
		}},
		Changes: []changes.ProposedChange{{
			FilePath:     "debug.log",
			Status:       changes.StatusApplied,
			AfterContent: "debug\n",
			DiffText:     changes.GenerateUnifiedDiff("debug.log", "", "debug\n"),
		}},
	})
	if len(report.Findings) == 0 || report.Findings[0].Category != "diff" {
		t.Fatalf("expected diff finding for extra file, got %#v", report)
	}
}

func TestBuildFindsSecuritySecret(t *testing.T) {
	report := Build(Input{
		TaskSpec:  &taskspec.Spec{Goal: "bot", Requirements: []string{"env token"}},
		Blueprint: &blueprint.Blueprint{ExpectedFiles: []blueprint.ExpectedFile{{Path: "bot.py"}}},
		Changes: []changes.ProposedChange{{
			FilePath:     "bot.py",
			Status:       changes.StatusApplied,
			AfterContent: "BOT_TOKEN=\"123\"\n",
			DiffText:     changes.GenerateUnifiedDiff("bot.py", "", "BOT_TOKEN=\"123\"\n"),
		}},
	})
	if report.RecommendedRole != reviews.ReturnToDeveloper {
		t.Fatalf("expected developer route, got %#v", report)
	}
	if !strings.Contains(RenderPrompt(report), "security") {
		t.Fatalf("expected security gate prompt, got %s", RenderPrompt(report))
	}
}

func TestBuildRoutesBlockedChecksToTester(t *testing.T) {
	report := Build(Input{
		TaskSpec:  &taskspec.Spec{Goal: "CLI", Requirements: []string{"Go"}},
		Blueprint: &blueprint.Blueprint{Stack: blueprint.StackGo},
		Tests: []checks.TestRun{{
			Command: ".venv/bin/python missing.py",
			Status:  checks.StatusBlocked,
			Error:   "not applicable",
		}},
	})
	if report.RecommendedRole != reviews.ReturnToTester {
		t.Fatalf("expected tester route, got %#v", report)
	}
}

func TestEnforceConvertsAcceptedWithBlockingFindings(t *testing.T) {
	report := Build(Input{
		TaskSpec:  &taskspec.Spec{Goal: "CLI", Requirements: []string{"Go"}},
		Blueprint: &blueprint.Blueprint{ExpectedFiles: []blueprint.ExpectedFile{{Path: "main.go"}}},
		Changes: []changes.ProposedChange{{
			FilePath:     "main.go",
			Status:       changes.StatusApplied,
			AfterContent: "--- /dev/null\n+++ b/main.go\n@@\n+bad\n",
			DiffText:     changes.GenerateUnifiedDiff("main.go", "", "--- /dev/null\n+++ b/main.go\n@@\n+bad\n"),
		}},
	})
	got := Enforce(reviews.ParsedReview{Status: reviews.StatusAccepted, Summary: "ok"}, report)
	if got.Status != reviews.StatusNeedsWork || got.ReturnTo != reviews.ReturnToDeveloper {
		t.Fatalf("expected enforced needs_work, got %#v", got)
	}
	if len(got.Findings) == 0 || len(got.RequiredChanges) == 0 {
		t.Fatalf("expected findings and required changes, got %#v", got)
	}
}
