package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"zavod_ai/internal/llm"
	"zavod_ai/internal/webresearch"
)

type researchAnswerProvider struct {
	staticProvider
	generate func(context.Context, llm.Request) (*llm.Response, error)
}

func (p researchAnswerProvider) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	return p.generate(ctx, req)
}

func TestResearchAnswerFallbackPreservesWeatherFacts(t *testing.T) {
	sources := []webresearch.Source{
		{SourceType: "web", Title: "Unrelated page", ContentExcerpt: "irrelevant text"},
		{SourceType: "weather", Title: "Погода: Нижний Новгород, Россия", ContentExcerpt: "Данные на 2026-09-06T14:15. Температура: 16.2 °C. Ветер: 8.1 km/h.", URL: "https://api.open-meteo.com/v1/forecast?latitude=56.3287&longitude=44.0020"},
	}
	for _, tc := range []struct {
		name     string
		response *llm.Response
		err      error
	}{
		{"timeout", nil, context.DeadlineExceeded},
		{"provider error", nil, errors.New("private provider response")},
		{"nil", nil, nil},
		{"empty", &llm.Response{}, nil},
		{"JSON", &llm.Response{Content: `{"summary":"raw"}`}, nil},
		{"too long", &llm.Response{Content: strings.Repeat("a", managerMaxAnswerBytes+1)}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := researchAnswerProvider{generate: func(context.Context, llm.Request) (*llm.Response, error) { return tc.response, tc.err }}
			answer := generateResearchAnswer(context.Background(), provider, llm.Request{}, sources)
			for _, want := range []string{"Нижний Новгород", "16.2 °C", "8.1 km/h", "2026-09-06T14:15", "[Источник: Open-Meteo](https://api.open-meteo.com/"} {
				if !strings.Contains(answer, want) {
					t.Fatalf("missing %q in %s", want, answer)
				}
			}
			for _, unwanted := range []string{"Нашла источники", "irrelevant text", "private provider response", `{"summary"`} {
				if strings.Contains(answer, unwanted) {
					t.Fatalf("unexpected %q in %s", unwanted, answer)
				}
			}
		})
	}
}

func TestResearchAnswerBudgetAndCancellation(t *testing.T) {
	ctx := context.Background()
	searchCtx, cancelSearch := context.WithCancel(ctx)
	cancelSearch()
	if searchCtx.Err() == nil {
		t.Fatal("search must have finished")
	}
	provider := researchAnswerProvider{generate: func(answerCtx context.Context, req llm.Request) (*llm.Response, error) {
		deadline, ok := answerCtx.Deadline()
		if !ok || time.Until(deadline) < 175*time.Second || answerCtx.Err() != nil {
			t.Fatal("synthesis did not receive a fresh budget")
		}
		return &llm.Response{Content: "Ответ по источникам."}, nil
	}}
	if got := generateResearchAnswer(ctx, provider, llm.Request{}, nil); got != "Ответ по источникам." {
		t.Fatal(got)
	}
	parent, cancel := context.WithCancel(ctx)
	cancel()
	provider.generate = func(answerCtx context.Context, _ llm.Request) (*llm.Response, error) {
		if !errors.Is(answerCtx.Err(), context.Canceled) {
			t.Fatal("user cancellation was ignored")
		}
		return nil, answerCtx.Err()
	}
	generateResearchAnswer(parent, provider, llm.Request{}, nil)
}

func TestResearchFallbackDoesNotInventWeather(t *testing.T) {
	for _, sources := range [][]webresearch.Source{nil, {{SourceType: "web", ContentExcerpt: "a page"}}, {{SourceType: "weather"}}} {
		if got := webResearchFallbackAnswer(sources); !strings.Contains(got, "Нашла источники") || strings.Contains(got, "Open-Meteo") {
			t.Fatal(got)
		}
	}
}
