package orchestration

import (
	"strings"
	"testing"

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/router"
)

func TestDecideDirectAnswerKeepsCurrentGroupAndSkipsWorkflow(t *testing.T) {
	dev := agentgroups.Group{ID: "group_dev_squad", Name: "Dev Squad", Kind: agentgroups.GroupKindDev, DefaultLifecycleID: "lifecycle_dev_default"}
	decision := Decide(Input{
		Message:        "выведи сюда спеку этого задания",
		RouterDecision: router.Route("выведи сюда спеку этого задания", router.Context{}),
		CurrentGroup:   &dev,
		AvailableGroups: []agentgroups.Group{
			dev,
		},
		HasTaskSpec: true,
	})

	if decision.Mode != ModeDirect || decision.NeedsWorkflow {
		t.Fatalf("expected direct answer, got %#v", decision)
	}
	if decision.GroupID != dev.ID {
		t.Fatalf("expected current group, got %#v", decision)
	}
	if !decision.UsedMemory || !strings.Contains(decision.Explanation, "живая спека") {
		t.Fatalf("expected memory/spec explanation, got %#v", decision)
	}
	if len(decision.SkippedSteps) == 0 {
		t.Fatalf("expected skipped workflow steps, got %#v", decision)
	}
}

func TestDecideResearchSelectsResearchGroup(t *testing.T) {
	dev := agentgroups.Group{ID: "group_dev_squad", Name: "Dev Squad", Kind: agentgroups.GroupKindDev, DefaultLifecycleID: "lifecycle_dev_default"}
	research := agentgroups.Group{ID: "group_research_squad", Name: "Research Squad", Kind: agentgroups.GroupKindResearch, DefaultLifecycleID: "lifecycle_research_default"}
	decision := Decide(Input{
		Message:        "загугли погоду в Минске",
		RouterDecision: router.Route("загугли погоду в Минске", router.Context{}),
		CurrentGroup:   &dev,
		AvailableGroups: []agentgroups.Group{
			dev,
			research,
		},
		HasProjectMemory: true,
	})

	if decision.Mode != ModeWorkflow || !decision.NeedsWorkflow {
		t.Fatalf("expected workflow, got %#v", decision)
	}
	if decision.GroupID != research.ID || decision.LifecycleID != "lifecycle_research_default" {
		t.Fatalf("expected research group/lifecycle, got %#v", decision)
	}
	if !strings.Contains(decision.Explanation, "Research Squad") {
		t.Fatalf("expected explanation to mention selected group, got %q", decision.Explanation)
	}
}

func TestDecideCTFSelectsCTFGroup(t *testing.T) {
	dev := agentgroups.Group{ID: "group_dev_squad", Name: "Dev Squad", Kind: agentgroups.GroupKindDev, DefaultLifecycleID: "lifecycle_dev_default"}
	ctfGroup := agentgroups.Group{ID: "group_ctf_cell", Name: "CTF Cell", Kind: agentgroups.GroupKindCTF, DefaultLifecycleID: "lifecycle_ctf_default"}
	decision := Decide(Input{
		Message:        "реши CTF web challenge с LFI",
		RouterDecision: router.Route("реши CTF web challenge с LFI", router.Context{}),
		CurrentGroup:   &dev,
		AvailableGroups: []agentgroups.Group{
			dev,
			ctfGroup,
		},
	})

	if decision.GroupID != ctfGroup.ID || decision.GroupKind != agentgroups.GroupKindCTF {
		t.Fatalf("expected CTF group, got %#v", decision)
	}
}

func TestDecideSecurityAuditSelectsSecurityGroup(t *testing.T) {
	dev := agentgroups.Group{ID: "group_dev_squad", Name: "Dev Squad", Kind: agentgroups.GroupKindDev, DefaultLifecycleID: "lifecycle_dev_default"}
	security := agentgroups.Group{ID: "group_security_audit", Name: "Security Audit", Kind: agentgroups.GroupKindSecurity, DefaultLifecycleID: "lifecycle_security_default"}
	decision := Decide(Input{
		Message:        "проверь проект на уязвимости и сделай threat model",
		RouterDecision: router.Route("проверь проект на уязвимости и сделай threat model", router.Context{}),
		CurrentGroup:   &dev,
		AvailableGroups: []agentgroups.Group{
			dev,
			security,
		},
	})

	if decision.GroupID != security.ID || decision.GroupKind != agentgroups.GroupKindSecurity {
		t.Fatalf("expected security group, got %#v", decision)
	}
}

func TestDecidePlainSQLiAuditDoesNotSelectCTF(t *testing.T) {
	dev := agentgroups.Group{ID: "group_dev_squad", Name: "Dev Squad", Kind: agentgroups.GroupKindDev, DefaultLifecycleID: "lifecycle_dev_default"}
	ctfGroup := agentgroups.Group{ID: "group_ctf_cell", Name: "CTF Cell", Kind: agentgroups.GroupKindCTF, DefaultLifecycleID: "lifecycle_ctf_default"}
	security := agentgroups.Group{ID: "group_security_audit", Name: "Security Audit", Kind: agentgroups.GroupKindSecurity, DefaultLifecycleID: "lifecycle_security_default"}
	decision := Decide(Input{
		Message:        "исправь sql injection и сделай аудит безопасности",
		RouterDecision: router.Route("исправь sql injection и сделай аудит безопасности", router.Context{}),
		CurrentGroup:   &dev,
		AvailableGroups: []agentgroups.Group{
			dev,
			ctfGroup,
			security,
		},
	})

	if decision.GroupID != security.ID || decision.GroupKind != agentgroups.GroupKindSecurity {
		t.Fatalf("expected security group for plain SQLi audit, got %#v", decision)
	}
}
