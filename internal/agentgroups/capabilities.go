package agentgroups

import "strings"

// NormalizeCapabilities keeps custom profile fields intact and fills missing
// contracts from stable role/tool defaults.
func NormalizeCapabilities(profile Profile) Profile {
	profile.DefaultSkills = NormalizeDefaultSkills(profile.DefaultSkills)
	profile.Capabilities = cleanList(profile.Capabilities)
	profile.AllowedTools = cleanList(profile.AllowedTools)
	profile.ReadPaths = cleanList(profile.ReadPaths)
	profile.WritePaths = cleanList(profile.WritePaths)
	profile.HandoffRules = cleanList(profile.HandoffRules)
	if len(profile.DefaultSkills) == 0 {
		profile.DefaultSkills = DefaultSkillsForRole(profile.RoleKey)
	}
	if len(profile.Capabilities) == 0 {
		profile.Capabilities = defaultCapabilities(profile.RoleKey)
	}
	if len(profile.AllowedTools) == 0 {
		profile.AllowedTools = defaultAllowedTools(profile.RoleKey, profile.ToolProfileID)
	}
	if len(profile.ReadPaths) == 0 {
		profile.ReadPaths = defaultReadPaths(profile.RoleKey)
	}
	if len(profile.WritePaths) == 0 {
		profile.WritePaths = defaultWritePaths(profile.RoleKey)
	}
	if len(profile.HandoffRules) == 0 {
		profile.HandoffRules = defaultHandoffRules(profile.RoleKey)
	}
	return profile
}

func NormalizeDefaultSkills(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "$"))
		if value == "" {
			continue
		}
		value = strings.ToLower(value)
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func DefaultSkillsForRole(roleKey string) []string {
	skills := []string{"pony-tail"}
	switch normalizedRole(roleKey) {
	case "researcher", "source_reviewer", "analyst":
		skills = append(skills, "research")
	case "security", "threat_modeler", "remediator":
		skills = append(skills, "security")
	case "ctf_scout", "ctf_web", "ctf_lfi", "ctf_rce", "ctf_sqli", "ctf_pwn", "ctf_crypto", "ctf_reverse", "ctf_forensics", "ctf_validator":
		skills = append(skills, "ctf")
	case "developer", "tester", "reviewer", "architect", "docs", "release":
		skills = append(skills, "dev")
	}
	return skills
}

func defaultCapabilities(roleKey string) []string {
	switch normalizedRole(roleKey) {
	case "manager":
		return []string{"intent routing", "task spec ownership", "workflow coordination", "direct answers", "final summary"}
	case "product":
		return []string{"requirements", "acceptance criteria", "scope boundaries", "user scenarios"}
	case "architect":
		return []string{"task blueprint", "architecture plan", "risk analysis", "implementation sequencing"}
	case "developer":
		return []string{"structured code changes", "Go 1.25+ implementation", "Python implementation", "repair loop"}
	case "tester":
		return []string{"stack-aware verification", "test command selection", "test result analysis", "regression checks"}
	case "reviewer":
		return []string{"mandatory quality gate", "diff review", "requirements compliance", "return-to-role decision"}
	case "docs":
		return []string{"README", "developer setup", "build instructions", "release notes"}
	case "release":
		return []string{"release notes", "macOS build", "DMG packaging", "artifact checklist"}
	case "security":
		return []string{"security scope review", "threat analysis", "defensive recommendations", "safe testing boundaries"}
	case "threat_modeler":
		return []string{"asset inventory", "trust boundaries", "threat model", "mitigation mapping"}
	case "remediator":
		return []string{"remediation planning", "hardening checklist", "secure defaults", "risk prioritization"}
	case "researcher":
		return []string{"web search", "source collection", "citation notes", "freshness checks"}
	case "analyst":
		return []string{"source comparison", "fact synthesis", "assumption tracking", "concise conclusions"}
	case "source_reviewer":
		return []string{"source quality review", "link validation", "recency check", "citation hygiene"}
	case "ctf_scout", "scout":
		return []string{"CTF scope check", "artifact inventory", "category triage", "hypothesis board"}
	case "ctf_web":
		return []string{"web CTF analysis", "HTTP probing in scope", "payload notes", "flag extraction"}
	case "ctf_lfi":
		return []string{"LFI/path traversal analysis", "safe payload planning", "evidence capture", "flag extraction"}
	case "ctf_rce":
		return []string{"RCE challenge analysis", "command injection reasoning", "scope-safe payload notes", "flag extraction"}
	case "ctf_sqli":
		return []string{"SQLi challenge analysis", "query payload reasoning", "database behavior notes", "flag extraction"}
	case "ctf_pwn":
		return []string{"binary triage", "exploit strategy", "local debugging", "solver notes"}
	case "ctf_crypto":
		return []string{"crypto analysis", "solver scripting", "attack selection", "flag recovery"}
	case "ctf_reverse":
		return []string{"reverse engineering", "static analysis", "behavior notes", "flag recovery"}
	case "ctf_forensics":
		return []string{"file forensics", "metadata analysis", "dump inspection", "evidence timeline"}
	case "ctf_validator", "validator":
		return []string{"flag validation", "reproduction check", "writeup review", "scope compliance"}
	default:
		return []string{"role-specific task execution", "handoff context", "result validation"}
	}
}

func defaultAllowedTools(roleKey, toolProfileID string) []string {
	switch strings.TrimSpace(toolProfileID) {
	case "tool_go_dev":
		return []string{"gofmt", "go test ./...", "go vet ./...", "go mod/make/wails only with confirmation"}
	case "tool_python_dev":
		return []string{".venv/bin/python <script.py>", ".venv/bin/python -m pytest", ".venv/bin/python -m py_compile", "backend-managed virtualenv setup"}
	case "tool_research":
		return []string{"web_search", "fetch_url", "source_notes"}
	case "tool_ctf_web":
		return []string{".venv/bin/python solve/*.py", "file/strings local artifacts", "curl/dig/whois only with explicit CTF scope"}
	case "tool_ctf_lfi":
		return []string{".venv/bin/python solve/*.py", "file/strings local artifacts", "curl only with explicit CTF scope", "LFI payload notes in evidence"}
	case "tool_ctf_rce":
		return []string{".venv/bin/python solve/*.py", "file/strings local artifacts", "curl only with explicit CTF scope", "RCE payload notes in evidence"}
	case "tool_ctf_sqli":
		return []string{".venv/bin/python solve/*.py", "file/strings local artifacts", "curl/sqlmap only with explicit CTF scope"}
	case "tool_ctf_pwn":
		return []string{"file", "strings", "checksec", "readelf", "objdump", "nm", ".venv/bin/python with pwntools", "gdb/ROPgadget only with confirmation"}
	case "tool_ctf_crypto":
		return []string{".venv/bin/python crypto solvers", "file/strings local artifacts", "sage only with confirmation"}
	case "tool_ctf_reverse":
		return []string{"file", "strings", "readelf", "objdump", "nm", ".venv/bin/python helper scripts", "radare2/r2 only with confirmation"}
	case "tool_ctf_forensics":
		return []string{"file", "strings", "exiftool", "binwalk without extract", "xxd", ".venv/bin/python helpers", "binwalk extract/foremost/tshark only with confirmation"}
	case "tool_ctf_validator":
		return []string{".venv/bin/python validate scripts", "file/strings local artifacts", "writeup/evidence consistency checks"}
	}
	switch normalizedRole(roleKey) {
	case "manager", "product", "architect", "reviewer", "researcher", "analyst", "source_reviewer", "security", "threat_modeler", "remediator":
		return []string{"read project context", "write workflow notes"}
	case "developer", "tester":
		return []string{"project-local shell commands", "language test commands", "diff inspection"}
	default:
		return []string{"read project context"}
	}
}

func defaultReadPaths(roleKey string) []string {
	switch normalizedRole(roleKey) {
	case "manager", "product", "architect", "reviewer", "docs", "release", "researcher", "analyst", "source_reviewer", "security", "threat_modeler", "remediator":
		return []string{"README*", "docs/**", "zavod_ai/**", "frontend/**", "internal/**", "*.go", "*.py", "go.mod", "requirements.txt"}
	case "ctf_scout", "scout", "ctf_web", "ctf_lfi", "ctf_rce", "ctf_sqli", "ctf_pwn", "ctf_crypto", "ctf_reverse", "ctf_forensics", "ctf_validator", "validator":
		return []string{"README*", "ctf/**", "artifacts/**", "evidence/**", "solve/**", "*.pcap", "*.bin", "*.txt"}
	default:
		return []string{"./**", "!./.git/**", "!./.env*"}
	}
}

func defaultWritePaths(roleKey string) []string {
	switch normalizedRole(roleKey) {
	case "manager", "product", "architect", "reviewer", "researcher", "analyst", "source_reviewer", "security", "threat_modeler", "remediator":
		return []string{"docs/**"}
	case "developer", "tester":
		return []string{"./**", "!./.git/**", "!./.env*"}
	case "docs", "release":
		return []string{"README*", "docs/**", "CHANGELOG*"}
	case "ctf_scout", "scout", "ctf_web", "ctf_lfi", "ctf_rce", "ctf_sqli", "ctf_pwn", "ctf_crypto", "ctf_reverse", "ctf_forensics", "ctf_validator", "validator":
		return []string{"ctf/**", "evidence/**", "solve/**", "writeup.md"}
	default:
		return []string{"docs/**"}
	}
}

func defaultHandoffRules(roleKey string) []string {
	switch normalizedRole(roleKey) {
	case "manager":
		return []string{"Answer directly when the request is informational and context is already available.", "Route coding work to Product/Architect/Developer.", "Send final summary only after checks and review are accepted."}
	case "product":
		return []string{"Pass to Architect after requirements and acceptance criteria are clear.", "Return to Manager if user intent or scope is ambiguous."}
	case "architect":
		return []string{"Pass to Developer after blueprint covers files, stack, risks and tests.", "Return to Product if requirements conflict or are incomplete."}
	case "developer":
		return []string{"Pass to Tester after structured changes are applied.", "Return to Architect if implementation requires a design decision."}
	case "tester":
		return []string{"Pass to Reviewer when relevant checks pass.", "Return to Developer with concrete failure output when checks fail."}
	case "reviewer":
		return []string{"Accept only when requirements, diff and checks agree.", "Return to Developer for code defects.", "Return to Product/Architect for scope or design gaps."}
	case "researcher":
		return []string{"Pass collected sources to Source Reviewer.", "Save source notes and research notes.", "Return to Manager if the query contains secrets or private data."}
	case "source_reviewer":
		return []string{"Pass accepted sources to Analyst.", "Return to Researcher when sources are stale, weak, broken or insufficient.", "Flag contradictions explicitly."}
	case "analyst":
		return []string{"Pass concise synthesis to Manager/final answer.", "Separate source facts from inference.", "Return to Source Reviewer when citations do not support the conclusion."}
	case "security", "ctf_scout", "scout":
		return []string{"Ask for explicit scope before active testing.", "Route to the matching CTF specialist by category.", "Stop and summarize if the task is outside allowed scope."}
	case "ctf_web", "ctf_lfi", "ctf_rce", "ctf_sqli", "ctf_pwn", "ctf_crypto", "ctf_reverse", "ctf_forensics":
		return []string{"Work only within stated CTF/lab scope.", "Pass to Validator with exploit steps, evidence and flag candidate.", "Return to Scout if category or artifacts are wrong."}
	case "ctf_validator", "validator":
		return []string{"Accept only reproducible flags and writeups.", "Return to category specialist when proof is incomplete."}
	default:
		return []string{"Pass forward when the current responsibility is complete.", "Return to the previous role when required context is missing."}
	}
}

func normalizedRole(roleKey string) string {
	return strings.ToLower(strings.TrimSpace(roleKey))
}

func cleanList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}
