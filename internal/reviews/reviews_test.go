package reviews

import "testing"

func TestParseAcceptedReview(t *testing.T) {
	parsed, err := Parse(`{
		"status": "accepted",
		"summary": "Все ок",
		"findings": [],
		"required_changes": [],
		"recommended_next_step": "Можно закрывать"
	}`)
	if err != nil {
		t.Fatalf("parse review: %v", err)
	}
	if parsed.Status != StatusAccepted || parsed.Summary != "Все ок" {
		t.Fatalf("unexpected review: %#v", parsed)
	}
}

func TestParseNeedsWorkReview(t *testing.T) {
	parsed, err := Parse("```json\n" + `{
		"status": "needs_work",
		"summary": "Нужно исправить",
		"findings": [
			{"severity":"major","file_path":"main.go","message":"нет проверки","suggestion":"добавить проверку"}
		],
		"required_changes": ["добавить проверку"],
		"recommended_next_step": "Вернуть Разработчику"
	}` + "\n```")
	if err != nil {
		t.Fatalf("parse review: %v", err)
	}
	if parsed.Status != StatusNeedsWork || len(parsed.Findings) != 1 || parsed.Findings[0].Severity != "major" {
		t.Fatalf("unexpected review: %#v", parsed)
	}
	if parsed.ReturnTo != ReturnToDeveloper {
		t.Fatalf("expected default return_to developer, got %q", parsed.ReturnTo)
	}
}

func TestParseBlockedReview(t *testing.T) {
	parsed, err := Parse(`{
		"status": "blocked",
		"summary": "Нужен ответ пользователя",
		"blocking_reason": "Неясно, можно ли удалять файл",
		"return_to": "user"
	}`)
	if err != nil {
		t.Fatalf("parse review: %v", err)
	}
	if parsed.Status != StatusBlocked || parsed.ReturnTo != ReturnToUser {
		t.Fatalf("unexpected review route: %#v", parsed)
	}
	if parsed.BlockingReason != "Неясно, можно ли удалять файл" {
		t.Fatalf("unexpected blocking reason: %q", parsed.BlockingReason)
	}
}

func TestParseFixableBlockedReviewBecomesNeedsWork(t *testing.T) {
	parsed, err := Parse(`{
		"status": "blocked",
		"summary": "Компиляция упала",
		"return_to": "developer",
		"required_changes": ["исправить синтаксис"]
	}`)
	if err != nil {
		t.Fatalf("parse review: %v", err)
	}
	if parsed.Status != StatusNeedsWork || parsed.ReturnTo != ReturnToDeveloper {
		t.Fatalf("expected fixable blocked review to route to developer, got %#v", parsed)
	}
}

func TestParseRejectsBadStatus(t *testing.T) {
	if _, err := Parse(`{"status":"maybe","summary":"x"}`); err == nil {
		t.Fatal("expected bad status error")
	}
}
