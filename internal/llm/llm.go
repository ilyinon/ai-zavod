package llm

import "context"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type Request struct {
	NoRetry     bool      `json:"-"`
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"maxTokens"`
	Tools       []Tool    `json:"tools,omitempty"`
}

type Response struct {
	Content      string     `json:"content"`
	InputTokens  int        `json:"inputTokens"`
	OutputTokens int        `json:"outputTokens"`
	TotalTokens  int        `json:"totalTokens"`
	ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
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
