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
