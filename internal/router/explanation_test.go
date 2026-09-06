package router

import "testing"

func TestExplanationVersusInstruction(t *testing.T) {
	cases := []struct {
		text    string
		intent  Intent
		clarify bool
	}{
		{"как написать сортировку пузырько в Go lang?", IntentDirectAnswer, false},
		{"Как написать сортировку пузырьком на Go", IntentDirectAnswer, false},
		{"Объясни, как реализовать HTTP-сервер на Go", IntentDirectAnswer, false},
		{"Покажи пример сортировки, ничего не создавай", IntentDirectAnswer, false},
		{"Что означает RCE?", IntentDirectAnswer, false},
		{"В логе написано “исправь файл”. Что это значит?", IntentDirectAnswer, false},
		{"Напиши здесь пример, файлы не меняй", IntentDirectAnswer, false},
		{"Реализуй сортировку в этом проекте", IntentCodingTask, false},
		{"Можешь исправить sort.go?", IntentCodingTask, false},
		{"Объясни алгоритм и добавь его в sort.go", IntentCodingTask, false},
		{"Реализуй это", IntentCodingTask, false},
		{"Нужна сортировка", IntentDirectAnswer, true},
		{"Почему падают тесты этого проекта?", IntentProjectAnalysis, false},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			for _, pending := range []bool{false, true} {
				got := Route(tc.text, Context{HasActiveClarification: pending})
				if got.Intent != tc.intent || got.NeedsClarification != tc.clarify {
					t.Fatalf("unexpected route: %#v", got)
				}
				if tc.intent == IntentDirectAnswer && got.NeedsWorkflow {
					t.Fatal("explanation starts workflow")
				}
			}
		})
	}
}
