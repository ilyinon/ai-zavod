package router

import "testing"

func TestWeatherNeedsResearchWithoutSearchCommand(t *testing.T) {
	for _, question := range []string{
		"Какая погода в нижнем Новгороде?", "Какая погода в Нижнем Новгороде", "Погода в Минске", "Какой прогноз погоды в Москве?", "Какая погода сейчас в Сочи?", "Weather in London today?",
	} {
		t.Run(question, func(t *testing.T) {
			d := Route(question, Context{})
			if d.Intent != IntentResearchTask || !d.NeedsWorkflow || d.NeedsProjectContext || d.NeedsClarification {
				t.Fatalf("weather bypasses retrieval: %+v", d)
			}
		})
	}
}

func TestWeatherLearningDoesNotStartResearch(t *testing.T) {
	for _, question := range []string{"Что такое погода?", "Объясни, как работает API погоды", "Как получить погоду через API на Go?", "Как формируется прогноз погоды?"} {
		if d := Route(question, Context{}); d.Intent != IntentDirectAnswer || d.NeedsWorkflow {
			t.Fatalf("%q: %+v", question, d)
		}
	}
	if d := Route("Напиши на Go скрипт прогноза погоды", Context{}); d.Intent != IntentCodingTask {
		t.Fatalf("lost coding intent: %+v", d)
	}
	if d := Route("Объясни прогноз погоды с источниками", Context{}); d.Intent != IntentResearchTask {
		t.Fatalf("explicit sources ignored: %+v", d)
	}
}
