package app

import (
	"context"
	"fmt"
	"strings"

	"zavod_ai/internal/agentgroups"
)

type AgentGroupTemplateDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	AgentCount  int    `json:"agentCount"`
	StepCount   int    `json:"stepCount"`
}

type CreateAgentGroupFromTemplateInput struct {
	TemplateID         string `json:"templateId"`
	Name               string `json:"name"`
	DefaultModelID     string `json:"defaultModelId"`
	SelectForProjectID string `json:"selectForProjectId"`
}

type groupTemplate struct {
	ID                  string
	Name                string
	Kind                string
	Description         string
	LifecycleName       string
	LifecycleDesc       string
	MaxTotalIterations  int
	MaxRepairIterations int
	SameErrorLimit      int
	Profiles            []templateProfile
	Steps               []templateStep
}

type templateProfile struct {
	Key           string
	Name          string
	RoleKey       string
	Description   string
	ToolProfileID string
	Temperature   float64
	ContextBudget int
	Enabled       bool
}

type templateStep struct {
	StepKey          string
	Title            string
	ProfileKey       string
	Mode             string
	Required         bool
	CanRetry         bool
	MaxRetries       int
	OnSuccessStepKey string
	OnFailureStepKey string
	VisibleToUser    bool
}

func (s *Service) ListAgentGroupTemplates(ctx context.Context) ([]AgentGroupTemplateDTO, error) {
	_ = ctx
	templates := agentGroupTemplates()
	out := make([]AgentGroupTemplateDTO, 0, len(templates))
	for _, template := range templates {
		out = append(out, AgentGroupTemplateDTO{
			ID:          template.ID,
			Name:        template.Name,
			Kind:        template.Kind,
			Description: template.Description,
			AgentCount:  len(template.Profiles),
			StepCount:   len(template.Steps),
		})
	}
	return out, nil
}

func (s *Service) CreateAgentGroupFromTemplate(ctx context.Context, input CreateAgentGroupFromTemplateInput) ([]AgentGroupDTO, error) {
	template, ok := findAgentGroupTemplate(input.TemplateID)
	if !ok {
		return nil, fmt.Errorf("шаблон группы не найден")
	}
	defaultModelID := strings.TrimSpace(input.DefaultModelID)
	if defaultModelID == "" {
		model, err := s.store.ActiveModelConfig(ctx)
		if err != nil {
			return nil, err
		}
		defaultModelID = model.ID
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = s.uniqueTemplateGroupName(ctx, template.Name)
	}

	group, err := s.store.CreateAgentGroup(ctx, agentgroups.Group{
		Name:           name,
		Kind:           template.Kind,
		Description:    template.Description,
		DefaultModelID: defaultModelID,
	})
	if err != nil {
		return nil, err
	}
	lifecycle, err := s.store.SaveLifecycleDefinition(ctx, agentgroups.LifecycleDefinition{
		ID:                  group.DefaultLifecycleID,
		GroupID:             group.ID,
		Name:                template.LifecycleName,
		Kind:                template.Kind,
		Description:         template.LifecycleDesc,
		MaxTotalIterations:  template.MaxTotalIterations,
		MaxRepairIterations: template.MaxRepairIterations,
		SameErrorLimit:      template.SameErrorLimit,
		Status:              agentgroups.StatusActive,
	})
	if err != nil {
		return nil, err
	}

	profileIDs := map[string]string{}
	for index, profile := range template.Profiles {
		saved, err := s.store.SaveAgentProfile(ctx, agentgroups.Profile{
			GroupID:       group.ID,
			Name:          profile.Name,
			RoleKey:       profile.RoleKey,
			Description:   profile.Description,
			ModelID:       defaultModelID,
			ToolProfileID: profile.ToolProfileID,
			Temperature:   profile.Temperature,
			ContextBudget: profile.ContextBudget,
			Enabled:       profile.Enabled,
			SortOrder:     index,
		})
		if err != nil {
			return nil, err
		}
		if _, err := s.ensureSoulFile(ctx, saved); err != nil {
			return nil, err
		}
		profileIDs[profile.Key] = saved.ID
	}
	for index, step := range template.Steps {
		profileID := profileIDs[step.ProfileKey]
		if profileID == "" {
			return nil, fmt.Errorf("шаблон %s ссылается на неизвестного агента %s", template.ID, step.ProfileKey)
		}
		if _, err := s.store.SaveLifecycleStep(ctx, agentgroups.LifecycleStep{
			LifecycleID:      lifecycle.ID,
			StepKey:          step.StepKey,
			Title:            step.Title,
			AgentProfileID:   profileID,
			Mode:             step.Mode,
			Required:         step.Required,
			CanRetry:         step.CanRetry,
			MaxRetries:       step.MaxRetries,
			OnSuccessStepKey: step.OnSuccessStepKey,
			OnFailureStepKey: step.OnFailureStepKey,
			VisibleToUser:    step.VisibleToUser,
			SortOrder:        index,
		}); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(input.SelectForProjectID) != "" {
		if _, err := s.store.BindProjectToAgentGroup(ctx, input.SelectForProjectID, group.ID, lifecycle.ID); err != nil {
			return nil, err
		}
	}
	return s.store.ListAgentGroups(ctx, false)
}

func (s *Service) uniqueTemplateGroupName(ctx context.Context, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "Agent Group"
	}
	groups, err := s.store.ListAgentGroups(ctx, true)
	if err != nil {
		return base
	}
	used := map[string]bool{}
	for _, group := range groups {
		used[strings.ToLower(strings.TrimSpace(group.Name))] = true
	}
	candidate := base + " copy"
	if !used[strings.ToLower(candidate)] {
		return candidate
	}
	for index := 2; ; index++ {
		candidate = fmt.Sprintf("%s copy %d", base, index)
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

func findAgentGroupTemplate(id string) (groupTemplate, bool) {
	id = strings.TrimSpace(id)
	for _, template := range agentGroupTemplates() {
		if template.ID == id {
			return template, true
		}
	}
	return groupTemplate{}, false
}

func agentGroupTemplates() []groupTemplate {
	return []groupTemplate{
		devGroupTemplate(),
		ctfGroupTemplate(),
		researchGroupTemplate(),
		securityAuditGroupTemplate(),
		soloLumenTemplate(),
	}
}

func devGroupTemplate() groupTemplate {
	return groupTemplate{
		ID:                  "dev_squad",
		Name:                "Dev Squad",
		Kind:                agentgroups.GroupKindDev,
		Description:         "Команда для Python/Go разработки: требования, blueprint, код, проверки и ревью.",
		LifecycleName:       "Dev Autopilot",
		LifecycleDesc:       "Dev workflow: intake, requirements, blueprint, architecture, development, checks, review, final.",
		MaxTotalIterations:  16,
		MaxRepairIterations: 2,
		SameErrorLimit:      2,
		Profiles: []templateProfile{
			{Key: "lumen", Name: "Люмен", RoleKey: "manager", Description: "Принимает задачу, роутит intent, держит task spec и итог.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "product", Name: "Продакт", RoleKey: "product", Description: "Формулирует требования, сценарии и критерии готовности.", Temperature: 0.2, ContextBudget: 9000, Enabled: true},
			{Key: "architect", Name: "Архитектор", RoleKey: "architect", Description: "Проектирует blueprint, риски и порядок внедрения.", ToolProfileID: "tool_go_dev", Temperature: 0.15, ContextBudget: 10000, Enabled: true},
			{Key: "developer", Name: "Разработчик", RoleKey: "developer", Description: "Готовит structured changes и кодовые изменения.", ToolProfileID: "tool_go_dev", Temperature: 0.12, ContextBudget: 14000, Enabled: true},
			{Key: "tester", Name: "Тестировщик", RoleKey: "tester", Description: "Подбирает и запускает минимальные проверки по стеку.", ToolProfileID: "tool_python_dev", Temperature: 0.1, ContextBudget: 8000, Enabled: true},
			{Key: "reviewer", Name: "Ревьюер", RoleKey: "reviewer", Description: "Обязательный gate качества перед итогом.", Temperature: 0.08, ContextBudget: 10000, Enabled: true},
			{Key: "docs", Name: "Докер", RoleKey: "docs", Description: "Обновляет README и инструкции разработки.", Temperature: 0.15, ContextBudget: 8000, Enabled: false},
			{Key: "release", Name: "Релизер", RoleKey: "release", Description: "Готовит changelog, сборку и release notes.", Temperature: 0.1, ContextBudget: 8000, Enabled: false},
		},
		Steps: []templateStep{
			{StepKey: "manager_intake", Title: "Постановка задачи", ProfileKey: "lumen", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "product_requirements", Title: "Требования", ProfileKey: "product", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "task_blueprint", Title: "Blueprint", ProfileKey: "architect", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "architect_plan", Title: "Архитектурный план", ProfileKey: "architect", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "developer_plan", Title: "Разработка", ProfileKey: "developer", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 2, OnFailureStepKey: "developer_plan", VisibleToUser: true},
			{StepKey: "tester_commands", Title: "Проверка", ProfileKey: "tester", Mode: "checks", Required: true, CanRetry: true, MaxRetries: 2, OnFailureStepKey: "developer_plan", VisibleToUser: true},
			{StepKey: "review", Title: "Ревью", ProfileKey: "reviewer", Mode: "review", Required: true, CanRetry: true, MaxRetries: 2, OnFailureStepKey: "developer_plan", VisibleToUser: true},
			{StepKey: "manager_final", Title: "Итог", ProfileKey: "lumen", Mode: "final", Required: true, VisibleToUser: true},
		},
	}
}

func ctfGroupTemplate() groupTemplate {
	return groupTemplate{
		ID:                  "ctf_cell",
		Name:                "CTF Cell",
		Kind:                agentgroups.GroupKindCTF,
		Description:         "Команда для CTF и легитимных lab-задач: triage, scope, категория, решение и writeup.",
		LifecycleName:       "CTF Challenge",
		LifecycleDesc:       "CTF workflow: intake, scope, artifacts, triage, hypothesis, category solver, validation, writeup.",
		MaxTotalIterations:  18,
		MaxRepairIterations: 2,
		SameErrorLimit:      2,
		Profiles: []templateProfile{
			{Key: "lumen", Name: "Люмен", RoleKey: "manager", Description: "Принимает CTF-задачу, классифицирует категорию и собирает writeup.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "scout", Name: "Разведчик", RoleKey: "ctf_scout", Description: "Собирает вводные, артефакты, scope и первые наблюдения.", ToolProfileID: "tool_research", Temperature: 0.12, ContextBudget: 10000, Enabled: true},
			{Key: "web", Name: "Web Exploiter", RoleKey: "ctf_web", Description: "Решает общие web challenge только в рамках scope.", ToolProfileID: "tool_ctf_web", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "lfi", Name: "LFI Hunter", RoleKey: "ctf_lfi", Description: "Решает LFI/path traversal challenge только в рамках scope.", ToolProfileID: "tool_ctf_lfi", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "rce", Name: "RCE Analyst", RoleKey: "ctf_rce", Description: "Решает RCE/command injection challenge только в рамках scope.", ToolProfileID: "tool_ctf_rce", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "sqli", Name: "SQLi Solver", RoleKey: "ctf_sqli", Description: "Решает SQL injection challenge только в рамках scope.", ToolProfileID: "tool_ctf_sqli", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "pwn", Name: "Pwner", RoleKey: "ctf_pwn", Description: "Разбирает локальные pwn/binary exploitation challenge.", ToolProfileID: "tool_ctf_pwn", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "crypto", Name: "Криптограф", RoleKey: "ctf_crypto", Description: "Строит solver для crypto challenge.", ToolProfileID: "tool_ctf_crypto", Temperature: 0.08, ContextBudget: 12000, Enabled: true},
			{Key: "reverse", Name: "Реверсер", RoleKey: "ctf_reverse", Description: "Анализирует reverse engineering задачи.", ToolProfileID: "tool_ctf_reverse", Temperature: 0.08, ContextBudget: 12000, Enabled: true},
			{Key: "forensics", Name: "Форензик", RoleKey: "ctf_forensics", Description: "Разбирает файлы, дампы, изображения и сетевые артефакты.", ToolProfileID: "tool_ctf_forensics", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "validator", Name: "Валидатор", RoleKey: "ctf_validator", Description: "Проверяет flag, воспроизводимость решения и writeup.", ToolProfileID: "tool_ctf_validator", Temperature: 0.05, ContextBudget: 9000, Enabled: true},
		},
		Steps: []templateStep{
			{StepKey: "intake", Title: "Постановка CTF", ProfileKey: "lumen", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "scope_check", Title: "Scope", ProfileKey: "scout", Mode: "human_gate", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "artifact_collection", Title: "Артефакты", ProfileKey: "scout", Mode: "tool", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "triage", Title: "Категория", ProfileKey: "scout", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "hypothesis_board", Title: "Гипотезы", ProfileKey: "lumen", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "category_solver", Title: "Решение", ProfileKey: "web", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 3, VisibleToUser: true},
			{StepKey: "validation", Title: "Проверка flag", ProfileKey: "validator", Mode: "review", Required: true, CanRetry: true, MaxRetries: 2, VisibleToUser: true},
			{StepKey: "writeup", Title: "Writeup", ProfileKey: "lumen", Mode: "final", Required: true, VisibleToUser: true},
		},
	}
}

func researchGroupTemplate() groupTemplate {
	return groupTemplate{
		ID:                  "research_desk",
		Name:                "Research Squad",
		Kind:                agentgroups.GroupKindResearch,
		Description:         "Команда для поиска в интернете, проверки источников, сравнения, аналитики и research notes.",
		LifecycleName:       "Research Workflow",
		LifecycleDesc:       "Research workflow: план поиска, сбор источников, проверка свежести, синтез, notes и итог.",
		MaxTotalIterations:  12,
		MaxRepairIterations: 1,
		SameErrorLimit:      2,
		Profiles: []templateProfile{
			{Key: "lumen", Name: "Люмен", RoleKey: "manager", Description: "Понимает вопрос и собирает итоговый ответ.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "researcher", Name: "Исследователь", RoleKey: "researcher", Description: "Формирует запросы, ищет источники и фиксирует найденное.", ToolProfileID: "tool_research", Temperature: 0.12, ContextBudget: 12000, Enabled: true},
			{Key: "source_reviewer", Name: "Проверяющая источники", RoleKey: "source_reviewer", Description: "Проверяет свежесть, доверие, прямые ссылки и противоречия.", ToolProfileID: "tool_research", Temperature: 0.05, ContextBudget: 9000, Enabled: true},
			{Key: "analyst", Name: "Аналитик", RoleKey: "analyst", Description: "Сравнивает источники, отделяет факты от выводов и собирает аналитику.", ToolProfileID: "tool_research", Temperature: 0.12, ContextBudget: 11000, Enabled: true},
		},
		Steps: []templateStep{
			{StepKey: "web_research", Title: "Поиск", ProfileKey: "researcher", Mode: "tool", Required: true, CanRetry: true, MaxRetries: 2, VisibleToUser: true},
			{StepKey: "source_review", Title: "Источники", ProfileKey: "source_reviewer", Mode: "review", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "research_synthesis", Title: "Аналитика", ProfileKey: "analyst", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "research_notes", Title: "Research notes", ProfileKey: "researcher", Mode: "artifact", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "manager_final", Title: "Итог", ProfileKey: "lumen", Mode: "final", Required: true, VisibleToUser: true},
		},
	}
}

func securityAuditGroupTemplate() groupTemplate {
	return groupTemplate{
		ID:                  "security_audit",
		Name:                "Security Audit",
		Kind:                agentgroups.GroupKindSecurity,
		Description:         "Команда для defensive-аудита, threat model, hardening и remediation.",
		LifecycleName:       "Security Audit Workflow",
		LifecycleDesc:       "Security workflow: scope, анализ, remediation plan, review, итог.",
		MaxTotalIterations:  12,
		MaxRepairIterations: 1,
		SameErrorLimit:      2,
		Profiles: []templateProfile{
			{Key: "lumen", Name: "Люмен", RoleKey: "manager", Description: "Координирует аудит и итог.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "security", Name: "ИБ-специалист", RoleKey: "security", Description: "Разбирает scope, риски и безопасные проверки.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
			{Key: "threat_modeler", Name: "Threat Modeler", RoleKey: "threat_modeler", Description: "Строит модель угроз и trust boundaries.", ToolProfileID: "tool_research", Temperature: 0.12, ContextBudget: 10000, Enabled: true},
			{Key: "remediator", Name: "Remediator", RoleKey: "remediator", Description: "Формирует план исправлений и hardening.", Temperature: 0.1, ContextBudget: 9000, Enabled: true},
			{Key: "reviewer", Name: "Security Reviewer", RoleKey: "reviewer", Description: "Проверяет полноту аудита и отсутствие unsafe-рекомендаций.", Temperature: 0.06, ContextBudget: 9000, Enabled: true},
		},
		Steps: []templateStep{
			{StepKey: "security_scope", Title: "Scope", ProfileKey: "security", Mode: "human_gate", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "security_analysis", Title: "ИБ-анализ", ProfileKey: "security", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "threat_model", Title: "Threat model", ProfileKey: "threat_modeler", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "remediation_plan", Title: "Исправления", ProfileKey: "remediator", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "review", Title: "Ревью", ProfileKey: "reviewer", Mode: "review", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "manager_final", Title: "Итог", ProfileKey: "lumen", Mode: "final", Required: true, VisibleToUser: true},
		},
	}
}

func soloLumenTemplate() groupTemplate {
	return groupTemplate{
		ID:                  "solo_lumen",
		Name:                "Solo Lumen",
		Kind:                agentgroups.GroupKindCustom,
		Description:         "Минимальная группа: Люмен отвечает напрямую и запускает только короткий одиночный workflow.",
		LifecycleName:       "Solo Workflow",
		LifecycleDesc:       "Solo workflow: понять задачу и дать компактный ответ.",
		MaxTotalIterations:  4,
		MaxRepairIterations: 0,
		SameErrorLimit:      2,
		Profiles: []templateProfile{
			{Key: "lumen", Name: "Люмен", RoleKey: "manager", Description: "Понимает запрос, отвечает и при необходимости формирует короткий план.", ToolProfileID: "tool_research", Temperature: 0.1, ContextBudget: 12000, Enabled: true},
		},
		Steps: []templateStep{
			{StepKey: "manager_intake", Title: "Понять задачу", ProfileKey: "lumen", Mode: "llm", Required: true, CanRetry: true, MaxRetries: 1, VisibleToUser: true},
			{StepKey: "manager_final", Title: "Ответ", ProfileKey: "lumen", Mode: "final", Required: true, VisibleToUser: true},
		},
	}
}
