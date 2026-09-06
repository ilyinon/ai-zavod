package router

import "testing"

func TestCurrentProductFactsRequireResearch(t *testing.T) {
	for _, text := range []string{
		"чем отличается модель Astra от Open AI от Sol ?",
		"Объясни разницу между моделями Astra и Sol",
		"Сравни модели `Astra` и `Sol`",
		"Какая модель Qwen сейчас лучше?",
		"Сколько стоит подписка OpenAI?",
		"Compare GPT and Claude",
		"Сравни ноутбуки MacBook Air и MacBook Pro",
	} {
		decision := Route(text, Context{})
		if decision.Intent != IntentResearchTask || !decision.NeedsWorkflow {
			t.Fatalf("%q: %+v", text, decision)
		}
	}
}

func TestCurrentFactsDoesNotHijackConceptsOrCoding(t *testing.T) {
	for _, text := range []string{
		"Чем отличается линейная модель от логистической?",
		"Чем отличается модель данных от схемы?",
		"Что такое языковая модель?",
		"Как написать сортировку пузырьком на Go?",
		"Добавь выбор модели GPT в настройки",
		"Объясни код: `compare GPT and Claude`",
	} {
		if NeedsCurrentSources(text) {
			t.Fatalf("unexpected source gate: %q", text)
		}
	}
	if got := Route("Добавь выбор модели GPT в настройки", Context{}); got.Intent != IntentCodingTask {
		t.Fatal(got)
	}
}
