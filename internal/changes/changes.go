package changes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ActionCreate  = "create"
	ActionReplace = "replace"

	StatusPending    = "pending"
	StatusApplied    = "applied"
	StatusFailed     = "failed"
	StatusRolledBack = "rolled_back"

	MaxContentBytes = 200 * 1024
)

type ProposedChange struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	TaskID        string `json:"taskId"`
	WorkflowRunID string `json:"workflowRunId"`
	AgentID       string `json:"agentId"`
	FilePath      string `json:"filePath"`
	Action        string `json:"action"`
	Content       string `json:"content"`
	Reason        string `json:"reason"`
	Status        string `json:"status"`
	Error         string `json:"error"`
	BackupPath    string `json:"backupPath"`
	BeforeContent string `json:"beforeContent"`
	AfterContent  string `json:"afterContent"`
	DiffText      string `json:"diffText"`
	CreatedAt     string `json:"createdAt"`
	AppliedAt     string `json:"appliedAt"`
}

type Draft struct {
	FilePath string `json:"file_path"`
	Action   string `json:"action"`
	Content  string `json:"content"`
	Reason   string `json:"reason"`
}

type ApplyResult struct {
	BackupPath    string
	BeforeContent string
	AfterContent  string
	DiffText      string
}

type RollbackResult struct {
	BeforeContent string
	AfterContent  string
	DiffText      string
}

func ExtractDrafts(text string) []Draft {
	drafts, _ := ExtractDraftsWithError(text)
	return drafts
}

func ExtractDraftsWithError(text string) ([]Draft, error) {
	hasProposedSection := strings.Contains(strings.ToLower(text), "## proposed changes")
	candidates := jsonCandidates(text)
	if len(candidates) == 0 {
		if hasProposedSection {
			return nil, fmt.Errorf("секция ## Proposed changes не содержит JSON-массив")
		}
		return nil, nil
	}

	var lastErr error
	for _, candidate := range candidates {
		var drafts []Draft
		if err := json.Unmarshal([]byte(candidate), &drafts); err != nil {
			lastErr = err
			continue
		}
		normalized := normalizeDrafts(drafts)
		if len(drafts) > 0 && len(normalized) == 0 {
			lastErr = fmt.Errorf("секция ## Proposed changes не содержит валидных file_path/action/content")
			continue
		}
		return normalized, nil
	}
	if hasProposedSection {
		if lastErr != nil {
			return nil, fmt.Errorf("не удалось разобрать ## Proposed changes как JSON-массив: %w", lastErr)
		}
		return nil, fmt.Errorf("не удалось разобрать ## Proposed changes как JSON-массив")
	}
	return nil, nil
}

func Apply(projectPath string, change ProposedChange) (ApplyResult, error) {
	relativePath, err := ValidateRelativePath(change.FilePath)
	if err != nil {
		return ApplyResult{}, err
	}
	if len([]byte(change.Content)) > MaxContentBytes {
		return ApplyResult{}, fmt.Errorf("файл %s больше лимита %d байт", relativePath, MaxContentBytes)
	}

	targetPath, err := safeJoin(projectPath, relativePath)
	if err != nil {
		return ApplyResult{}, err
	}

	switch change.Action {
	case ActionCreate:
		if existing, err := os.ReadFile(targetPath); err == nil {
			beforeContent := string(existing)
			if beforeContent == change.Content {
				return ApplyResult{
					BeforeContent: beforeContent,
					AfterContent:  change.Content,
					DiffText:      "",
				}, nil
			}
			return ApplyResult{}, fmt.Errorf("файл уже существует: %s; для изменения существующего файла нужен action=replace", relativePath)
		} else if !os.IsNotExist(err) {
			return ApplyResult{}, err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return ApplyResult{}, err
		}
		if err := os.WriteFile(targetPath, []byte(change.Content), 0o644); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{
			AfterContent: change.Content,
			DiffText:     GenerateUnifiedDiff(relativePath, "", change.Content),
		}, nil
	case ActionReplace:
		existing, err := os.ReadFile(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return ApplyResult{}, fmt.Errorf("для replace файл должен существовать: %s", relativePath)
			}
			return ApplyResult{}, err
		}
		beforeContent := string(existing)
		backupRelativePath := filepath.Join(".zavod", "backups", safePathPart(change.ID), relativePath)
		backupPath, err := safeJoin(projectPath, backupRelativePath)
		if err != nil {
			return ApplyResult{}, err
		}
		if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
			return ApplyResult{}, err
		}
		if err := os.WriteFile(backupPath, existing, 0o644); err != nil {
			return ApplyResult{}, err
		}
		if err := os.WriteFile(targetPath, []byte(change.Content), 0o644); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{
			BackupPath:    backupRelativePath,
			BeforeContent: beforeContent,
			AfterContent:  change.Content,
			DiffText:      GenerateUnifiedDiff(relativePath, beforeContent, change.Content),
		}, nil
	default:
		return ApplyResult{}, fmt.Errorf("неподдерживаемое действие: %s", change.Action)
	}
}

func Rollback(projectPath string, change ProposedChange) (RollbackResult, error) {
	relativePath, err := ValidateRelativePath(change.FilePath)
	if err != nil {
		return RollbackResult{}, err
	}
	if change.Status != StatusApplied {
		return RollbackResult{}, fmt.Errorf("можно откатить только примененное изменение")
	}
	targetPath, err := safeJoin(projectPath, relativePath)
	if err != nil {
		return RollbackResult{}, err
	}
	current, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) && change.AfterContent == "" {
			return RollbackResult{}, nil
		}
		return RollbackResult{}, err
	}
	currentContent := string(current)
	if currentContent != change.AfterContent {
		return RollbackResult{}, fmt.Errorf("файл изменился после применения; автоматический rollback остановлен: %s", relativePath)
	}
	if change.Action == ActionCreate && change.BeforeContent == "" {
		if err := os.Remove(targetPath); err != nil {
			return RollbackResult{}, err
		}
		return RollbackResult{
			BeforeContent: currentContent,
			AfterContent:  "",
			DiffText:      GenerateUnifiedDiff(relativePath, currentContent, ""),
		}, nil
	}
	if err := os.WriteFile(targetPath, []byte(change.BeforeContent), 0o644); err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{
		BeforeContent: currentContent,
		AfterContent:  change.BeforeContent,
		DiffText:      GenerateUnifiedDiff(relativePath, currentContent, change.BeforeContent),
	}, nil
}

func GenerateUnifiedDiff(filePath string, before string, after string) string {
	if before == after {
		return ""
	}

	beforeLines := splitLines(before)
	afterLines := splitLines(after)
	ops := diffOps(beforeLines, afterLines)

	var builder strings.Builder
	if before == "" {
		builder.WriteString("--- /dev/null\n")
	} else {
		builder.WriteString("--- a/")
		builder.WriteString(filePath)
		builder.WriteString("\n")
	}
	builder.WriteString("+++ b/")
	builder.WriteString(filePath)
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("@@ -1,%d +1,%d @@\n", len(beforeLines), len(afterLines)))
	for _, op := range ops {
		builder.WriteString(op.prefix)
		builder.WriteString(op.value)
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func ValidateRelativePath(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("путь файла пустой")
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("абсолютные пути запрещены: %s", value)
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("выход за каталог проекта запрещен: %s", value)
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	if len(parts) > 0 {
		switch parts[0] {
		case ".git", ".zavod":
			return "", fmt.Errorf("запись в %s запрещена", parts[0])
		}
	}
	if filepath.Base(clean) == "zavod.db" {
		return "", fmt.Errorf("запись в zavod.db запрещена")
	}
	return clean, nil
}

func AppliedAt() string {
	return time.Now().UTC().Format(time.RFC3339)
}

type lineOp struct {
	prefix string
	value  string
}

func diffOps(before []string, after []string) []lineOp {
	lcs := make([][]int, len(before)+1)
	for index := range lcs {
		lcs[index] = make([]int, len(after)+1)
	}
	for left := len(before) - 1; left >= 0; left-- {
		for right := len(after) - 1; right >= 0; right-- {
			if before[left] == after[right] {
				lcs[left][right] = lcs[left+1][right+1] + 1
				continue
			}
			if lcs[left+1][right] >= lcs[left][right+1] {
				lcs[left][right] = lcs[left+1][right]
			} else {
				lcs[left][right] = lcs[left][right+1]
			}
		}
	}

	ops := make([]lineOp, 0, len(before)+len(after))
	left := 0
	right := 0
	for left < len(before) && right < len(after) {
		if before[left] == after[right] {
			ops = append(ops, lineOp{prefix: " ", value: before[left]})
			left++
			right++
			continue
		}
		if lcs[left+1][right] >= lcs[left][right+1] {
			ops = append(ops, lineOp{prefix: "-", value: before[left]})
			left++
			continue
		}
		ops = append(ops, lineOp{prefix: "+", value: after[right]})
		right++
	}
	for left < len(before) {
		ops = append(ops, lineOp{prefix: "-", value: before[left]})
		left++
	}
	for right < len(after) {
		ops = append(ops, lineOp{prefix: "+", value: after[right]})
		right++
	}
	return ops
}

func splitLines(value string) []string {
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func normalizeDrafts(drafts []Draft) []Draft {
	out := make([]Draft, 0, len(drafts))
	for _, draft := range drafts {
		draft.FilePath = strings.TrimSpace(draft.FilePath)
		draft.Action = strings.ToLower(strings.TrimSpace(draft.Action))
		draft.Reason = strings.TrimSpace(draft.Reason)
		draft.Content = NormalizeFileContent(draft.FilePath, draft.Content)
		if draft.FilePath == "" || draft.Content == "" {
			continue
		}
		if draft.Action != ActionCreate && draft.Action != ActionReplace {
			continue
		}
		if _, err := ValidateRelativePath(draft.FilePath); err != nil {
			continue
		}
		out = append(out, draft)
	}
	return out
}

func NormalizeFileContent(filePath string, content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if diffContent, ok := contentFromUnifiedDiff(filePath, content); ok {
		content = diffContent
	}
	if looksDoubleEscaped(content) {
		content = unescapeGeneratedContent(content)
	}
	return content
}

func looksDoubleEscaped(content string) bool {
	escapedNewlines := strings.Count(content, `\n`)
	if escapedNewlines < 2 {
		return false
	}
	actualNewlines := strings.Count(content, "\n")
	return actualNewlines <= 1 || escapedNewlines > actualNewlines*4
}

func unescapeGeneratedContent(content string) string {
	replacer := strings.NewReplacer(
		`\r\n`, "\n",
		`\n`, "\n",
		`\t`, "\t",
		`\"`, `"`,
	)
	return replacer.Replace(content)
}

func contentFromUnifiedDiff(filePath string, content string) (string, bool) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "--- ") || !strings.HasPrefix(lines[1], "+++ ") {
		return "", false
	}
	target := strings.TrimPrefix(strings.TrimSpace(lines[1]), "+++ ")
	target = strings.TrimPrefix(target, "b/")
	target = strings.Trim(target, `"`)
	if filePath != "" && filepath.ToSlash(target) != filepath.ToSlash(strings.Trim(filePath, "/")) {
		baseTarget := filepath.Base(target)
		baseFile := filepath.Base(filePath)
		if baseTarget != "" && baseFile != "" && baseTarget != baseFile {
			return "", false
		}
	}

	var builder strings.Builder
	inHunk := false
	changed := false
	for _, line := range lines[2:] {
		switch {
		case strings.HasPrefix(line, "@@"):
			inHunk = true
		case !inHunk:
			continue
		case strings.HasPrefix(line, "+++"):
			continue
		case strings.HasPrefix(line, "+"):
			writeDiffPayloadLine(&builder, line[1:])
			changed = true
		case strings.HasPrefix(line, " "):
			writeDiffPayloadLine(&builder, line[1:])
		case strings.HasPrefix(line, "-"):
			changed = true
		case line == `\ No newline at end of file`:
			continue
		default:
			writeDiffPayloadLine(&builder, line)
		}
	}
	return builder.String(), changed
}

func writeDiffPayloadLine(builder *strings.Builder, value string) {
	builder.WriteString(value)
	if !strings.Contains(value, `\n`) && !strings.HasSuffix(value, "\n") {
		builder.WriteString("\n")
	}
}

func jsonCandidates(text string) []string {
	var candidates []string
	lower := strings.ToLower(text)
	if sectionIndex := strings.LastIndex(lower, "## proposed changes"); sectionIndex >= 0 {
		section := text[sectionIndex:]
		if candidate := bracketCandidate(section); candidate != "" {
			candidates = append(candidates, candidate)
		}
	}

	fencePattern := regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)```")
	for _, match := range fencePattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 {
			candidates = append(candidates, strings.TrimSpace(match[1]))
		}
	}

	if candidate := bracketCandidate(text); candidate != "" {
		candidates = append(candidates, candidate)
	}
	return candidates
}

func bracketCandidate(text string) string {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(text[start : end+1])
}

func safeJoin(root string, relativePath string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, relativePath))
	if err != nil {
		return "", err
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("путь выходит за каталог проекта: %s", relativePath)
	}
	return targetAbs, nil
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "change"
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
		return "change"
	}
	return result
}
