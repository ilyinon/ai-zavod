package agents

const (
	ManagerID   = "manager"
	ProductID   = "product"
	ArchitectID = "architect"

	ManagerRole   = "manager"
	ProductRole   = "product"
	ArchitectRole = "architect"

	ManagerName   = "Менеджер"
	ProductName   = "Продакт"
	ArchitectName = "Архитектор"
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

func IdleStatuses(modelID string, updatedAt string) []Status {
	return []Status{
		IdleStatus(ManagerID, modelID, updatedAt),
		IdleStatus(ProductID, modelID, updatedAt),
		IdleStatus(ArchitectID, modelID, updatedAt),
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
	default:
		return ManagerRole, ManagerName
	}
}
