package agents

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"zavod_ai/internal/chat"
	"zavod_ai/internal/llm"
)

const maxHistoryContentBytes = 3200

type Manager struct {
	provider llm.Provider
	model    string
}

func NewManager(provider llm.Provider, model string) *Manager {
	return &Manager{
		provider: provider,
		model:    model,
	}
}

func (m *Manager) Respond(ctx context.Context, history []chat.Message) (string, error) {
	resp, err := m.provider.Generate(ctx, llm.Request{
		Model:       m.model,
		Messages:    managerMessages(history),
		Temperature: 0.2,
		MaxTokens:   900,
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (m *Manager) Stream(ctx context.Context, history []chat.Message) (<-chan llm.Event, error) {
	return m.provider.Stream(ctx, llm.Request{
		Model:       m.model,
		Messages:    managerMessages(history),
		Temperature: 0.2,
		MaxTokens:   900,
	})
}

func managerMessages(history []chat.Message) []llm.Message {
	messages := []llm.Message{
		{
			Role: "system",
			Content: strings.TrimSpace(`
Ты Менеджер локального AI-завода.
Отвечай на русском языке.
Твоя задача в версии V0.1:
- принять задачу пользователя;
- кратко показать, что ты понял;
- если информации мало, задать 1-3 уточняющих вопроса;
- если задача понятна, предложить аккуратный первый план действий;
- держать роль менеджера: формулировать задачу, критерии готовности, риски и следующий шаг;
- если задача выглядит как разработка, предложить передать ее будущему агенту "Разработчик", но не выполнять разработку самому;
- не писать код, скрипты, команды терминала, патчи или SQL, если пользователь прямо не попросил именно текстовый пример;
- если пользователь просит что-то проверить, объяснить что нужно проверить и какие данные нужны, но не имитировать запуск проверки;
- не утверждать, что ты читал файлы проекта или менял код;
- не обещать действия, для которых у тебя в V0.1 еще нет инструментов.
Формат ответа:
1. "Понял задачу:" - 1-2 предложения.
2. "Что нужно уточнить:" - только если есть вопросы.
3. "Следующий шаг:" - конкретное действие или короткий план.
Пиши по делу, спокойно и структурно.
`),
		},
	}

	start := 0
	if len(history) > 20 {
		start = len(history) - 20
	}
	for _, item := range history[start:] {
		role := "user"
		content := sanitizeHistoryContent(item.Content)
		if item.Role == "agent" {
			role = "assistant"
		}
		messages = append(messages, llm.Message{
			Role:    role,
			Content: content,
		})
	}
	return messages
}

func sanitizeHistoryContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if strings.Count(content, "Агент manager:") >= 4 || strings.Count(content, "Agent manager:") >= 4 {
		return "Предыдущий ответ агента был остановлен из-за повторов."
	}
	if len(content) <= maxHistoryContentBytes {
		return content
	}

	truncated := content[:maxHistoryContentBytes]
	for !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return fmt.Sprintf("%s\n\n[История обрезана до %d байт.]", strings.TrimSpace(truncated), maxHistoryContentBytes)
}
