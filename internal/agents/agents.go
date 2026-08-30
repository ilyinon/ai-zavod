package agents

import "strings"

const (
	ManagerID   = "manager"
	ProductID   = "product"
	ArchitectID = "architect"
	DeveloperID = "developer"
	TesterID    = "tester"
	ReviewerID  = "reviewer"
	SecurityID  = "security"

	ManagerRole   = "manager"
	ProductRole   = "product"
	ArchitectRole = "architect"
	DeveloperRole = "developer"
	TesterRole    = "tester"
	ReviewerRole  = "reviewer"
	SecurityRole  = "security"

	ManagerName   = "Люмен"
	ProductName   = "Продакт"
	ArchitectName = "Архитектор"
	DeveloperName = "Разработчик"
	TesterName    = "Тестировщик"
	ReviewerName  = "Ревьюер"
	SecurityName  = "ИБ-специалист"
)

type Status struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Activity  string `json:"activity"`
	ModelID   string `json:"modelId"`
	UpdatedAt string `json:"updatedAt"`
}

type Spec struct {
	ID           string
	Role         string
	Name         string
	SystemPrompt string
	MaxTokens    int
	Temperature  float64
}

func DefaultSkillPrompt() string {
	return "Навык по умолчанию: используй $pony-tail как базовый стиль работы, если пользователь явно не выбрал другой skill. Применяй его как мягкий рабочий режим: сохраняй ясность, аккуратность, spec-driven подход и компактные практичные ответы. Если runtime модели не поддерживает skills напрямую, воспринимай это как инструкцию поведения, а не как команду внешнему инструменту."
}

func WithDefaultSkills(systemPrompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	if systemPrompt == "" {
		return DefaultSkillPrompt()
	}
	return strings.TrimSpace(DefaultSkillPrompt() + "\n\n" + systemPrompt)
}

func IdleStatuses(modelID string, updatedAt string) []Status {
	return []Status{
		IdleStatus(ManagerID, modelID, updatedAt),
		IdleStatus(ProductID, modelID, updatedAt),
		IdleStatus(ArchitectID, modelID, updatedAt),
		IdleStatus(DeveloperID, modelID, updatedAt),
		IdleStatus(TesterID, modelID, updatedAt),
		IdleStatus(ReviewerID, modelID, updatedAt),
		IdleStatus(SecurityID, modelID, updatedAt),
	}
}

func IdleStatus(agentID string, modelID string, updatedAt string) Status {
	role, name := Describe(agentID)
	return Status{
		ID:        agentID,
		Role:      role,
		Name:      name,
		Status:    "idle",
		Activity:  "Ждет задачу",
		ModelID:   modelID,
		UpdatedAt: updatedAt,
	}
}

func Describe(agentID string) (string, string) {
	switch agentID {
	case ProductID:
		return ProductRole, ProductName
	case ArchitectID:
		return ArchitectRole, ArchitectName
	case DeveloperID:
		return DeveloperRole, DeveloperName
	case TesterID:
		return TesterRole, TesterName
	case ReviewerID:
		return ReviewerRole, ReviewerName
	case SecurityID:
		return SecurityRole, SecurityName
	default:
		return ManagerRole, ManagerName
	}
}
