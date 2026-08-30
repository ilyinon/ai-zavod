package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Artifact struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	TaskID        string `json:"taskId"`
	WorkflowRunID string `json:"workflowRunId"`
	AgentID       string `json:"agentId"`
	Kind          string `json:"kind"`
	Title         string `json:"title"`
	Path          string `json:"path"`
	RelativePath  string `json:"relativePath"`
	CreatedAt     string `json:"createdAt"`
}

type WorkflowOutput struct {
	ProjectPath   string
	TaskTitle     string
	WorkflowRunID string
	Intake        string
	Product       string
	Blueprint     string
	Architect     string
	Developer     string
	Final         string
	CreatedAt     time.Time
}

type WrittenFile struct {
	Kind         string
	Title        string
	AgentID      string
	Path         string
	RelativePath string
}

func WriteWorkflowOutput(output WorkflowOutput) ([]WrittenFile, error) {
	projectPath := strings.TrimSpace(output.ProjectPath)
	if projectPath == "" {
		return nil, fmt.Errorf("путь проекта пустой")
	}
	if output.CreatedAt.IsZero() {
		output.CreatedAt = time.Now()
	}

	runDir := filepath.Join(".zavod", "runs", safePathPart(output.WorkflowRunID))
	files := []WrittenFile{
		{
			Kind:         "task_spec",
			Title:        "Спека задачи",
			AgentID:      "manager",
			RelativePath: filepath.Join("docs", "task-spec.md"),
		},
		{
			Kind:         "workflow_step",
			Title:        "Task brief Люмен",
			AgentID:      "manager",
			RelativePath: filepath.Join(runDir, "01-manager-task-brief.md"),
		},
		{
			Kind:         "workflow_step",
			Title:        "Требования продакта",
			AgentID:      "product",
			RelativePath: filepath.Join(runDir, "02-product-requirements.md"),
		},
		{
			Kind:         "task_blueprint",
			Title:        "Task Blueprint",
			AgentID:      "architect",
			RelativePath: filepath.Join(runDir, "03-task-blueprint.md"),
		},
		{
			Kind:         "workflow_step",
			Title:        "Архитектурный план",
			AgentID:      "architect",
			RelativePath: filepath.Join(runDir, "04-architecture-plan.md"),
		},
		{
			Kind:         "developer_plan",
			Title:        "План разработки",
			AgentID:      "developer",
			RelativePath: filepath.Join(runDir, "05-developer-plan.md"),
		},
		{
			Kind:         "workflow_step",
			Title:        "Итог Люмен",
			AgentID:      "manager",
			RelativePath: filepath.Join(runDir, "06-manager-summary.md"),
		},
	}

	contents := []string{
		buildTaskSpec(output),
		wrapStep("Task brief Люмен", output.Intake),
		wrapStep("Требования продакта", output.Product),
		wrapStep("Task Blueprint", formatBlueprintForArtifact(output.Blueprint)),
		wrapStep("Архитектурный план", output.Architect),
		wrapStep("План разработки", output.Developer),
		wrapStep("Итог Люмен", output.Final),
	}

	written := make([]WrittenFile, 0, len(files))
	for index := range files {
		if strings.TrimSpace(contents[index]) == "" {
			continue
		}
		path, err := safeJoin(projectPath, files[index].RelativePath)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte(contents[index]), 0o644); err != nil {
			return nil, err
		}
		files[index].Path = path
		written = append(written, files[index])
	}
	return written, nil
}

func buildTaskSpec(output WorkflowOutput) string {
	var builder strings.Builder
	title := strings.TrimSpace(output.TaskTitle)
	if title == "" {
		title = "Задача AI-завода"
	}
	intake := parseIntake(output.Intake)
	specTitle := firstNonEmpty(title, intake.Summary, "Задача AI-завода")
	runDir := filepath.Join(".zavod", "runs", safePathPart(output.WorkflowRunID))

	builder.WriteString("# ")
	builder.WriteString(specTitle)
	builder.WriteString("\n\n")

	builder.WriteString("> Рабочая спецификация задачи. Это source of truth для агентов и человека; подробные ответы ролей сохранены отдельно в `")
	builder.WriteString(runDir)
	builder.WriteString("`.\n\n")

	builder.WriteString("## Метаданные\n\n")
	builder.WriteString("- Workflow: `")
	builder.WriteString(output.WorkflowRunID)
	builder.WriteString("`\n")
	builder.WriteString("- Сгенерировано: ")
	builder.WriteString(output.CreatedAt.Format(time.RFC3339))
	builder.WriteString("\n")
	builder.WriteString("- Формат: human-readable Markdown + отдельные служебные артефакты\n\n")

	appendSection(&builder, "Запрос пользователя", firstNonEmpty(intake.Summary, title))
	appendSection(&builder, "Цель", firstNonEmpty(intake.Goal, intake.Summary, title))
	appendSection(&builder, "Требования", firstNonEmpty(
		extractMarkdownSection(output.Product, "функциональные требования", "требования"),
		"Требования не были явно выделены. Используй запрос пользователя и цель как базовый контракт.",
	))
	appendSection(&builder, "Технический контракт", firstNonEmpty(
		formatBlueprintForSpec(output.Blueprint),
		extractMarkdownSection(output.Architect, "компоненты", "backend/api", "шаги реализации"),
		"Task Blueprint не был сформирован.",
	))
	appendSection(&builder, "Критерии готовности", firstNonEmpty(
		extractMarkdownSection(output.Product, "критерии готовности", "acceptance criteria"),
		defaultAcceptanceCriteria(),
	))
	appendSection(&builder, "Ограничения", firstNonEmpty(
		bullets(intake.Constraints),
		extractMarkdownSection(output.Product, "не входит", "нефункциональные требования"),
		"- Не менять файлы вне задачи.\n- Не добавлять скрытые ручные шаги без необходимости.",
	))
	appendSection(&builder, "Поддерживающие материалы", supportingArtifacts(runDir))
	return strings.TrimSpace(builder.String()) + "\n"
}

type intakeSpec struct {
	Summary     string   `json:"summary"`
	Goal        string   `json:"goal"`
	Constraints []string `json:"constraints"`
}

type blueprintSpec struct {
	Stack            string   `json:"stack"`
	Runtime          string   `json:"runtime"`
	ProjectType      string   `json:"project_type"`
	ScaffoldRequired bool     `json:"scaffold_required"`
	Entrypoints      []string `json:"entrypoints"`
	ExpectedFiles    []struct {
		Path    string `json:"path"`
		Action  string `json:"action"`
		Purpose string `json:"purpose"`
	} `json:"expected_files"`
	ForbiddenFiles []string `json:"forbidden_files"`
	Dependencies   struct {
		Policy string   `json:"policy"`
		Items  []string `json:"items"`
	} `json:"dependencies"`
	TestCommands []struct {
		Command    string `json:"command"`
		WorkingDir string `json:"working_dir"`
		Reason     string `json:"reason"`
	} `json:"test_commands"`
	Confidence string `json:"confidence"`
}

func parseIntake(content string) intakeSpec {
	var result intakeSpec
	trimmed := stripFence(content)
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		result.Summary = truncateText(strings.TrimSpace(content), 800)
	}
	return result
}

func formatBlueprintForSpec(content string) string {
	parsed, ok := parseBlueprint(content)
	if !ok {
		return ""
	}
	var builder strings.Builder
	writeKeyValue(&builder, "Стек", parsed.Stack)
	writeKeyValue(&builder, "Runtime", parsed.Runtime)
	writeKeyValue(&builder, "Тип проекта", parsed.ProjectType)
	if parsed.ScaffoldRequired {
		writeKeyValue(&builder, "Scaffold", "нужен")
	} else {
		writeKeyValue(&builder, "Scaffold", "не нужен")
	}
	if len(parsed.Entrypoints) > 0 {
		appendSubsection(&builder, "Entrypoints", bullets(parsed.Entrypoints))
	}
	if len(parsed.ExpectedFiles) > 0 {
		builder.WriteString("### Ожидаемые файлы\n\n")
		for _, file := range parsed.ExpectedFiles {
			builder.WriteString("- `")
			builder.WriteString(strings.TrimSpace(file.Path))
			builder.WriteString("`")
			if strings.TrimSpace(file.Action) != "" {
				builder.WriteString(" · ")
				builder.WriteString(strings.TrimSpace(file.Action))
			}
			if strings.TrimSpace(file.Purpose) != "" {
				builder.WriteString(" — ")
				builder.WriteString(strings.TrimSpace(file.Purpose))
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	if len(parsed.ForbiddenFiles) > 0 {
		appendSubsection(&builder, "Запрещено менять", codeBullets(parsed.ForbiddenFiles))
	}
	if len(parsed.TestCommands) > 0 {
		builder.WriteString("### Проверки\n\n")
		for _, command := range parsed.TestCommands {
			builder.WriteString("- `")
			builder.WriteString(strings.TrimSpace(command.Command))
			builder.WriteString("`")
			if strings.TrimSpace(command.WorkingDir) != "" && strings.TrimSpace(command.WorkingDir) != "." {
				builder.WriteString(" в `")
				builder.WriteString(strings.TrimSpace(command.WorkingDir))
				builder.WriteString("`")
			}
			if strings.TrimSpace(command.Reason) != "" {
				builder.WriteString(" — ")
				builder.WriteString(strings.TrimSpace(command.Reason))
			}
			builder.WriteString("\n")
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func formatBlueprintForArtifact(content string) string {
	formatted := formatBlueprintForSpec(content)
	if formatted != "" {
		return formatted
	}
	return strings.TrimSpace(content)
}

func parseBlueprint(content string) (blueprintSpec, bool) {
	var result blueprintSpec
	trimmed := stripFence(content)
	if trimmed == "" {
		return result, false
	}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return result, false
	}
	return result, true
}

func extractMarkdownSection(content string, names ...string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	start := -1
	startLevel := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		level := headingLevel(trimmed)
		title := normalizeHeading(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
		for _, name := range names {
			if strings.Contains(title, normalizeHeading(name)) {
				start = i + 1
				startLevel = level
				break
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "#") && headingLevel(trimmed) <= startLevel {
			end = i
			break
		}
	}
	return truncateText(strings.TrimSpace(strings.Join(lines[start:end], "\n")), 1800)
}

func headingLevel(line string) int {
	count := 0
	for _, r := range line {
		if r != '#' {
			break
		}
		count++
	}
	return count
}

func normalizeHeading(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func stripFence(content string) string {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func bullets(items []string) string {
	var builder strings.Builder
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(item)
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func codeBullets(items []string) string {
	var builder strings.Builder
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		builder.WriteString("- `")
		builder.WriteString(item)
		builder.WriteString("`\n")
	}
	return strings.TrimSpace(builder.String())
}

func writeKeyValue(builder *strings.Builder, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	builder.WriteString("- ")
	builder.WriteString(key)
	builder.WriteString(": ")
	builder.WriteString(value)
	builder.WriteString("\n")
}

func defaultAcceptanceCriteria() string {
	return "- Реализация соответствует запросу пользователя и требованиям.\n- Изменены только файлы, относящиеся к задаче.\n- Релевантные проверки проходят.\n- Ревьюер принимает результат или явно указывает причину остановки."
}

func supportingArtifacts(runDir string) string {
	items := []string{
		filepath.Join(runDir, "01-manager-task-brief.md") + " — исходная постановка Люмен",
		filepath.Join(runDir, "02-product-requirements.md") + " — требования продакта",
		filepath.Join(runDir, "03-task-blueprint.md") + " — машинный контракт задачи",
		filepath.Join(runDir, "04-architecture-plan.md") + " — архитектурный план",
		filepath.Join(runDir, "05-developer-plan.md") + " — план разработки",
		filepath.Join(runDir, "06-manager-summary.md") + " — итог выполнения",
	}
	var builder strings.Builder
	for _, item := range items {
		parts := strings.SplitN(item, " — ", 2)
		builder.WriteString("- `")
		builder.WriteString(parts[0])
		builder.WriteString("`")
		if len(parts) == 2 {
			builder.WriteString(" — ")
			builder.WriteString(parts[1])
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func truncateText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func wrapStep(title string, content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return "# " + strings.TrimSpace(title) + "\n\n" + content + "\n"
}

func appendSection(builder *strings.Builder, title string, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	builder.WriteString("## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString(content)
	builder.WriteString("\n\n")
}

func appendSubsection(builder *strings.Builder, title string, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	builder.WriteString("### ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	builder.WriteString(content)
	builder.WriteString("\n\n")
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "run"
	}
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteRune('-')
	}
	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "run"
	}
	return result
}

func safeJoin(root string, relativePath string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(rootAbs, relativePath)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("путь артефакта выходит за каталог проекта: %s", relativePath)
	}
	return targetAbs, nil
}
