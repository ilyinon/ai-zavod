package chat

type Task struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Message struct {
	ID        string `json:"id"`
	TaskID    string `json:"taskId"`
	Role      string `json:"role"`
	AgentID   string `json:"agentId"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
}
