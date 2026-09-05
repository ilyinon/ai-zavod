package agentgroups

const (
	GroupKindDev      = "dev"
	GroupKindCTF      = "ctf"
	GroupKindResearch = "research"
	GroupKindSecurity = "security"
	GroupKindCustom   = "custom"

	StatusActive   = "active"
	StatusArchived = "archived"
)

type Group struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Kind               string `json:"kind"`
	Description        string `json:"description"`
	DefaultModelID     string `json:"defaultModelId"`
	DefaultLifecycleID string `json:"defaultLifecycleId"`
	Status             string `json:"status"`
	CreatedAt          string `json:"createdAt"`
	UpdatedAt          string `json:"updatedAt"`
	AgentCount         int    `json:"agentCount"`
}

type Profile struct {
	ID            string   `json:"id"`
	GroupID       string   `json:"groupId"`
	Name          string   `json:"name"`
	RoleKey       string   `json:"roleKey"`
	Description   string   `json:"description"`
	AvatarPath    string   `json:"avatarPath"`
	SoulPath      string   `json:"soulPath"`
	ModelID       string   `json:"modelId"`
	ToolProfileID string   `json:"toolProfileId"`
	Capabilities  []string `json:"capabilities"`
	AllowedTools  []string `json:"allowedTools"`
	ReadPaths     []string `json:"readPaths"`
	WritePaths    []string `json:"writePaths"`
	HandoffRules  []string `json:"handoffRules"`
	Temperature   float64  `json:"temperature"`
	ContextBudget int      `json:"contextBudget"`
	Enabled       bool     `json:"enabled"`
	SortOrder     int      `json:"sortOrder"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

type ToolProfile struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Description     string `json:"description"`
	AllowedCommands string `json:"allowedCommands"`
	BlockedCommands string `json:"blockedCommands"`
	RequiresScope   bool   `json:"requiresScope"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type LifecycleDefinition struct {
	ID                  string `json:"id"`
	GroupID             string `json:"groupId"`
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	Description         string `json:"description"`
	MaxTotalIterations  int    `json:"maxTotalIterations"`
	MaxRepairIterations int    `json:"maxRepairIterations"`
	SameErrorLimit      int    `json:"sameErrorLimit"`
	Status              string `json:"status"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

type LifecycleStep struct {
	ID               string `json:"id"`
	LifecycleID      string `json:"lifecycleId"`
	StepKey          string `json:"stepKey"`
	Title            string `json:"title"`
	AgentProfileID   string `json:"agentProfileId"`
	Mode             string `json:"mode"`
	Required         bool   `json:"required"`
	CanRetry         bool   `json:"canRetry"`
	MaxRetries       int    `json:"maxRetries"`
	OnSuccessStepKey string `json:"onSuccessStepKey"`
	OnFailureStepKey string `json:"onFailureStepKey"`
	OutputSchema     string `json:"outputSchema"`
	VisibleToUser    bool   `json:"visibleToUser"`
	SortOrder        int    `json:"sortOrder"`
}

type ProjectBinding struct {
	ID          string `json:"id"`
	ProjectID   string `json:"projectId"`
	GroupID     string `json:"groupId"`
	LifecycleID string `json:"lifecycleId"`
	IsDefault   bool   `json:"isDefault"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}
