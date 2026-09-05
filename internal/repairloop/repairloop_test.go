package repairloop

import (
	"testing"

	"zavod_ai/internal/checks"
	"zavod_ai/internal/reviews"
)

func TestReviewFromTestsRoutesFailedChecksToDeveloper(t *testing.T) {
	got, ok := ReviewFromTests([]checks.TestRun{{
		Command: "go test ./...",
		Status:  checks.StatusFailed,
		Error:   "exit code 1",
	}})
	if !ok {
		t.Fatal("expected test repair review")
	}
	if got.Status != reviews.StatusNeedsWork || got.ReturnTo != reviews.ReturnToDeveloper {
		t.Fatalf("expected developer repair, got %#v", got)
	}
	if len(got.Findings) == 0 || got.Findings[0].Message == "" {
		t.Fatalf("expected actionable finding, got %#v", got)
	}
}

func TestReviewFromTestsRoutesBlockedChecksToTester(t *testing.T) {
	got, ok := ReviewFromTests([]checks.TestRun{{
		Command: ".venv/bin/python missing.py",
		Status:  checks.StatusBlocked,
		Error:   "Python-скрипт не найден",
	}})
	if !ok {
		t.Fatal("expected test repair review")
	}
	if got.Status != reviews.StatusNeedsWork || got.ReturnTo != reviews.ReturnToTester {
		t.Fatalf("expected tester repair, got %#v", got)
	}
}

func TestReviewFromTestsKeepsMissingSecretAsHumanBlocker(t *testing.T) {
	got, ok := ReviewFromTests([]checks.TestRun{{
		Command: ".venv/bin/python bot.py",
		Status:  checks.StatusFailed,
		Error:   "TELEGRAM_TOKEN missing token",
	}})
	if !ok {
		t.Fatal("expected test repair review")
	}
	if got.Status != reviews.StatusBlocked || got.ReturnTo != reviews.ReturnToUser {
		t.Fatalf("expected missing secret to require user, got %#v", got)
	}
}

func TestNormalizeReviewKeepsRealHumanBlockers(t *testing.T) {
	got := NormalizeReviewForAutopilot(reviews.ParsedReview{
		Status:         reviews.StatusBlocked,
		ReturnTo:       reviews.ReturnToUser,
		BlockingReason: "нет scope и подтверждения владельца цели",
	})
	if got.Status != reviews.StatusBlocked || got.ReturnTo != reviews.ReturnToUser {
		t.Fatalf("expected human blocker to stay blocked, got %#v", got)
	}
}

func TestNormalizeReviewConvertsFixableBlockedToRepair(t *testing.T) {
	got := NormalizeReviewForAutopilot(reviews.ParsedReview{
		Status:   reviews.StatusBlocked,
		ReturnTo: reviews.ReturnToUser,
		Summary:  "Синтаксическая ошибка в check.go, go test ./... падает",
	})
	if got.Status != reviews.StatusNeedsWork || got.ReturnTo != reviews.ReturnToDeveloper {
		t.Fatalf("expected fixable blocked review to continue via developer, got %#v", got)
	}
}

func TestNormalizeReviewRoutesBadCheckCommandsToTester(t *testing.T) {
	got := NormalizeReviewForAutopilot(reviews.ParsedReview{
		Status:   reviews.StatusBlocked,
		ReturnTo: reviews.ReturnToUser,
		Summary:  "Команда проверки заблокирована policy и не применима к проекту",
	})
	if got.Status != reviews.StatusNeedsWork || got.ReturnTo != reviews.ReturnToTester {
		t.Fatalf("expected tester route, got %#v", got)
	}
}
