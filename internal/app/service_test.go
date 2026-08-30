package app

import (
	"strings"
	"testing"

	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/reviews"
)

func TestLooksLikeRepetitionLoopDetectsRepeatedAgentPrefix(t *testing.T) {
	text := strings.Repeat("Агент manager: ", repetitionLoopMinRepeats)
	if !looksLikeRepetitionLoop(text) {
		t.Fatal("expected repeated agent prefix to be detected")
	}
}

func TestLooksLikeRepetitionLoopAllowsNormalAnswer(t *testing.T) {
	text := "Поняла задачу. Следующий шаг: уточнить цель, входные данные и критерии готовности."
	if looksLikeRepetitionLoop(text) {
		t.Fatal("normal answer was detected as repetition loop")
	}
}

func TestManagerNeedsClarificationFromJSON(t *testing.T) {
	output := `{"summary":"мало данных","needs_clarification":true}`
	if !managerNeedsClarification(output) {
		t.Fatal("expected clarification flag from json")
	}
}

func TestFormatClarificationMessageIsHumanReadable(t *testing.T) {
	message := formatClarificationMessage(managerIntakeResult{
		Summary:       "нужно разработать проверку локальной LLM",
		Goal:          "понять, доступна ли модель",
		OpenQuestions: []string{"Какой endpoint проверять?", "Какие критерии успеха?"},
	})

	if strings.Contains(message, `"summary"`) || strings.Contains(message, "needs_clarification") {
		t.Fatalf("expected human-readable message, got %q", message)
	}
	if !strings.Contains(message, "Поняла задачу") || !strings.Contains(message, "Что нужно уточнить") {
		t.Fatalf("expected clarification sections, got %q", message)
	}
}

func TestFilterRelevantTestSuggestionsKeepsOnlyChangedStack(t *testing.T) {
	suggestions := []checks.Suggestion{
		{Command: "go test ./...", Reason: "Go"},
		{Command: "python3 check.py", Reason: "Python"},
	}
	applied := []changes.ProposedChange{
		{FilePath: "check.go", Status: changes.StatusApplied},
		{FilePath: "main.go", Status: changes.StatusApplied},
	}

	got := filterRelevantTestSuggestions(suggestions, applied)
	if len(got) != 1 || got[0].Command != "go test ./..." {
		t.Fatalf("expected only Go test suggestion, got %#v", got)
	}
}

func TestLatestTestRunsByCommandKeepsNewestAttempt(t *testing.T) {
	items := []checks.TestRun{
		{Command: "go test ./...", Status: checks.StatusPassed, CreatedAt: "2026-08-30T12:02:00Z"},
		{Command: "go test ./...", Status: checks.StatusFailed, CreatedAt: "2026-08-30T12:01:00Z"},
		{Command: "npm run build", WorkingDir: "frontend", Status: checks.StatusPassed, CreatedAt: "2026-08-30T12:00:00Z"},
	}

	got := latestTestRunsByCommand(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 latest command results, got %#v", got)
	}
	if got[0].Command != "go test ./..." || got[0].Status != checks.StatusPassed {
		t.Fatalf("expected latest Go result to be passed, got %#v", got[0])
	}
}

func TestWantsSavedTaskSpec(t *testing.T) {
	if !wantsSavedTaskSpec("выведи спеку проекта по которой ты написал этот код") {
		t.Fatal("expected saved task spec request to be detected")
	}
	if wantsSavedTaskSpec("распиши спеку для следующего шага") {
		t.Fatal("new spec authoring request should not be treated as saved spec output")
	}
}

func TestDeterministicBlockedFinalIncludesReviewDetails(t *testing.T) {
	message := deterministicBlockedFinal(autopilotResult{
		AppliedFiles:   2,
		TestsPassed:    1,
		ReviewStatus:   reviews.StatusNeedsWork,
		ReviewReturnTo: reviews.ReturnToDeveloper,
		ReviewSummary:  "не выполнено требование по CLI параметру",
		ReviewRequired: []string{"Добавить обработку аргумента адреса сайта"},
		ReviewFindings: []reviews.Finding{
			{FilePath: "main.go", Message: "адрес не передается в CheckSite", Suggestion: "прочитать os.Args[1]"},
		},
		Iterations:  3,
		Blocked:     true,
		BlockReason: "исчерпан лимит repair-итераций: последнее ревью все еще требует доработку",
	})

	for _, want := range []string{"Причина ревью", "Что не принято ревьюером", "Добавить обработку аргумента", "main.go"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in blocked final message, got %q", want, message)
		}
	}
}

func TestPythonRequirementsContentFromBlueprintDependencies(t *testing.T) {
	got := pythonRequirementsContent([]string{"python-telegram-bot", "requests", "python-telegram-bot"})
	if got != "python-telegram-bot\nrequests\n" {
		t.Fatalf("unexpected requirements content: %q", got)
	}
}

func TestBlueprintExpectedPath(t *testing.T) {
	items := []blueprint.ExpectedFile{{Path: "requirements.txt"}}
	if !blueprintExpectedPath(items, "requirements.txt") {
		t.Fatal("expected requirements.txt to be detected")
	}
}
