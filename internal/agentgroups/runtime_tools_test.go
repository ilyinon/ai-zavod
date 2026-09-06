package agentgroups

import (
	"slices"
	"testing"
)

func TestRuntimeToolsRespectCustomAllowlist(t *testing.T) {
	profile := NormalizeCapabilities(Profile{RoleKey: "developer", ToolProfileID: "tool_go_dev", Enabled: true})
	if !slices.Contains(RuntimeTools(profile), "run_check") {
		t.Fatal("legacy dev defaults not adapted")
	}
	profile.AllowedTools = []string{"read_file"}
	if got := RuntimeTools(profile); !slices.Equal(got, []string{"read_file"}) {
		t.Fatal(got)
	}
	profile.Enabled = false
	if len(RuntimeTools(profile)) != 0 {
		t.Fatal("disabled agent got tools")
	}
}
