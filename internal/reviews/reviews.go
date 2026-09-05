package reviews

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusAccepted  = "accepted"
	StatusNeedsWork = "needs_work"
	StatusBlocked   = "blocked"
	StatusFailed    = "failed"
)

const (
	ReturnToProduct   = "product"
	ReturnToArchitect = "architect"
	ReturnToDeveloper = "developer"
	ReturnToTester    = "tester"
	ReturnToUser      = "user"
)

type Finding struct {
	Category   string `json:"category,omitempty"`
	Severity   string `json:"severity"`
	FilePath   string `json:"file_path"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion"`
}

type ReviewRun struct {
	ID                  string    `json:"id"`
	ProjectID           string    `json:"projectId"`
	TaskID              string    `json:"taskId"`
	WorkflowRunID       string    `json:"workflowRunId"`
	Status              string    `json:"status"`
	Summary             string    `json:"summary"`
	Findings            []Finding `json:"findings"`
	RequiredChanges     []string  `json:"requiredChanges"`
	RecommendedNextStep string    `json:"recommendedNextStep"`
	ReturnTo            string    `json:"returnTo"`
	Iteration           int       `json:"iteration"`
	BlockingReason      string    `json:"blockingReason"`
	Error               string    `json:"error"`
	StartedAt           string    `json:"startedAt"`
	FinishedAt          string    `json:"finishedAt"`
	CreatedAt           string    `json:"createdAt"`
}

type ParsedReview struct {
	Status              string    `json:"status"`
	Summary             string    `json:"summary"`
	Findings            []Finding `json:"findings"`
	RequiredChanges     []string  `json:"required_changes"`
	RecommendedNextStep string    `json:"recommended_next_step"`
	ReturnTo            string    `json:"return_to"`
	BlockingReason      string    `json:"blocking_reason"`
}

func Parse(output string) (ParsedReview, error) {
	trimmed := stripCodeFence(output)
	if trimmed == "" {
		return ParsedReview{}, fmt.Errorf("ответ ревьюера пустой")
	}
	var parsed ParsedReview
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return ParsedReview{}, fmt.Errorf("ответ ревьюера не похож на JSON: %w", err)
	}
	parsed.Status = strings.TrimSpace(parsed.Status)
	if parsed.Status == "" {
		if len(parsed.Findings) > 0 || len(parsed.RequiredChanges) > 0 {
			parsed.Status = StatusNeedsWork
		} else {
			parsed.Status = StatusAccepted
		}
	}
	if parsed.Status != StatusAccepted && parsed.Status != StatusNeedsWork && parsed.Status != StatusBlocked {
		return ParsedReview{}, fmt.Errorf("неподдерживаемый статус ревью: %s", parsed.Status)
	}
	parsed.Summary = strings.TrimSpace(parsed.Summary)
	parsed.RecommendedNextStep = strings.TrimSpace(parsed.RecommendedNextStep)
	parsed.ReturnTo = normalizeReturnTo(parsed.ReturnTo, parsed.Status)
	parsed.BlockingReason = strings.TrimSpace(parsed.BlockingReason)
	if parsed.Status == StatusBlocked && parsed.ReturnTo != ReturnToUser {
		parsed.Status = StatusNeedsWork
	}
	parsed.Findings = normalizeFindings(parsed.Findings)
	parsed.RequiredChanges = normalizeStrings(parsed.RequiredChanges)
	if parsed.Summary == "" {
		parsed.Summary = "Ревью завершено."
	}
	if parsed.Status == StatusBlocked && parsed.BlockingReason == "" {
		parsed.BlockingReason = parsed.Summary
	}
	return parsed, nil
}

func FindingsToJSON(items []Finding) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func RequiredChangesToJSON(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func FindingsFromJSON(value string) []Finding {
	var items []Finding
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &items); err != nil {
		return nil
	}
	return normalizeFindings(items)
}

func RequiredChangesFromJSON(value string) []string {
	var items []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &items); err != nil {
		return nil
	}
	return normalizeStrings(items)
}

func stripCodeFence(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

func normalizeFindings(items []Finding) []Finding {
	out := make([]Finding, 0, len(items))
	for _, item := range items {
		item.Category = normalizeCategory(item.Category)
		item.Severity = normalizeSeverity(item.Severity)
		item.FilePath = strings.TrimSpace(item.FilePath)
		item.Message = strings.TrimSpace(item.Message)
		item.Suggestion = strings.TrimSpace(item.Suggestion)
		if item.Message == "" && item.Suggestion == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeCategory(value string) string {
	switch strings.TrimSpace(value) {
	case "spec", "blueprint", "diff", "tests", "security", "quality":
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizeSeverity(value string) string {
	switch strings.TrimSpace(value) {
	case "critical", "major", "minor", "note":
		return strings.TrimSpace(value)
	default:
		return "note"
	}
}

func normalizeStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeReturnTo(value string, status string) string {
	value = strings.TrimSpace(value)
	if status == StatusAccepted {
		return ""
	}
	switch value {
	case ReturnToProduct, ReturnToArchitect, ReturnToDeveloper, ReturnToTester, ReturnToUser:
		return value
	default:
		if status == StatusBlocked {
			return ReturnToUser
		}
		return ReturnToDeveloper
	}
}
