package openaiapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"zavod_ai/internal/llm"
)

type Client struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewClient(baseURL string, apiKeyRef string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  resolveAPIKey(apiKeyRef),
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (c *Client) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	if err := c.validate(req.Model); err != nil {
		return nil, err
	}

	payload := chatCompletionRequest{
		Model:       req.Model,
		Messages:    toAPIMessage(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatCompletionsURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model api вернул %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var decoded chatCompletionResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	if len(decoded.Choices) == 0 {
		return nil, fmt.Errorf("model api не вернул choices")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return nil, fmt.Errorf("model api вернул пустой ответ")
	}
	return &llm.Response{Content: content}, nil
}

func (c *Client) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	if err := c.validate(req.Model); err != nil {
		return nil, err
	}

	payload := chatCompletionRequest{
		Model:       req.Model,
		Messages:    toAPIMessage(req.Messages),
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.chatCompletionsURL(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	events := make(chan llm.Event, 16)
	go func() {
		defer close(events)
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
			events <- llm.Event{Error: fmt.Sprintf("model api вернул %s: %s", resp.Status, strings.TrimSpace(string(raw))), Done: true}
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		accumulated := ""
		for scanner.Scan() {
			event, ok := parseStreamLine(scanner.Text())
			if !ok {
				continue
			}
			if event.Delta != "" {
				nextAccumulated, delta, ok := normalizeStreamDelta(accumulated, event.Delta)
				accumulated = nextAccumulated
				if !ok {
					continue
				}
				event.Delta = delta
			}
			events <- event
			if event.Done || event.Error != "" {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			events <- llm.Event{Error: err.Error(), Done: true}
			return
		}
		events <- llm.Event{Done: true}
	}()
	return events, nil
}

func (c *Client) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		Streaming: true,
		Tools:     false,
	}
}

func (c *Client) Check(ctx context.Context, modelName string) llm.ModelCheckResult {
	start := time.Now()
	now := time.Now().UTC().Format(time.RFC3339)
	result := llm.ModelCheckResult{
		Status:        "offline",
		LastCheckedAt: now,
	}

	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := c.Generate(checkCtx, llm.Request{
		Model: modelName,
		Messages: []llm.Message{
			{Role: "system", Content: "Ответь только OK."},
			{Role: "user", Content: "Проверка подключения."},
		},
		Temperature: 0,
		MaxTokens:   8,
	})
	result.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		result.LastError = err.Error()
		return result
	}
	if strings.TrimSpace(resp.Content) == "" {
		result.LastError = "модель вернула пустой ответ"
		return result
	}

	result.Status = "online"
	return result
}

func (c *Client) validate(modelName string) error {
	if c.baseURL == "" {
		return fmt.Errorf("base_url модели пустой")
	}
	if modelName == "" {
		return fmt.Errorf("model_name модели пустой")
	}
	if strings.Contains(c.baseURL, "api.openai.com") && c.apiKey == "" {
		return fmt.Errorf("для OpenAI нужен API key или переменная окружения OPENAI_API_KEY")
	}
	return nil
}

func (c *Client) chatCompletionsURL() string {
	if strings.HasSuffix(c.baseURL, "/chat/completions") {
		return c.baseURL
	}
	if strings.HasSuffix(c.baseURL, "/v1") {
		return c.baseURL + "/chat/completions"
	}
	return c.baseURL + "/v1/chat/completions"
}

func resolveAPIKey(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if value := os.Getenv(ref); value != "" {
		return value
	}
	return ref
}

func toAPIMessage(messages []llm.Message) []apiMessage {
	out := make([]apiMessage, 0, len(messages))
	for _, message := range messages {
		role := message.Role
		if role == "" {
			role = "user"
		}
		out = append(out, apiMessage{
			Role:    role,
			Content: message.Content,
		})
	}
	return out
}

type chatCompletionRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	Temperature float64      `json:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Stream      bool         `json:"stream"`
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message apiMessage `json:"message"`
	} `json:"choices"`
}

type streamCompletionResponse struct {
	Choices []struct {
		Delta apiMessage `json:"delta"`
	} `json:"choices"`
}

func parseStreamLine(line string) (llm.Event, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
		return llm.Event{}, false
	}
	if !strings.HasPrefix(line, "data:") {
		return llm.Event{}, false
	}

	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "" {
		return llm.Event{}, false
	}
	if payload == "[DONE]" {
		return llm.Event{Done: true}, true
	}

	var decoded streamCompletionResponse
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return llm.Event{Error: err.Error(), Done: true}, true
	}
	if len(decoded.Choices) == 0 {
		return llm.Event{}, false
	}

	delta := decoded.Choices[0].Delta.Content
	if delta == "" {
		return llm.Event{}, false
	}
	return llm.Event{Delta: delta}, true
}

func normalizeStreamDelta(accumulated string, chunk string) (string, string, bool) {
	if chunk == "" {
		return accumulated, "", false
	}
	if accumulated != "" && len(chunk) > len(accumulated) && strings.HasPrefix(chunk, accumulated) {
		return chunk, strings.TrimPrefix(chunk, accumulated), true
	}
	return accumulated + chunk, chunk, true
}
