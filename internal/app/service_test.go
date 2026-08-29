package app

import (
	"strings"
	"testing"
)

func TestLooksLikeRepetitionLoopDetectsRepeatedAgentPrefix(t *testing.T) {
	text := strings.Repeat("Агент manager: ", repetitionLoopMinRepeats)
	if !looksLikeRepetitionLoop(text) {
		t.Fatal("expected repeated agent prefix to be detected")
	}
}

func TestLooksLikeRepetitionLoopAllowsNormalAnswer(t *testing.T) {
	text := "Понял задачу. Следующий шаг: уточнить цель, входные данные и критерии готовности."
	if looksLikeRepetitionLoop(text) {
		t.Fatal("normal answer was detected as repetition loop")
	}
}

func TestManagerNeedsClarificationFromJSON(t *testing.T) {
	output := `{"summary":"мало данных","needs_clarification":true}`
	if !managerNeedsClarification(output) {
		t.Fatal("expected clarification flag from json")
	}
}

func TestFormatClarificationMessageIsHumanReadable(t *testing.T) {
	message := formatClarificationMessage(managerIntakeResult{
		Summary:       "нужно разработать проверку локальной LLM",
		Goal:          "понять, доступна ли модель",
		OpenQuestions: []string{"Какой endpoint проверять?", "Какие критерии успеха?"},
	})

	if strings.Contains(message, `"summary"`) || strings.Contains(message, "needs_clarification") {
		t.Fatalf("expected human-readable message, got %q", message)
	}
	if !strings.Contains(message, "Понял задачу") || !strings.Contains(message, "Что нужно уточнить") {
		t.Fatalf("expected clarification sections, got %q", message)
	}
}
