package orchestration

import (
	"fmt"
	"strings"

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/router"
)

type Mode string

const (
	ModeDirect   Mode = "direct"
	ModeWorkflow Mode = "workflow"
)

type Decision struct {
	Intent              router.Intent `json:"intent"`
	Mode                Mode          `json:"mode"`
	NeedsWorkflow       bool          `json:"needsWorkflow"`
	NeedsProjectContext bool          `json:"needsProjectContext"`
	NeedsClarification  bool          `json:"needsClarification"`
	GroupID             string        `json:"groupId"`
	GroupName           string        `json:"groupName"`
	GroupKind           string        `json:"groupKind"`
	LifecycleID         string        `json:"lifecycleId"`
	Reason              string        `json:"reason"`
	Explanation         string        `json:"explanation"`
	SkippedSteps        []string      `json:"skippedSteps"`
	UsedMemory          bool          `json:"usedMemory"`
	Source              string        `json:"source"`
}

type Input struct {
	Message          string
	RouterDecision   router.Decision
	CurrentGroup     *agentgroups.Group
	AvailableGroups  []agentgroups.Group
	HasTaskSpec      bool
	HasProjectMemory bool
}

func Decide(input Input) Decision {
	routerDecision := input.RouterDecision
	groupKind := groupKindForIntent(routerDecision.Intent, input.Message)
	mode := ModeDirect
	if routerDecision.NeedsWorkflow {
		mode = ModeWorkflow
	}
	if shouldAnswerDirect(routerDecision.Intent) {
		mode = ModeDirect
	}

	group := selectGroup(input.CurrentGroup, input.AvailableGroups, groupKind)
	result := Decision{
		Intent:              routerDecision.Intent,
		Mode:                mode,
		NeedsWorkflow:       mode == ModeWorkflow,
		NeedsProjectContext: routerDecision.NeedsProjectContext,
		NeedsClarification:  routerDecision.NeedsClarification,
		Reason:              strings.TrimSpace(routerDecision.Reason),
		SkippedSteps:        skippedStepsForMode(mode, routerDecision.Intent),
		UsedMemory:          input.HasTaskSpec || input.HasProjectMemory,
		Source:              firstNonEmpty(routerDecision.Source, "rules"),
	}
	if group != nil {
		result.GroupID = group.ID
		result.GroupName = group.Name
		result.GroupKind = group.Kind
		result.LifecycleID = group.DefaultLifecycleID
	}
	if result.GroupKind == "" {
		result.GroupKind = groupKind
	}
	result.Explanation = explain(result, input.CurrentGroup, input.HasTaskSpec, input.HasProjectMemory)
	return result
}

func groupKindForIntent(intent router.Intent, message string) string {
	switch intent {
	case router.IntentResearchTask:
		return agentgroups.GroupKindResearch
	case router.IntentPentestTask:
		if hasCTFContext(message) {
			return agentgroups.GroupKindCTF
		}
		return agentgroups.GroupKindSecurity
	case router.IntentCodingTask, router.IntentClarificationAnswer:
		return agentgroups.GroupKindDev
	default:
		return ""
	}
}

func hasCTFContext(message string) bool {
	text := strings.ToLower(strings.Join(strings.Fields(message), " "))
	return containsAny(text,
		"ctf", "capture the flag", "challenge", "lab", "лаба", "лаборатор",
		"flag", "флаг", "writeup", "райтап", "hackthebox", "tryhackme",
		"picoctf", "ctftime", "root-me", "portswigger lab",
	)
}

func shouldAnswerDirect(intent router.Intent) bool {
	switch intent {
	case router.IntentDirectAnswer, router.IntentProjectAnalysis, router.IntentWorkflowControl, router.IntentGeneralChat:
		return true
	default:
		return false
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func selectGroup(current *agentgroups.Group, groups []agentgroups.Group, targetKind string) *agentgroups.Group {
	if strings.TrimSpace(targetKind) == "" {
		if current != nil {
			return current
		}
		return firstGroup(groups)
	}
	if current != nil && current.Kind == targetKind {
		return current
	}
	for _, group := range groups {
		if group.Kind == targetKind && group.Status != agentgroups.StatusArchived {
			copy := group
			return &copy
		}
	}
	if current != nil {
		return current
	}
	return firstGroup(groups)
}

func firstGroup(groups []agentgroups.Group) *agentgroups.Group {
	for _, group := range groups {
		if group.Status == agentgroups.StatusArchived {
			continue
		}
		copy := group
		return &copy
	}
	return nil
}

func skippedStepsForMode(mode Mode, intent router.Intent) []string {
	if mode == ModeWorkflow {
		switch intent {
		case router.IntentResearchTask:
			return []string{"dev_requirements", "dev_blueprint", "dev_checks", "dev_review"}
		case router.IntentPentestTask:
			return []string{"dev_scaffold", "unrelated_code_review"}
		default:
			return nil
		}
	}
	switch intent {
	case router.IntentProjectAnalysis:
		return []string{"requirements", "blueprint", "development", "checks", "review"}
	case router.IntentWorkflowControl:
		return []string{"new_workflow"}
	case router.IntentDirectAnswer, router.IntentGeneralChat:
		return []string{"requirements", "blueprint", "development", "checks", "review"}
	default:
		return nil
	}
}

func explain(decision Decision, current *agentgroups.Group, hasSpec bool, hasMemory bool) string {
	var parts []string
	if decision.Mode == ModeWorkflow {
		parts = append(parts, fmt.Sprintf("запускаю workflow для intent `%s`", decision.Intent))
	} else {
		parts = append(parts, fmt.Sprintf("отвечаю напрямую для intent `%s`", decision.Intent))
	}
	if decision.GroupName != "" {
		if current != nil && current.ID != "" && current.ID != decision.GroupID {
			parts = append(parts, fmt.Sprintf("выбрана группа `%s` вместо `%s`", decision.GroupName, current.Name))
		} else {
			parts = append(parts, fmt.Sprintf("используется группа `%s`", decision.GroupName))
		}
	}
	if decision.LifecycleID != "" && decision.Mode == ModeWorkflow {
		parts = append(parts, "lifecycle `"+decision.LifecycleID+"`")
	}
	if hasSpec {
		parts = append(parts, "учтена живая спека")
	}
	if hasMemory {
		parts = append(parts, "учтена память проекта")
	}
	if len(decision.SkippedSteps) > 0 {
		parts = append(parts, fmt.Sprintf("лишние шаги пропущены: %s", strings.Join(decision.SkippedSteps, ", ")))
	}
	if decision.Reason != "" {
		parts = append(parts, "причина: "+decision.Reason)
	}
	return strings.Join(parts, "; ")
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
