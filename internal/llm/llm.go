package llm

import "context"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"maxTokens"`
}

type Response struct {
	Content      string `json:"content"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	TotalTokens  int    `json:"totalTokens"`
}

type Event struct {
	Delta string `json:"delta"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
}

type Capabilities struct {
	Streaming bool `json:"streaming"`
	Tools     bool `json:"tools"`
}

type Provider interface {
	Generate(ctx context.Context, req Request) (*Response, error)
	Stream(ctx context.Context, req Request) (<-chan Event, error)
	Capabilities() Capabilities
}

type ModelConfig struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	BaseURL       string `json:"baseUrl"`
	APIKeyRef     string `json:"apiKeyRef"`
	ModelName     string `json:"modelName"`
	IsActive      bool   `json:"isActive"`
	Status        string `json:"status"`
	LastCheckedAt string `json:"lastCheckedAt"`
	LastError     string `json:"lastError"`
	LatencyMS     int64  `json:"latencyMs"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type ModelCheckResult struct {
	ModelID       string `json:"modelId"`
	Status        string `json:"status"`
	LastCheckedAt string `json:"lastCheckedAt"`
	LastError     string `json:"lastError"`
	LatencyMS     int64  `json:"latencyMs"`
}
