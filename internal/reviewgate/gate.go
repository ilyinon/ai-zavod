package reviewgate

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/reviews"
	"zavod_ai/internal/taskspec"
)

type Check struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Report struct {
	Checks          []Check           `json:"checks"`
	Findings        []reviews.Finding `json:"findings"`
	RecommendedRole string            `json:"recommendedRole"`
}

type Input struct {
	TaskSpec  *taskspec.Spec
	Blueprint *blueprint.Blueprint
	Changes   []changes.ProposedChange
	Tests     []checks.TestRun
}

func Build(input Input) Report {
	report := Report{
		Checks: []Check{
			{Key: "spec", Title: "Task spec", Status: "passed", Detail: "спека доступна"},
			{Key: "blueprint", Title: "Blueprint", Status: "passed", Detail: "blueprint доступен"},
			{Key: "diff", Title: "Diff", Status: "passed", Detail: "примененные изменения есть"},
			{Key: "tests", Title: "Tests", Status: "passed", Detail: "последние проверки прошли"},
			{Key: "security", Title: "Security", Status: "passed", Detail: "секреты и опасные команды не обнаружены"},
			{Key: "quality", Title: "Quality", Status: "passed", Detail: "грубые признаки некачественного вывода не обнаружены"},
		},
	}

	expected := expectedFiles(input.Blueprint)
	if input.TaskSpec == nil || (strings.TrimSpace(input.TaskSpec.Goal) == "" && len(input.TaskSpec.Requirements) == 0 && len(input.TaskSpec.AcceptanceCriteria) == 0) {
		report.warn("spec", reviews.Finding{Category: "spec", Severity: "minor", Message: "Task spec неполная или не найдена", Suggestion: "Вернуть к Продакту, если требования нельзя проверить по текущему контексту."})
	}
	if input.Blueprint == nil || (strings.TrimSpace(input.Blueprint.Stack) == "" && len(input.Blueprint.ExpectedFiles) == 0) {
		report.fail("blueprint", reviews.Finding{Category: "blueprint", Severity: "major", Message: "Task Blueprint отсутствует или пустой", Suggestion: "Вернуть Архитектору для пересборки blueprint."})
	}

	applied := 0
	for _, change := range input.Changes {
		path := cleanPath(change.FilePath)
		switch change.Status {
		case changes.StatusApplied:
			applied++
			if len(expected) > 0 && !expected[path] {
				report.warn("diff", reviews.Finding{Category: "diff", Severity: "minor", FilePath: path, Message: "Файл изменен, но не указан в Task Blueprint", Suggestion: "Вернуть Архитектору, если файл действительно должен входить в задачу; иначе убрать лишнее изменение."})
			}
			if strings.TrimSpace(change.DiffText) == "" && change.BeforeContent != change.AfterContent {
				report.fail("diff", reviews.Finding{Category: "diff", Severity: "major", FilePath: path, Message: "Для примененного изменения отсутствует diff", Suggestion: "Пересохранить изменение с корректным unified diff."})
			}
			if looksLikePatchWrittenAsContent(change.AfterContent) || looksLikeEscapedNewlineFile(change.AfterContent) {
				report.fail("quality", reviews.Finding{Category: "quality", Severity: "critical", FilePath: path, Message: "Файл выглядит как записанный patch/escaped текст, а не нормальный исходный файл", Suggestion: "Вернуть Разработчику и записать реальное содержимое файла."})
			}
			if secretFinding := secretInContent(path, change.AfterContent); secretFinding.Message != "" {
				report.fail("security", secretFinding)
			}
		case changes.StatusFailed:
			report.fail("diff", reviews.Finding{Category: "diff", Severity: "major", FilePath: path, Message: "Изменение не применено: " + change.Error, Suggestion: "Вернуть Разработчику для исправления proposed changes."})
		case changes.StatusPending:
			report.fail("diff", reviews.Finding{Category: "diff", Severity: "major", FilePath: path, Message: "Есть непримененное изменение", Suggestion: "Применить безопасное изменение или вернуть Разработчику."})
		}
	}
	if applied == 0 && len(input.Changes) > 0 {
		report.fail("diff", reviews.Finding{Category: "diff", Severity: "major", Message: "Нет примененных изменений", Suggestion: "Вернуть Разработчику."})
	}
	if applied == 0 && len(expected) > 0 {
		report.fail("diff", reviews.Finding{Category: "diff", Severity: "major", Message: "Task Blueprint ожидает изменения файлов, но примененного diff нет", Suggestion: "Вернуть Разработчику для подготовки proposed changes."})
	}
	if applied > 0 && len(input.Tests) == 0 {
		report.fail("tests", reviews.Finding{Category: "tests", Severity: "major", Message: "После изменений не запущены проверки", Suggestion: "Вернуть Тестировщику для выбора и запуска применимых проверок."})
	}

	for _, testRun := range latestTests(input.Tests) {
		switch testRun.Status {
		case checks.StatusFailed:
			report.fail("tests", reviews.Finding{Category: "tests", Severity: "major", Message: fmt.Sprintf("%s завершилась ошибкой: %s", commandLabel(testRun), strings.TrimSpace(testRun.Error)), Suggestion: "Вернуть Разработчику для исправления причины падения."})
		case checks.StatusBlocked:
			report.fail("tests", reviews.Finding{Category: "tests", Severity: "major", Message: fmt.Sprintf("%s заблокирована: %s", commandLabel(testRun), strings.TrimSpace(testRun.Error)), Suggestion: "Вернуть Тестировщику для выбора применимой auto-проверки."})
		case checks.StatusPending, checks.StatusRunning:
			report.fail("tests", reviews.Finding{Category: "tests", Severity: "major", Message: commandLabel(testRun) + " не завершена", Suggestion: "Вернуть Тестировщику и дождаться финального результата проверки."})
		}
		if dangerousTestCommand(testRun.Command) {
			report.fail("security", reviews.Finding{Category: "security", Severity: "critical", Message: "Проверка содержит опасную команду: " + testRun.Command, Suggestion: "Вернуть Тестировщику и выбрать команду из Code Execution Policy."})
		}
	}

	report.RecommendedRole = recommendedRole(report.Findings)
	return report
}

func Enforce(review reviews.ParsedReview, report Report) reviews.ParsedReview {
	if review.Status != reviews.StatusAccepted || !hasBlockingFindings(report.Findings) {
		return review
	}
	review.Status = reviews.StatusNeedsWork
	review.ReturnTo = report.RecommendedRole
	if review.ReturnTo == "" {
		review.ReturnTo = reviews.ReturnToDeveloper
	}
	review.Summary = "Review Gate 2.0 нашел обязательные замечания, поэтому работа не может быть принята."
	review.Findings = mergeFindings(review.Findings, report.Findings)
	review.RequiredChanges = mergeRequired(review.RequiredChanges, report.Findings)
	review.RecommendedNextStep = "Вернуть к роли: " + review.ReturnTo
	return review
}

func RenderPrompt(report Report) string {
	var builder strings.Builder
	builder.WriteString("# Review Gate 2.0 checklist\n")
	for _, check := range report.Checks {
		builder.WriteString("- ")
		builder.WriteString(check.Title)
		builder.WriteString(": ")
		builder.WriteString(check.Status)
		if check.Detail != "" {
			builder.WriteString(" — ")
			builder.WriteString(check.Detail)
		}
		builder.WriteString("\n")
	}
	if len(report.Findings) == 0 {
		builder.WriteString("\nDeterministic findings: none.\n")
		return builder.String()
	}
	builder.WriteString("\nDeterministic findings:\n")
	for _, finding := range report.Findings {
		builder.WriteString("- ")
		if finding.Category != "" {
			builder.WriteString(finding.Category)
			builder.WriteString(" · ")
		}
		builder.WriteString(finding.Severity)
		if finding.FilePath != "" {
			builder.WriteString(" · `")
			builder.WriteString(finding.FilePath)
			builder.WriteString("`")
		}
		builder.WriteString(" — ")
		builder.WriteString(finding.Message)
		if finding.Suggestion != "" {
			builder.WriteString(" / ")
			builder.WriteString(finding.Suggestion)
		}
		builder.WriteString("\n")
	}
	if report.RecommendedRole != "" {
		builder.WriteString("\nRecommended return_to: ")
		builder.WriteString(report.RecommendedRole)
		builder.WriteString("\n")
	}
	return builder.String()
}

func (r *Report) fail(key string, finding reviews.Finding) {
	r.setCheck(key, "failed", finding.Message)
	r.Findings = append(r.Findings, finding)
}

func (r *Report) warn(key string, finding reviews.Finding) {
	if r.checkStatus(key) != "failed" {
		r.setCheck(key, "warning", finding.Message)
	}
	r.Findings = append(r.Findings, finding)
}

func (r *Report) setCheck(key string, status string, detail string) {
	for index := range r.Checks {
		if r.Checks[index].Key == key {
			r.Checks[index].Status = status
			r.Checks[index].Detail = strings.TrimSpace(detail)
			return
		}
	}
	r.Checks = append(r.Checks, Check{Key: key, Title: key, Status: status, Detail: detail})
}

func (r Report) checkStatus(key string) string {
	for _, check := range r.Checks {
		if check.Key == key {
			return check.Status
		}
	}
	return ""
}

func expectedFiles(taskBlueprint *blueprint.Blueprint) map[string]bool {
	if taskBlueprint == nil {
		return nil
	}
	out := map[string]bool{}
	for _, item := range taskBlueprint.ExpectedFiles {
		if path := cleanPath(item.Path); path != "" {
			out[path] = true
		}
	}
	return out
}

func cleanPath(path string) string {
	return filepath.ToSlash(strings.Trim(path, "/"))
}

func latestTests(items []checks.TestRun) []checks.TestRun {
	out := make([]checks.TestRun, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		key := strings.TrimSpace(item.WorkingDir) + "\x00" + strings.TrimSpace(item.Command)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func commandLabel(testRun checks.TestRun) string {
	if strings.TrimSpace(testRun.WorkingDir) == "" {
		return testRun.Command
	}
	return testRun.WorkingDir + " $ " + testRun.Command
}

func looksLikePatchWrittenAsContent(content string) bool {
	trimmed := strings.TrimSpace(content)
	return strings.HasPrefix(trimmed, "--- /dev/null\n+++ ") || strings.HasPrefix(trimmed, "--- a/")
}

func looksLikeEscapedNewlineFile(content string) bool {
	return strings.Count(content, `\n`) >= 5 && strings.Count(content, "\n") <= 2
}

func secretInContent(path string, content string) reviews.Finding {
	lowerPath := strings.ToLower(path)
	lowerContent := strings.ToLower(content)
	if lowerPath == ".env" || strings.HasSuffix(lowerPath, "/.env") {
		return reviews.Finding{Category: "security", Severity: "critical", FilePath: path, Message: ".env не должен попадать в proposed changes", Suggestion: "Использовать .env.example без реальных секретов."}
	}
	if strings.Contains(lowerContent, "-----begin ") && strings.Contains(lowerContent, " private key-----") {
		return reviews.Finding{Category: "security", Severity: "critical", FilePath: path, Message: "В файле похожий на приватный ключ секрет", Suggestion: "Удалить секрет и использовать переменную окружения."}
	}
	for _, needle := range []string{"api_key=", "apikey=", "token=", "password=", "secret=", "bot_token="} {
		if strings.Contains(lowerContent, needle) {
			return reviews.Finding{Category: "security", Severity: "critical", FilePath: path, Message: "В файле похожий на захардкоженный секрет", Suggestion: "Передавать секрет через env и добавить .env.example."}
		}
	}
	return reviews.Finding{}
}

func dangerousTestCommand(command string) bool {
	lower := strings.ToLower(command)
	for _, token := range []string{" rm ", "rm -", "sudo", "docker", "kubectl", "helm", "nmap", "masscan", "hydra", "curl "} {
		if strings.Contains(" "+lower+" ", token) {
			return true
		}
	}
	return strings.ContainsAny(command, ";&|<>`")
}

func hasBlockingFindings(items []reviews.Finding) bool {
	for _, item := range items {
		if item.Severity == "critical" || item.Severity == "major" {
			return true
		}
	}
	return false
}

func recommendedRole(items []reviews.Finding) string {
	highest := ""
	highestRank := -1
	for _, item := range items {
		role := roleForFinding(item)
		rank := severityRank(item.Severity)
		if roleRank(role) > roleRank(highest) || roleRank(role) == roleRank(highest) && rank > highestRank {
			highest = role
			highestRank = rank
		}
	}
	if highest == "" {
		return reviews.ReturnToDeveloper
	}
	return highest
}

func roleForFinding(item reviews.Finding) string {
	switch item.Category {
	case "spec":
		return reviews.ReturnToProduct
	case "blueprint":
		return reviews.ReturnToArchitect
	case "tests":
		if strings.Contains(strings.ToLower(item.Suggestion+" "+item.Message), "тестиров") {
			return reviews.ReturnToTester
		}
		return reviews.ReturnToDeveloper
	case "security", "quality", "diff":
		return reviews.ReturnToDeveloper
	default:
		return reviews.ReturnToDeveloper
	}
}

func severityRank(value string) int {
	switch value {
	case "critical":
		return 3
	case "major":
		return 2
	case "minor":
		return 1
	default:
		return 0
	}
}

func roleRank(value string) int {
	switch value {
	case reviews.ReturnToDeveloper:
		return 4
	case reviews.ReturnToTester:
		return 3
	case reviews.ReturnToArchitect:
		return 2
	case reviews.ReturnToProduct:
		return 1
	default:
		return 0
	}
}

func mergeFindings(left []reviews.Finding, right []reviews.Finding) []reviews.Finding {
	out := append([]reviews.Finding{}, left...)
	out = append(out, right...)
	return out
}

func mergeRequired(existing []string, findings []reviews.Finding) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(existing)+len(findings))
	for _, item := range existing {
		item = strings.TrimSpace(item)
		if item != "" && !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	for _, finding := range findings {
		item := strings.TrimSpace(finding.Suggestion)
		if item == "" {
			item = strings.TrimSpace(finding.Message)
		}
		if item != "" && !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}
