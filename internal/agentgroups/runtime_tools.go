package agentgroups

import "slices"

// Legacy defaults were human-readable command descriptions. Only an exact
// default contract is adapted; custom allowlists must name tools explicitly.
func RuntimeTools(profile Profile) []string {
	if !profile.Enabled {
		return nil
	}
	tools := []string{}
	legacy := slices.Equal(profile.AllowedTools, defaultAllowedTools(profile.RoleKey, profile.ToolProfileID))
	for _, name := range []string{"list_files", "read_file", "search_files", "run_check"} {
		permitted := slices.Contains(profile.AllowedTools, name)
		if name != "run_check" && (legacy || slices.Contains(profile.AllowedTools, "read project context")) {
			permitted = true
		}
		if name == "run_check" && legacy && (profile.ToolProfileID == "tool_go_dev" || profile.ToolProfileID == "tool_python_dev") {
			permitted = true
		}
		if permitted {
			tools = append(tools, name)
		}
	}
	return tools
}
