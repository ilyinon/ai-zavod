package taskspec

import (
	"fmt"
	"strings"
)

const (
	StatusDraft                = "draft"
	StatusActive               = "active"
	StatusWaitingClarification = "waiting_clarification"
	StatusDone                 = "done"
	StatusBlocked              = "blocked"
)

type AcceptedAnswer struct {
	QuestionID string `json:"questionId"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

type Spec struct {
	ID                 string           `json:"id"`
	ProjectID          string           `json:"projectId"`
	TaskID             string           `json:"taskId"`
	WorkflowRunID      string           `json:"workflowRunId"`
	UserRequest        string           `json:"userRequest"`
	Summary            string           `json:"summary"`
	Goal               string           `json:"goal"`
	Requirements       []string         `json:"requirements"`
	AcceptanceCriteria []string         `json:"acceptanceCriteria"`
	Decisions          []string         `json:"decisions"`
	OpenQuestions      []string         `json:"openQuestions"`
	AcceptedAnswers    []AcceptedAnswer `json:"acceptedAnswers"`
	Status             string           `json:"status"`
	Source             string           `json:"source"`
	CreatedAt          string           `json:"createdAt"`
	UpdatedAt          string           `json:"updatedAt"`
}

func RenderMarkdown(spec Spec) string {
	var builder strings.Builder
	builder.WriteString("# Task Spec\n\n")
	writeSection(&builder, "User Request", firstNonEmpty(spec.UserRequest, spec.Summary, "Не зафиксирован."))
	writeSection(&builder, "Goal", firstNonEmpty(spec.Goal, spec.Summary, spec.UserRequest, "Не зафиксирована."))
	writeListSection(&builder, "Requirements", spec.Requirements, "Требования пока не выделены.")
	writeListSection(&builder, "Acceptance Criteria", spec.AcceptanceCriteria, "Критерии готовности пока не выделены.")
	writeListSection(&builder, "Decisions", spec.Decisions, "Решения пока не зафиксированы.")
	writeListSection(&builder, "Open Questions", spec.OpenQuestions, "Открытых вопросов нет.")
	writeAnswersSection(&builder, spec.AcceptedAnswers)
	writeSection(&builder, "Status", firstNonEmpty(spec.Status, StatusDraft))
	if strings.TrimSpace(spec.WorkflowRunID) != "" {
		writeSection(&builder, "Workflow Run", "`"+strings.TrimSpace(spec.WorkflowRunID)+"`")
	}
	return strings.TrimSpace(builder.String()) + "\n"
}

func MergeStringLists(base []string, next []string) []string {
	out := make([]string, 0, len(base)+len(next))
	seen := map[string]bool{}
	for _, item := range append(base, next...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func MergeAnswers(base []AcceptedAnswer, next []AcceptedAnswer) []AcceptedAnswer {
	out := make([]AcceptedAnswer, 0, len(base)+len(next))
	seen := map[string]bool{}
	for _, item := range append(base, next...) {
		item.QuestionID = strings.TrimSpace(item.QuestionID)
		item.Question = strings.TrimSpace(item.Question)
		item.Answer = strings.TrimSpace(item.Answer)
		if item.Answer == "" {
			continue
		}
		key := firstNonEmpty(item.QuestionID, item.Question, item.Answer)
		key = strings.ToLower(key)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func writeSection(builder *strings.Builder, title string, content string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(content))
	builder.WriteString("\n\n")
}

func writeListSection(builder *strings.Builder, title string, items []string, fallback string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	clean := MergeStringLists(nil, items)
	if len(clean) == 0 {
		builder.WriteString(fallback)
		builder.WriteString("\n\n")
		return
	}
	for _, item := range clean {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func writeAnswersSection(builder *strings.Builder, answers []AcceptedAnswer) {
	builder.WriteString("## Accepted User Answers\n\n")
	clean := MergeAnswers(nil, answers)
	if len(clean) == 0 {
		builder.WriteString("Принятых ответов пользователя пока нет.\n\n")
		return
	}
	for index, item := range clean {
		builder.WriteString(fmt.Sprintf("%d. ", index+1))
		if item.Question != "" {
			builder.WriteString(item.Question)
			builder.WriteString("\n   Answer: ")
		}
		builder.WriteString(item.Answer)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
