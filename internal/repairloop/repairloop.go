package repairloop

import (
	"fmt"
	"strings"

	"zavod_ai/internal/checks"
	"zavod_ai/internal/reviews"
)

func ReviewFromTests(testRuns []checks.TestRun) (reviews.ParsedReview, bool) {
	latest := latestTestRunsByCommand(testRuns)
	failed := make([]checks.TestRun, 0, len(latest))
	blocked := make([]checks.TestRun, 0, len(latest))
	for _, testRun := range latest {
		switch testRun.Status {
		case checks.StatusFailed:
			failed = append(failed, testRun)
		case checks.StatusBlocked:
			blocked = append(blocked, testRun)
		}
	}
	if len(failed) == 0 && len(blocked) == 0 {
		return reviews.ParsedReview{}, false
	}

	if len(failed) > 0 {
		if blocker := humanBlockerFromTests(failed); blocker != "" {
			return reviews.ParsedReview{
				Status:         reviews.StatusBlocked,
				Summary:        "Проверки требуют данных, которых нет у Autopilot.",
				ReturnTo:       reviews.ReturnToUser,
				BlockingReason: blocker,
				RequiredChanges: []string{
					"Передать недостающий scope/секрет или изменить требование так, чтобы проверка могла быть выполнена без него.",
				},
				Findings: testFindings(failed, "failed"),
			}, true
		}
		return reviews.ParsedReview{
			Status:   reviews.StatusNeedsWork,
			Summary:  "Последние проверки упали. Autopilot возвращает задачу разработчику без вмешательства пользователя.",
			ReturnTo: reviews.ReturnToDeveloper,
			RequiredChanges: []string{
				"Исправить код по stdout/stderr/error последней упавшей проверки.",
				"Вернуть structured Proposed changes только для необходимых файлов.",
			},
			Findings: testFindings(failed, "failed"),
		}, true
	}

	return reviews.ParsedReview{
		Status:   reviews.StatusNeedsWork,
		Summary:  "Последние проверки были заблокированы policy или не подходят проекту. Autopilot возвращает задачу тестировщику для подбора корректных проверок.",
		ReturnTo: reviews.ReturnToTester,
		RequiredChanges: []string{
			"Подобрать команды только из auto-раздела Code Execution Policy.",
			"Не предлагать проверки, которые не соответствуют структуре проекта.",
		},
		Findings: testFindings(blocked, "blocked"),
	}, true
}

func humanBlockerFromTests(items []checks.TestRun) string {
	for _, item := range items {
		text := strings.ToLower(strings.TrimSpace(item.Command + "\n" + item.Error + "\n" + item.Stdout + "\n" + item.Stderr))
		if containsAny(text,
			"missing token", "token required", "no token", "токен не задан", "нет токена",
			"secret required", "missing secret", "секрет не задан", "нет секрета",
			"api key required", "missing api key", "api_key", "нет api key",
			"scope required", "нет scope", "нет разрешения", "authorization required",
			"permission denied by scope",
		) {
			return "нет scope/секрета для выполнения проверки"
		}
	}
	return ""
}

func NormalizeReviewForAutopilot(review reviews.ParsedReview) reviews.ParsedReview {
	review.ReturnTo = strings.TrimSpace(review.ReturnTo)
	if review.Status != reviews.StatusBlocked || review.ReturnTo != reviews.ReturnToUser {
		if review.Status == reviews.StatusNeedsWork && review.ReturnTo == "" {
			review.ReturnTo = reviews.ReturnToDeveloper
		}
		return review
	}
	if IsHumanBlocker(review) {
		return review
	}
	review.Status = reviews.StatusNeedsWork
	review.ReturnTo = routeFixableReview(review)
	review.BlockingReason = ""
	if strings.TrimSpace(review.Summary) == "" {
		review.Summary = "Ревьюер остановил workflow, но причина выглядит исправимой. Autopilot продолжает repair-loop."
	} else {
		review.Summary += " Autopilot трактует это как исправимую доработку и продолжает repair-loop."
	}
	return review
}

func IsHumanBlocker(review reviews.ParsedReview) bool {
	text := reviewText(review)
	return containsAny(text,
		"scope", "скоуп", "разрешени", "authorization", "authorized", "подтверждени", "владелец",
		"secret", "секрет", "token", "токен", "api key", "ключ api", "credential", "парол",
		"конфликт требован", "противореч", "несовместим", "невозможно выбрать требование",
		"модель недоступ", "provider недоступ", "quota", "лимит модели", "инфраструктур",
	)
}

func routeFixableReview(review reviews.ParsedReview) string {
	text := reviewText(review)
	switch {
	case containsAny(text, "требован", "acceptance", "критер", "спецификац", "spec"):
		return reviews.ReturnToProduct
	case containsAny(text, "blueprint", "архитект", "структур", "модул", "контракт"):
		return reviews.ReturnToArchitect
	case containsAny(text, "провер", "test", "pytest", "go test", "npm", "команд"):
		if containsAny(text, "неподходящ", "заблокирован", "policy", "allowlist", "не применим") {
			return reviews.ReturnToTester
		}
		return reviews.ReturnToDeveloper
	default:
		return reviews.ReturnToDeveloper
	}
}

func testFindings(items []checks.TestRun, status string) []reviews.Finding {
	out := make([]reviews.Finding, 0, len(items))
	for _, item := range items {
		message := fmt.Sprintf("%s: %s", item.Command, item.Error)
		if strings.TrimSpace(item.Error) == "" {
			message = item.Command + ": " + item.Status
		}
		out = append(out, reviews.Finding{
			Severity:   "major",
			Message:    strings.TrimSpace(message),
			Suggestion: testSuggestion(status),
		})
	}
	return out
}

func testSuggestion(status string) string {
	if status == "blocked" {
		return "Подобрать применимую auto-проверку по структуре проекта."
	}
	return "Исправить причину падения и повторить проверку."
}

func latestTestRunsByCommand(items []checks.TestRun) []checks.TestRun {
	latest := make([]checks.TestRun, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		key := strings.TrimSpace(item.WorkingDir) + "\x00" + strings.TrimSpace(item.Command)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		latest = append(latest, item)
	}
	return latest
}

func reviewText(review reviews.ParsedReview) string {
	var builder strings.Builder
	builder.WriteString(review.Summary)
	builder.WriteString("\n")
	builder.WriteString(review.BlockingReason)
	builder.WriteString("\n")
	builder.WriteString(review.RecommendedNextStep)
	for _, item := range review.RequiredChanges {
		builder.WriteString("\n")
		builder.WriteString(item)
	}
	for _, finding := range review.Findings {
		builder.WriteString("\n")
		builder.WriteString(finding.Message)
		builder.WriteString("\n")
		builder.WriteString(finding.Suggestion)
	}
	return strings.ToLower(builder.String())
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
