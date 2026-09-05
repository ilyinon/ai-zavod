package agents

import "strings"

const (
	ManagerID      = "manager"
	ProductID      = "product"
	ArchitectID    = "architect"
	DeveloperID    = "developer"
	TesterID       = "tester"
	ReviewerID     = "reviewer"
	SecurityID     = "security"
	ResearcherID   = "researcher"
	AnalystID      = "analyst"
	SourceReviewID = "source_reviewer"
	CTFScoutID     = "ctf_scout"
	CTFWebID       = "ctf_web"
	CTFLFIID       = "ctf_lfi"
	CTFRCEID       = "ctf_rce"
	CTFSQLiID      = "ctf_sqli"
	CTFPwnID       = "ctf_pwn"
	CTFCryptoID    = "ctf_crypto"
	CTFReverseID   = "ctf_reverse"
	CTFForensicsID = "ctf_forensics"
	CTFValidatorID = "ctf_validator"

	ManagerRole      = "manager"
	ProductRole      = "product"
	ArchitectRole    = "architect"
	DeveloperRole    = "developer"
	TesterRole       = "tester"
	ReviewerRole     = "reviewer"
	SecurityRole     = "security"
	ResearcherRole   = "researcher"
	AnalystRole      = "analyst"
	SourceReviewRole = "source_reviewer"
	CTFScoutRole     = "ctf_scout"
	CTFWebRole       = "ctf_web"
	CTFLFIRole       = "ctf_lfi"
	CTFRCERole       = "ctf_rce"
	CTFSQLiRole      = "ctf_sqli"
	CTFPwnRole       = "ctf_pwn"
	CTFCryptoRole    = "ctf_crypto"
	CTFReverseRole   = "ctf_reverse"
	CTFForensicsRole = "ctf_forensics"
	CTFValidatorRole = "ctf_validator"

	ManagerName      = "Люмен"
	ProductName      = "Продакт"
	ArchitectName    = "Архитектор"
	DeveloperName    = "Разработчик"
	TesterName       = "Тестировщик"
	ReviewerName     = "Ревьюер"
	SecurityName     = "ИБ-специалист"
	ResearcherName   = "Исследователь"
	AnalystName      = "Аналитик"
	SourceReviewName = "Проверяющая источники"
	CTFScoutName     = "Разведчик"
	CTFWebName       = "Web Exploiter"
	CTFLFIName       = "LFI Hunter"
	CTFRCEName       = "RCE Analyst"
	CTFSQLiName      = "SQLi Solver"
	CTFPwnName       = "Pwner"
	CTFCryptoName    = "Криптограф"
	CTFReverseName   = "Реверсер"
	CTFForensicsName = "Форензик"
	CTFValidatorName = "Валидатор"
)

type Status struct {
	ID           string `json:"id"`
	Role         string `json:"role"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Activity     string `json:"activity"`
	ModelID      string `json:"modelId"`
	ToolID       string `json:"toolId"`
	SoulPath     string `json:"soulPath"`
	StepKey      string `json:"stepKey"`
	StartedAt    string `json:"startedAt"`
	ElapsedMS    int64  `json:"elapsedMs"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	TotalTokens  int    `json:"totalTokens"`
	UpdatedAt    string `json:"updatedAt"`
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
		IdleStatus(ResearcherID, modelID, updatedAt),
		IdleStatus(SourceReviewID, modelID, updatedAt),
		IdleStatus(AnalystID, modelID, updatedAt),
		IdleStatus(CTFScoutID, modelID, updatedAt),
		IdleStatus(CTFWebID, modelID, updatedAt),
		IdleStatus(CTFLFIID, modelID, updatedAt),
		IdleStatus(CTFRCEID, modelID, updatedAt),
		IdleStatus(CTFSQLiID, modelID, updatedAt),
		IdleStatus(CTFPwnID, modelID, updatedAt),
		IdleStatus(CTFCryptoID, modelID, updatedAt),
		IdleStatus(CTFReverseID, modelID, updatedAt),
		IdleStatus(CTFForensicsID, modelID, updatedAt),
		IdleStatus(CTFValidatorID, modelID, updatedAt),
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
	case ResearcherID:
		return ResearcherRole, ResearcherName
	case AnalystID:
		return AnalystRole, AnalystName
	case SourceReviewID:
		return SourceReviewRole, SourceReviewName
	case CTFScoutID:
		return CTFScoutRole, CTFScoutName
	case CTFWebID:
		return CTFWebRole, CTFWebName
	case CTFLFIID:
		return CTFLFIRole, CTFLFIName
	case CTFRCEID:
		return CTFRCERole, CTFRCEName
	case CTFSQLiID:
		return CTFSQLiRole, CTFSQLiName
	case CTFPwnID:
		return CTFPwnRole, CTFPwnName
	case CTFCryptoID:
		return CTFCryptoRole, CTFCryptoName
	case CTFReverseID:
		return CTFReverseRole, CTFReverseName
	case CTFForensicsID:
		return CTFForensicsRole, CTFForensicsName
	case CTFValidatorID:
		return CTFValidatorRole, CTFValidatorName
	default:
		return ManagerRole, ManagerName
	}
}
