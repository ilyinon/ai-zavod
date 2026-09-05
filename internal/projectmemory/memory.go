package projectmemory

import "strings"

type Memory struct {
	ID                string   `json:"id"`
	ProjectID         string   `json:"projectId"`
	Architecture      string   `json:"architecture"`
	Stack             string   `json:"stack"`
	Runtime           string   `json:"runtime"`
	ProjectType       string   `json:"projectType"`
	BuildCommands     []string `json:"buildCommands"`
	TestCommands      []string `json:"testCommands"`
	StyleGuide        []string `json:"styleGuide"`
	Decisions         []string `json:"decisions"`
	Environment       []string `json:"environment"`
	UpdatedFromTaskID string   `json:"updatedFromTaskId"`
	CreatedAt         string   `json:"createdAt"`
	UpdatedAt         string   `json:"updatedAt"`
}

func RenderMarkdown(memory Memory) string {
	var builder strings.Builder
	builder.WriteString("# Project Memory\n\n")
	writeSection(&builder, "Architecture", firstNonEmpty(memory.Architecture, "Не зафиксирована."))
	writeSection(&builder, "Stack", firstNonEmpty(memory.Stack, "Не зафиксирован."))
	writeSection(&builder, "Runtime", firstNonEmpty(memory.Runtime, "Не зафиксирован."))
	writeSection(&builder, "Project Type", firstNonEmpty(memory.ProjectType, "Не зафиксирован."))
	writeListSection(&builder, "Build Commands", memory.BuildCommands, "Команды сборки пока не зафиксированы.")
	writeListSection(&builder, "Test Commands", memory.TestCommands, "Команды проверки пока не зафиксированы.")
	writeListSection(&builder, "Style Guide", memory.StyleGuide, "Style guide пока не зафиксирован.")
	writeListSection(&builder, "Past Decisions", memory.Decisions, "Прошлые решения пока не зафиксированы.")
	writeListSection(&builder, "Environment Notes", memory.Environment, "Особенности окружения пока не зафиксированы.")
	return strings.TrimSpace(builder.String()) + "\n"
}

func Merge(base Memory, patch Memory) Memory {
	if strings.TrimSpace(base.ProjectID) == "" {
		base.ProjectID = strings.TrimSpace(patch.ProjectID)
	}
	if strings.TrimSpace(patch.Architecture) != "" {
		base.Architecture = strings.TrimSpace(patch.Architecture)
	}
	if strings.TrimSpace(patch.Stack) != "" {
		base.Stack = strings.Join(MergeStringLists(splitStack(base.Stack), splitStack(patch.Stack)), ", ")
	}
	if strings.TrimSpace(patch.Runtime) != "" {
		base.Runtime = strings.TrimSpace(patch.Runtime)
	}
	if strings.TrimSpace(patch.ProjectType) != "" {
		base.ProjectType = strings.TrimSpace(patch.ProjectType)
	}
	if strings.TrimSpace(patch.UpdatedFromTaskID) != "" {
		base.UpdatedFromTaskID = strings.TrimSpace(patch.UpdatedFromTaskID)
	}
	base.BuildCommands = MergeStringLists(base.BuildCommands, patch.BuildCommands)
	base.TestCommands = MergeStringLists(base.TestCommands, patch.TestCommands)
	base.StyleGuide = MergeStringLists(base.StyleGuide, patch.StyleGuide)
	base.Decisions = MergeStringLists(base.Decisions, patch.Decisions)
	base.Environment = MergeStringLists(base.Environment, patch.Environment)
	return base
}

func MergeStringLists(base []string, next []string) []string {
	out := make([]string, 0, len(base)+len(next))
	seen := map[string]bool{}
	for _, item := range append(base, next...) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}

func writeSection(builder *strings.Builder, title string, content string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString(strings.TrimSpace(content))
	builder.WriteString("\n\n")
}

func writeListSection(builder *strings.Builder, title string, items []string, fallback string) {
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	clean := MergeStringLists(nil, items)
	if len(clean) == 0 {
		builder.WriteString(fallback)
		builder.WriteString("\n\n")
		return
	}
	for _, item := range clean {
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func splitStack(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '/'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
