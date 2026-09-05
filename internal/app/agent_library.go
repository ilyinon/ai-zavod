package app

import (
	"context"
	"fmt"
	"os"
	"strings"

	"zavod_ai/internal/agentgroups"
)

type AgentLibraryItemDTO struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	RoleKey       string   `json:"roleKey"`
	Category      string   `json:"category"`
	Description   string   `json:"description"`
	ToolProfileID string   `json:"toolProfileId"`
	Capabilities  []string `json:"capabilities"`
	AllowedTools  []string `json:"allowedTools"`
	ReadPaths     []string `json:"readPaths"`
	WritePaths    []string `json:"writePaths"`
	HandoffRules  []string `json:"handoffRules"`
	Tags          []string `json:"tags"`
}

type AddAgentFromLibraryInput struct {
	GroupID        string `json:"groupId"`
	LibraryAgentID string `json:"libraryAgentId"`
	ModelID        string `json:"modelId"`
}

type DuplicateAgentProfileInput struct {
	ProfileID string `json:"profileId"`
}

type ReplaceAgentSoulFromLibraryInput struct {
	ProfileID       string `json:"profileId"`
	LibraryAgentID  string `json:"libraryAgentId"`
	ReplaceContract bool   `json:"replaceContract"`
}

type libraryAgent struct {
	ID            string
	Name          string
	RoleKey       string
	Category      string
	Description   string
	ToolProfileID string
	Capabilities  []string
	AllowedTools  []string
	ReadPaths     []string
	WritePaths    []string
	HandoffRules  []string
	Tags          []string
	Soul          string
}

func (s *Service) ListAgentLibrary(ctx context.Context) ([]AgentLibraryItemDTO, error) {
	_ = ctx
	agents := agentLibrary()
	out := make([]AgentLibraryItemDTO, 0, len(agents))
	for _, item := range agents {
		profile := agentgroups.NormalizeCapabilities(agentgroups.Profile{
			Name:          item.Name,
			RoleKey:       item.RoleKey,
			Description:   item.Description,
			ToolProfileID: item.ToolProfileID,
			Capabilities:  item.Capabilities,
			AllowedTools:  item.AllowedTools,
			ReadPaths:     item.ReadPaths,
			WritePaths:    item.WritePaths,
			HandoffRules:  item.HandoffRules,
		})
		out = append(out, AgentLibraryItemDTO{
			ID:            item.ID,
			Name:          item.Name,
			RoleKey:       item.RoleKey,
			Category:      item.Category,
			Description:   item.Description,
			ToolProfileID: item.ToolProfileID,
			Capabilities:  profile.Capabilities,
			AllowedTools:  profile.AllowedTools,
			ReadPaths:     profile.ReadPaths,
			WritePaths:    profile.WritePaths,
			HandoffRules:  profile.HandoffRules,
			Tags:          item.Tags,
		})
	}
	return out, nil
}

func (s *Service) AddAgentFromLibrary(ctx context.Context, input AddAgentFromLibraryInput) ([]AgentProfileDTO, error) {
	groupID := strings.TrimSpace(input.GroupID)
	if groupID == "" {
		return nil, fmt.Errorf("group_id пустой")
	}
	item, ok := findLibraryAgent(input.LibraryAgentID)
	if !ok {
		return nil, fmt.Errorf("агент библиотеки не найден")
	}
	group, err := s.store.GetAgentGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	modelID := strings.TrimSpace(input.ModelID)
	if modelID == "" {
		modelID = group.DefaultModelID
	}
	if modelID == "" {
		model, err := s.store.ActiveModelConfig(ctx)
		if err != nil {
			return nil, err
		}
		modelID = model.ID
	}
	profiles, err := s.store.ListAgentProfiles(ctx, groupID)
	if err != nil {
		return nil, err
	}
	profile := agentgroups.NormalizeCapabilities(agentgroups.Profile{
		GroupID:       groupID,
		Name:          s.uniqueAgentProfileName(ctx, groupID, item.Name),
		RoleKey:       item.RoleKey,
		Description:   item.Description,
		ModelID:       modelID,
		ToolProfileID: item.ToolProfileID,
		Capabilities:  item.Capabilities,
		AllowedTools:  item.AllowedTools,
		ReadPaths:     item.ReadPaths,
		WritePaths:    item.WritePaths,
		HandoffRules:  item.HandoffRules,
		Temperature:   0.1,
		ContextBudget: 10000,
		Enabled:       true,
		SortOrder:     len(profiles),
	})
	saved, err := s.store.SaveAgentProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	if err := s.writeLibrarySoul(ctx, saved, item); err != nil {
		return nil, err
	}
	return s.store.ListAgentProfiles(ctx, groupID)
}

func (s *Service) DuplicateAgentProfile(ctx context.Context, input DuplicateAgentProfileInput) ([]AgentProfileDTO, error) {
	source, err := s.store.GetAgentProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, err
	}
	profiles, err := s.store.ListAgentProfiles(ctx, source.GroupID)
	if err != nil {
		return nil, err
	}
	clone := source
	clone.ID = ""
	clone.Name = s.uniqueAgentProfileName(ctx, source.GroupID, source.Name+" copy")
	clone.SoulPath = ""
	clone.Enabled = true
	clone.SortOrder = len(profiles)
	saved, err := s.store.SaveAgentProfile(ctx, clone)
	if err != nil {
		return nil, err
	}
	newSoulPath, err := s.ensureSoulFile(ctx, saved)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(source.SoulPath) != "" && isPathInside(source.SoulPath, s.paths.AgentsDir) {
		if content, readErr := os.ReadFile(source.SoulPath); readErr == nil {
			if writeErr := os.WriteFile(newSoulPath, content, 0o644); writeErr != nil {
				return nil, writeErr
			}
		}
	}
	return s.store.ListAgentProfiles(ctx, source.GroupID)
}

func (s *Service) ReplaceAgentSoulFromLibrary(ctx context.Context, input ReplaceAgentSoulFromLibraryInput) (AgentSoulDTO, error) {
	profile, err := s.store.GetAgentProfile(ctx, input.ProfileID)
	if err != nil {
		return AgentSoulDTO{}, err
	}
	item, ok := findLibraryAgent(input.LibraryAgentID)
	if !ok {
		return AgentSoulDTO{}, fmt.Errorf("агент библиотеки не найден")
	}
	if input.ReplaceContract {
		profile.Capabilities = item.Capabilities
		profile.AllowedTools = item.AllowedTools
		profile.ReadPaths = item.ReadPaths
		profile.WritePaths = item.WritePaths
		profile.HandoffRules = item.HandoffRules
		profile.ToolProfileID = item.ToolProfileID
		saved, err := s.store.SaveAgentProfile(ctx, profile)
		if err != nil {
			return AgentSoulDTO{}, err
		}
		profile = saved
	}
	if err := s.writeLibrarySoul(ctx, profile, item); err != nil {
		return AgentSoulDTO{}, err
	}
	return s.GetAgentSoul(ctx, profile.ID)
}

func (s *Service) uniqueAgentProfileName(ctx context.Context, groupID string, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "Agent"
	}
	profiles, err := s.store.ListAgentProfiles(ctx, groupID)
	if err != nil {
		return base
	}
	used := map[string]bool{}
	for _, profile := range profiles {
		used[strings.ToLower(strings.TrimSpace(profile.Name))] = true
	}
	if !used[strings.ToLower(base)] {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s %d", base, index)
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}

func (s *Service) writeLibrarySoul(ctx context.Context, profile agentgroups.Profile, item libraryAgent) error {
	path, err := s.ensureSoulFile(ctx, profile)
	if err != nil {
		return err
	}
	content := strings.TrimSpace(item.Soul)
	if content == "" {
		content = defaultLibrarySoul(item)
	}
	return os.WriteFile(path, []byte(content+"\n"), 0o644)
}

func findLibraryAgent(id string) (libraryAgent, bool) {
	id = strings.TrimSpace(id)
	for _, item := range agentLibrary() {
		if item.ID == id {
			return item, true
		}
	}
	return libraryAgent{}, false
}

func defaultLibrarySoul(item libraryAgent) string {
	return strings.TrimSpace(fmt.Sprintf(`# Soul

## Кто я

Я — %s, готовый агент из локальной библиотеки Zavod AI.

## Роль

%s

## За что отвечаю

%s

## Стиль работы

- Действую строго в рамках выданной роли и capability-контракта.
- Передаю дальше только значимый контекст.
- Не вывожу служебные JSON, trace и внутренние файлы без явной просьбы.
`, item.Name, item.RoleKey, item.Description))
}

func agentLibrary() []libraryAgent {
	return []libraryAgent{
		{ID: "lib_lumen_manager", Name: "Люмен", RoleKey: "manager", Category: "Core", Description: "Координирует intent, direct answers, task spec и итог.", ToolProfileID: "tool_research", Tags: []string{"core", "router", "spec"}},
		{ID: "lib_product", Name: "Продакт", RoleKey: "product", Category: "Dev", Description: "Формулирует требования, сценарии и критерии готовности.", Tags: []string{"requirements", "acceptance"}},
		{ID: "lib_architect", Name: "Архитектор", RoleKey: "architect", Category: "Dev", Description: "Готовит blueprint, архитектурный план, риски и порядок внедрения.", ToolProfileID: "tool_go_dev", Tags: []string{"blueprint", "design"}},
		{ID: "lib_go_developer", Name: "Go-разработчик", RoleKey: "developer", Category: "Dev", Description: "Пишет Go 1.25+ код, правит модули, прогоняет gofmt и тесты.", ToolProfileID: "tool_go_dev", Capabilities: []string{"Go 1.25+ implementation", "gofmt", "go test repair loop", "structured changes"}, Tags: []string{"go", "backend"}},
		{ID: "lib_python_developer", Name: "Python-разработчик", RoleKey: "developer", Category: "Dev", Description: "Пишет Python-код с обязательными .venv и requirements.txt.", ToolProfileID: "tool_python_dev", Capabilities: []string{"Python implementation", "project-local virtualenv", "requirements.txt maintenance", "pytest/py_compile repair loop"}, Tags: []string{"python", "venv"}},
		{ID: "lib_tester", Name: "Тестировщик", RoleKey: "tester", Category: "Dev", Description: "Подбирает проверки только для затронутого стека и анализирует failures.", ToolProfileID: "tool_python_dev", Tags: []string{"qa", "checks"}},
		{ID: "lib_reviewer", Name: "Ревьюер", RoleKey: "reviewer", Category: "Dev", Description: "Обязательный gate качества: требования, diff, проверки, возврат на нужную роль.", Tags: []string{"review", "quality-gate"}},
		{ID: "lib_docs", Name: "Докер", RoleKey: "docs", Category: "Dev", Description: "Обновляет README, install/dev инструкции, сборку и usage.", Tags: []string{"docs", "readme"}},
		{ID: "lib_researcher", Name: "Исследователь", RoleKey: "researcher", Category: "Research", Description: "Ищет источники, проверяет свежесть и сохраняет краткие source notes.", ToolProfileID: "tool_research", Tags: []string{"web", "sources"}},
		{ID: "lib_source_reviewer", Name: "Проверяющая источники", RoleKey: "source_reviewer", Category: "Research", Description: "Проверяет свежесть, trust level, прямые ссылки и противоречия между источниками.", ToolProfileID: "tool_research", Tags: []string{"sources", "freshness", "citations"}},
		{ID: "lib_analyst", Name: "Аналитик", RoleKey: "analyst", Category: "Research", Description: "Сравнивает источники, отделяет факты от предположений и пишет вывод.", ToolProfileID: "tool_research", Tags: []string{"analysis", "synthesis"}},
		{ID: "lib_security", Name: "ИБ-специалист", RoleKey: "security", Category: "Security", Description: "Проверяет scope, риски, безопасные границы и defensive-рекомендации.", ToolProfileID: "tool_research", Tags: []string{"security", "scope"}},
		{ID: "lib_threat_modeler", Name: "Threat Modeler", RoleKey: "threat_modeler", Category: "Security", Description: "Строит модель угроз, trust boundaries и карту mitigation.", ToolProfileID: "tool_research", Tags: []string{"threat-model"}},
		{ID: "lib_ctf_scout", Name: "CTF Разведчик", RoleKey: "ctf_scout", Category: "CTF", Description: "Собирает артефакты, scope, категорию и первые гипотезы.", ToolProfileID: "tool_research", Tags: []string{"ctf", "triage"}},
		{ID: "lib_ctf_web", Name: "CTF Web", RoleKey: "ctf_web", Category: "CTF", Description: "Решает web challenge в рамках явно заданного CTF/lab scope.", ToolProfileID: "tool_ctf_web", Tags: []string{"web", "ctf"}},
		{ID: "lib_ctf_lfi", Name: "LFI Hunter", RoleKey: "ctf_lfi", Category: "CTF", Description: "Решает LFI/path traversal challenge только в scope.", ToolProfileID: "tool_ctf_lfi", Tags: []string{"lfi", "path-traversal"}},
		{ID: "lib_ctf_rce", Name: "RCE Analyst", RoleKey: "ctf_rce", Category: "CTF", Description: "Разбирает RCE/command injection challenge только в scope.", ToolProfileID: "tool_ctf_rce", Tags: []string{"rce", "command-injection"}},
		{ID: "lib_ctf_sqli", Name: "SQLi Solver", RoleKey: "ctf_sqli", Category: "CTF", Description: "Решает SQL injection challenge и фиксирует воспроизводимые payload notes.", ToolProfileID: "tool_ctf_sqli", Tags: []string{"sqli", "database"}},
		{ID: "lib_ctf_pwn", Name: "Pwner", RoleKey: "ctf_pwn", Category: "CTF", Description: "Разбирает локальные pwn/binary exploitation challenge: checksec/readelf/objdump и pwntools через .venv.", ToolProfileID: "tool_ctf_pwn", Tags: []string{"pwn", "binary", "pwntools"}},
		{ID: "lib_ctf_crypto", Name: "Криптограф", RoleKey: "ctf_crypto", Category: "CTF", Description: "Строит solver для crypto challenge и проверяет восстановление flag.", ToolProfileID: "tool_ctf_crypto", Tags: []string{"crypto", "solver"}},
		{ID: "lib_ctf_reverse", Name: "Реверсер", RoleKey: "ctf_reverse", Category: "CTF", Description: "Анализирует reverse engineering задачи и локальные бинарные артефакты.", ToolProfileID: "tool_ctf_reverse", Tags: []string{"reverse", "binary"}},
		{ID: "lib_ctf_forensics", Name: "Форензик", RoleKey: "ctf_forensics", Category: "CTF", Description: "Разбирает файлы, дампы, изображения и сетевые артефакты через file/strings/exiftool/binwalk.", ToolProfileID: "tool_ctf_forensics", Tags: []string{"forensics", "evidence", "binwalk", "exiftool"}},
		{ID: "lib_ctf_validator", Name: "CTF Валидатор", RoleKey: "ctf_validator", Category: "CTF", Description: "Проверяет flag, воспроизводимость решения и writeup.", ToolProfileID: "tool_ctf_validator", Tags: []string{"flag", "writeup"}},
	}
}
