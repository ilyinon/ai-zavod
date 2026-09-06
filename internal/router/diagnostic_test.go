package router

import "testing"

func TestDiagnosticRoutesWithoutWorkflow(t *testing.T) {
	for _, message := range []string{"разберись, почему падают тесты", "почему падает сборка", "найди причину ошибки"} {
		result := Route(message, Context{})
		if result.Intent != IntentProjectAnalysis || result.NeedsWorkflow {
			t.Fatalf("%s: %+v", message, result)
		}
	}
	if result := Route("исправь падающие тесты", Context{}); result.Intent != IntentCodingTask || !result.NeedsWorkflow {
		t.Fatal(result)
	}
}
