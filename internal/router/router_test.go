package router

import "testing"

func TestRouteDirectSpecQuestion(t *testing.T) {
	decision := Route("опиши спеку по которой работала", Context{})
	if decision.Intent != IntentDirectAnswer || decision.NeedsWorkflow {
		t.Fatalf("expected direct answer without workflow, got %#v", decision)
	}
	if !decision.NeedsProjectContext {
		t.Fatalf("expected project context for spec question, got %#v", decision)
	}
}

func TestRouteCodingTask(t *testing.T) {
	decision := Route("исправь скрипт проверки, оставляем язык Go", Context{})
	if decision.Intent != IntentCodingTask || !decision.NeedsWorkflow {
		t.Fatalf("expected coding workflow, got %#v", decision)
	}
}

func TestRouteWriteProgramAsCodingTask(t *testing.T) {
	decision := Route("Напиши на Go проверку сайтов на доступность по HTTP, должны принимать аргумент сайтов", Context{})
	if decision.Intent != IntentCodingTask || !decision.NeedsWorkflow {
		t.Fatalf("expected write-program request to start coding workflow, got %#v", decision)
	}
}

func TestRouteClarificationAnswerFromNativeUI(t *testing.T) {
	decision := Route("Ответы на уточнения:\n1. Нужен новый go.mod", Context{})
	if decision.Intent != IntentClarificationAnswer || !decision.NeedsWorkflow {
		t.Fatalf("expected clarification workflow continuation, got %#v", decision)
	}
}

func TestRoutePentestTask(t *testing.T) {
	decision := Route("проверь проект на уязвимости и сделай threat model", Context{})
	if decision.Intent != IntentPentestTask || !decision.NeedsWorkflow {
		t.Fatalf("expected pentest workflow, got %#v", decision)
	}
}

func TestRouteCTFTask(t *testing.T) {
	decision := Route("реши CTF web challenge с LFI и подготовь writeup", Context{})
	if decision.Intent != IntentPentestTask || !decision.NeedsWorkflow {
		t.Fatalf("expected CTF task to use pentest workflow route, got %#v", decision)
	}
}

func TestRouteSecurityCodingWordsToSecurityWorkflow(t *testing.T) {
	decision := Route("исправь sql injection и сделай аудит безопасности", Context{})
	if decision.Intent != IntentPentestTask || !decision.NeedsWorkflow {
		t.Fatalf("expected security workflow before coding, got %#v", decision)
	}
}

func TestRouteWorkflowControl(t *testing.T) {
	decision := Route("покажи diff последнего запуска", Context{})
	if decision.Intent != IntentWorkflowControl || decision.NeedsWorkflow {
		t.Fatalf("expected workflow control without new workflow, got %#v", decision)
	}
}
