package chat

type Task struct {
	GroupID        string `json:"groupId"`
	ModelID        string `json:"modelId"`
	ID             string `json:"id"`
	ProjectID      string `json:"projectId"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	Pinned         bool   `json:"pinned"`
	PendingRequest string `json:"pendingRequest,omitempty"`
}

type Message struct {
	ID        string `json:"id"`
	TaskID    string `json:"taskId"`
	Role      string `json:"role"`
	AgentID   string `json:"agentId"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}
