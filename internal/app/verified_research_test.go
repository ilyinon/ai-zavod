package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/webresearch"
)

func TestVerifiedResearchRequiresSourceEvidence(t *testing.T) {
	// Synthetic source, not a claim about a real model.
	sources := []webresearch.Source{{Title: "Example model", URL: "https://example.com/models", ContentExcerpt: "Example model supports text input."}}
	valid := `{"status":"answered","facts":[{"subject":"Example model","text":"Поддерживает текстовый ввод.","source_url":"https://example.com/models","evidence":"Example model supports text input."}]}`
	answer := renderVerifiedResearch(valid, sources)
	if !strings.Contains(answer, "Поддерживает текстовый ввод.") || !strings.Contains(answer, "[Источник](https://example.com/models)") || strings.Contains(answer, `{"`) {
		t.Fatal(answer)
	}
	for name, raw := range map[string]string{
		"invented company":      strings.ReplaceAll(valid, `"subject":"Example model"`, `"subject":"Astro"`),
		"invented source":       strings.ReplaceAll(valid, "example.com/models", "invented.example/models"),
		"invented quote":        strings.ReplaceAll(valid, "Example model supports text input.", "Example model is built by Astro."),
		"ambiguous":             `{"status":"needs_clarification","facts":[]}`,
		"insufficient":          `{"status":"insufficient_sources","facts":[]}`,
		"no facts":              `{"status":"answered","facts":[]}`,
		"old ungrounded answer": "Astra создана компанией Astro, Sol компанией SOL.",
		"invalid JSON":          `{"status":`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := renderVerifiedResearch(raw, sources); got != unverifiedProductAnswer {
				t.Fatal(got)
			}
		})
	}
	if got := renderVerifiedResearch(valid, nil); got != unverifiedProductAnswer {
		t.Fatal(got)
	}
}

func TestVerifiedResearchGenerationDoesNotFallBackToModelMemory(t *testing.T) {
	sources := []webresearch.Source{{Title: "Example model", URL: "https://example.com/models", ContentExcerpt: "Example model supports text input."}}
	calls := 0
	provider := researchAnswerProvider{generate: func(ctx context.Context, req llm.Request) (*llm.Response, error) {
		calls++
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "system" || last.Content != verifiedResearchFormat {
			t.Fatal("missing evidence contract")
		}
		return &llm.Response{Content: "Модель Astra разработала компания Astro."}, nil
	}}
	if got := generateResearchAnswerWithVerification(context.Background(), provider, llm.Request{}, nil, true); got != unverifiedProductAnswer || calls != 0 {
		t.Fatal(got, calls)
	}
	if got := generateResearchAnswerWithVerification(context.Background(), provider, llm.Request{}, sources, true); got != unverifiedProductAnswer || calls != 1 {
		t.Fatal(got, calls)
	}
}

func TestProjectResearchUsesSameEvidenceContract(t *testing.T) {
	input := buildResearchSynthesisInput("Чем отличается модель Astra от Sol?", project.Project{}, webresearch.Plan{}, nil, "")
	if !strings.Contains(input, verifiedResearchFormat) {
		t.Fatal("project synthesis missing verification")
	}
	input = buildResearchSynthesisInput("Найди историю сортировки пузырьком", project.Project{}, webresearch.Plan{}, nil, "")
	if strings.Contains(input, verifiedResearchFormat) {
		t.Fatal("ordinary research received comparison contract")
	}
}

func TestVerifiedResearchEscapesModelMarkup(t *testing.T) {
	raw := map[string]any{"status": "answered", "facts": []map[string]string{{"subject": "Example model", "text": "[bad](https://invented.example)", "source_url": "https://example.com/models", "evidence": "Example model supports text input."}}}
	data, _ := json.Marshal(raw)
	answer := renderVerifiedResearch(string(data), []webresearch.Source{{Title: "Example model", URL: "https://example.com/models", ContentExcerpt: "Example model supports text input."}})
	if !strings.Contains(answer, `\[bad\]`) {
		t.Fatal(answer)
	}
}
