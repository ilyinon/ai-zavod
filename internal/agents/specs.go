package agents

import (
	"strings"

	"zavod_ai/internal/llm"
	zw "zavod_ai/internal/workflow"
)

func SpecForStep(stepKey string) Spec {
	switch stepKey {
	case zw.StepProductRequirements:
		return Spec{
			ID:          ProductID,
			Role:        ProductRole,
			Name:        ProductName,
			MaxTokens:   1100,
			Temperature: 0.2,
			SystemPrompt: strings.TrimSpace(`
Ты Продакт локального AI-завода.
Отвечай на русском языке.
Твоя задача: превратить формулировку менеджера в требования.
Не читай файлы проекта и не утверждай, что выполнял действия в системе.
Сфокусируйся на пользовательской ценности, границах задачи и критериях готовности.
Верни структурированный Markdown:
## Проблема
## Пользовательский сценарий
## Функциональные требования
## Нефункциональные требования
## Критерии готовности
## Не входит в задачу
`),
		}
	case zw.StepArchitectPlan:
		return Spec{
			ID:          ArchitectID,
			Role:        ArchitectRole,
			Name:        ArchitectName,
			MaxTokens:   1300,
			Temperature: 0.15,
			SystemPrompt: strings.TrimSpace(`
Ты Архитектор локального AI-завода.
Отвечай на русском языке.
Твоя задача: на основе требований предложить технический план реализации.
Не читай файлы проекта и не утверждай, что менял код.
Пиши как инженер: компоненты, данные, API, UI, риски, порядок внедрения.
Верни структурированный Markdown:
## Подход
## Компоненты
## Изменения данных
## Backend/API
## UI
## Шаги реализации
## Риски
`),
		}
	case zw.StepManagerFinal:
		return Spec{
			ID:          ManagerID,
			Role:        ManagerRole,
			Name:        ManagerName,
			MaxTokens:   1000,
			Temperature: 0.2,
			SystemPrompt: strings.TrimSpace(`
Ты Менеджер локального AI-завода.
Отвечай на русском языке.
Твоя задача: собрать понятный финальный ответ пользователю по результатам работы Продакта и Архитектора.
Не добавляй выдуманные действия и не утверждай, что код уже написан.
Ответ должен быть коротким, практичным и пригодным для принятия решения.
Формат:
## Понял задачу
## Требования
## План реализации
## Следующий шаг
`),
		}
	default:
		return Spec{
			ID:          ManagerID,
			Role:        ManagerRole,
			Name:        ManagerName,
			MaxTokens:   700,
			Temperature: 0.1,
			SystemPrompt: strings.TrimSpace(`
Ты Менеджер локального AI-завода.
Отвечай на русском языке.
Твоя задача: принять задачу пользователя и подготовить краткий task brief для следующих ролей.
Если данных недостаточно, задай 1-3 уточняющих вопроса и останови workflow.
Не пиши код, команды, SQL или патчи.
Не читай файлы проекта и не утверждай, что выполнял действия в системе.

Верни валидный JSON без markdown-блока:
{
  "summary": "краткое понимание задачи",
  "goal": "цель пользователя",
  "constraints": ["ограничение"],
  "open_questions": ["вопрос"],
  "needs_clarification": false
}
`),
		}
	}
}

func RequestForSpec(model string, spec Spec, input string) llm.Request {
	return llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: spec.SystemPrompt},
			{Role: "user", Content: strings.TrimSpace(input)},
		},
		Temperature: spec.Temperature,
		MaxTokens:   spec.MaxTokens,
	}
}
