package llm

import (
	"context"
	"fmt"
	"time"
)

// ProviderError keeps a safe public explanation separate from transport details.
type ProviderError struct {
	Kind        string        `json:"kind"`
	Retryable   bool          `json:"retryable"`
	ModelID     string        `json:"modelId"`
	HTTPStatus  int           `json:"httpStatus,omitempty"`
	Attempt     int           `json:"attempt"`
	MaxAttempts int           `json:"maxAttempts"`
	ElapsedMS   int64         `json:"elapsedMs"`
	RetryAfter  time.Duration `json:"-"`
	Cause       error         `json:"-"`
}

func (e *ProviderError) Error() string {
	labels := map[string]string{
		"provider_timeout":          "Модель не ответила вовремя",
		"provider_unavailable":      "Сервис модели недоступен",
		"provider_rate_limited":     "Сервис модели ограничил частоту запросов",
		"provider_auth":             "Нет доступа к модели: проверь ключ и права",
		"provider_invalid_request":  "Некорректный запрос или конфигурация модели",
		"provider_invalid_response": "Модель вернула некорректный ответ",
		"cancelled":                 "Запрос отменён",
		"execution_deadline":        "Истёк общий лимит времени запроса",
	}
	text := labels[e.Kind]
	if text == "" {
		text = "Ошибка соединения с моделью"
	}
	if e.HTTPStatus > 0 {
		text += fmt.Sprintf(" (HTTP %d)", e.HTTPStatus)
	}
	if e.Attempt > 0 {
		text += fmt.Sprintf(". Попыток: %d/%d; %.1f с", e.Attempt, e.MaxAttempts, float64(e.ElapsedMS)/1000)
	}
	return text
}
func (e *ProviderError) Unwrap() error { return e.Cause }

type RetryEvent struct {
	Attempt, MaxAttempts int
	Delay                time.Duration
	Failure              *ProviderError
}
type retryObserverKey struct{}

func WithRetryObserver(ctx context.Context, f func(RetryEvent)) context.Context {
	return context.WithValue(ctx, retryObserverKey{}, f)
}
func NotifyRetry(ctx context.Context, e RetryEvent) {
	if f, ok := ctx.Value(retryObserverKey{}).(func(RetryEvent)); ok {
		f(e)
	}
}
