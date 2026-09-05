package agents

import (
	"strings"
	"testing"

	zw "zavod_ai/internal/workflow"
)

func TestRequestForSpecIncludesDefaultSkill(t *testing.T) {
	steps := []string{
		zw.StepManagerIntake,
		zw.StepProductRequirements,
		zw.StepTaskBlueprint,
		zw.StepArchitectPlan,
		zw.StepDeveloperPlan,
		zw.StepTesterCommands,
		zw.StepReview,
		zw.StepManagerFinal,
		zw.StepSecurityAnalysis,
		zw.StepWebResearch,
		zw.StepResearchSourceReview,
		zw.StepResearchSynthesis,
		zw.StepResearchNotes,
	}

	for _, step := range steps {
		req := RequestForSpec("qwen", SpecForStep(step), "input")
		if len(req.Messages) == 0 {
			t.Fatalf("%s: expected system message", step)
		}
		if !strings.Contains(req.Messages[0].Content, "$pony-tail") {
			t.Fatalf("%s: expected default skill in system prompt: %q", step, req.Messages[0].Content)
		}
	}
}

func TestRequestForSpecWithSoulIncludesSoulBeforeStepInstructions(t *testing.T) {
	spec := SpecForStep(zw.StepDeveloperPlan)
	req := RequestForSpecWithSoul("qwen", spec, "# Soul\n\nПиши аккуратно.", "input")
	if len(req.Messages) == 0 {
		t.Fatal("expected system message")
	}
	content := req.Messages[0].Content
	if !strings.Contains(content, "## Agent soul.md") {
		t.Fatalf("expected soul section, got %q", content)
	}
	if !strings.Contains(content, "Пиши аккуратно.") {
		t.Fatalf("expected soul content, got %q", content)
	}
	if !strings.Contains(content, "## Step instructions") {
		t.Fatalf("expected step instruction section, got %q", content)
	}
	if strings.Index(content, "## Agent soul.md") > strings.Index(content, "## Step instructions") {
		t.Fatalf("expected soul before step instructions, got %q", content)
	}
}
