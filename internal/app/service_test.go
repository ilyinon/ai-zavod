package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/agents"
	"zavod_ai/internal/artifacts"
	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/config"
	"zavod_ai/internal/ctf"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/projectmemory"
	"zavod_ai/internal/reviews"
	"zavod_ai/internal/router"
	"zavod_ai/internal/store"
	"zavod_ai/internal/taskspec"
	"zavod_ai/internal/webresearch"
	zw "zavod_ai/internal/workflow"
)

type staticProvider struct {
	content string
}

func (p staticProvider) Generate(ctx context.Context, req llm.Request) (*llm.Response, error) {
	return &llm.Response{Content: p.content}, nil
}

func (p staticProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.Event, error) {
	events := make(chan llm.Event, 1)
	events <- llm.Event{Delta: p.content, Done: true}
	close(events)
	return events, nil
}

func (p staticProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{}
}

func TestLooksLikeRepetitionLoopDetectsRepeatedAgentPrefix(t *testing.T) {
	text := strings.Repeat("Агент manager: ", repetitionLoopMinRepeats)
	if !looksLikeRepetitionLoop(text) {
		t.Fatal("expected repeated agent prefix to be detected")
	}
}

func TestLooksLikeRepetitionLoopAllowsNormalAnswer(t *testing.T) {
	text := "Поняла задачу. Следующий шаг: уточнить цель, входные данные и критерии готовности."
	if looksLikeRepetitionLoop(text) {
		t.Fatal("normal answer was detected as repetition loop")
	}
}

func TestAgentCapabilityContractIncludesBoundaries(t *testing.T) {
	contract := agentCapabilityContract(agentgroups.Profile{
		Name:         "ИБ-специалист",
		RoleKey:      "security",
		Capabilities: []string{"scope review"},
		AllowedTools: []string{"web_search"},
		ReadPaths:    []string{"docs/**"},
		WritePaths:   []string{"docs/security/**"},
		HandoffRules: []string{"Передать ревьюеру после анализа"},
	})
	for _, expected := range []string{"# Capabilities Contract", "scope review", "web_search", "docs/**", "docs/security/**", "Передать ревьюеру"} {
		if !strings.Contains(contract, expected) {
			t.Fatalf("expected %q in capability contract:\n%s", expected, contract)
		}
	}
}

func TestSaveResearchNotesCreatesProjectArtifact(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	currentProject, err := db.CreateProject(ctx, "Research project", tmp)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := db.CreateTask(ctx, currentProject.ID, "Research task")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	run, err := db.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	service := &Service{store: db}

	relativePath, err := service.saveResearchNotes(ctx, currentProject, task, run.ID, "# Research Notes\n\n- source checked")
	if err != nil {
		t.Fatalf("save notes: %v", err)
	}
	if relativePath != filepath.Join("docs", "research-notes.md") {
		t.Fatalf("relative path = %q", relativePath)
	}
	data, err := os.ReadFile(filepath.Join(tmp, relativePath))
	if err != nil {
		t.Fatalf("read notes: %v", err)
	}
	if !strings.Contains(string(data), "source checked") {
		t.Fatalf("expected notes content, got %q", string(data))
	}
	items, err := db.ListArtifacts(ctx, currentProject.ID, 10)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	if len(items) != 1 || items[0].Kind != "research_notes" || items[0].AgentID != agents.ResearcherID {
		t.Fatalf("expected research notes artifact, got %#v", items)
	}
}

func TestFallbackWorkflowPlanUsesResearchSquadSteps(t *testing.T) {
	currentProject := project.Project{ID: "project_1", Name: "Research project"}
	task := chat.Task{ID: "task_1", Title: "Research task"}
	_, steps := fallbackWorkflowPlan(currentProject, task, "run_1", "загугли актуальные цены", router.IntentResearchTask)
	want := []struct {
		key   string
		agent string
	}{
		{zw.StepWebResearch, agents.ResearcherID},
		{zw.StepResearchSourceReview, agents.SourceReviewID},
		{zw.StepResearchSynthesis, agents.AnalystID},
		{zw.StepResearchNotes, agents.ResearcherID},
		{zw.StepManagerFinal, agents.ManagerID},
	}
	if len(steps) != len(want) {
		t.Fatalf("expected research steps, got %#v", steps)
	}
	for index := range want {
		if steps[index].StepKey != want[index].key || steps[index].AgentID != want[index].agent {
			t.Fatalf("step %d = %#v, want %#v", index, steps[index], want[index])
		}
	}
}

func TestWebResearchFallbackAnswerUsesSourcePanel(t *testing.T) {
	answer := webResearchFallbackAnswer([]webresearch.Source{
		{Title: "Example", URL: "https://example.com", Snippet: "snippet"},
	})
	if strings.Contains(answer, "## Источники") || strings.Contains(answer, "https://example.com") {
		t.Fatalf("fallback answer should not inline source list, got %q", answer)
	}
	if !strings.Contains(answer, "Источники показаны отдельным блоком") {
		t.Fatalf("fallback answer should point to source UI block, got %q", answer)
	}
}

func TestLooksLikeRawJSONAnswer(t *testing.T) {
	if !looksLikeRawJSONAnswer(`{"summary":"raw","sources":[]}`) {
		t.Fatal("expected raw json answer to be detected")
	}
	if !looksLikeRawJSONAnswer("```json\n{\"summary\":\"raw\"}\n```") {
		t.Fatal("expected fenced raw json answer to be detected")
	}
	if looksLikeRawJSONAnswer("## Коротко\n\nОтвет без JSON.") {
		t.Fatal("markdown answer should not be detected as raw json")
	}
}

func TestManagerNeedsClarificationFromJSON(t *testing.T) {
	output := `{"summary":"мало данных","needs_clarification":true}`
	if !managerNeedsClarification(output) {
		t.Fatal("expected clarification flag from json")
	}
}

func TestFormatClarificationMessageIsHumanReadable(t *testing.T) {
	message := formatClarificationMessage(managerIntakeResult{
		Summary:       "нужно разработать проверку локальной LLM",
		Goal:          "понять, доступна ли модель",
		OpenQuestions: []string{"Какой endpoint проверять?", "Какие критерии успеха?"},
	})

	if strings.Contains(message, `"summary"`) || strings.Contains(message, "needs_clarification") {
		t.Fatalf("expected human-readable message, got %q", message)
	}
	if !strings.Contains(message, "Поняла задачу") || !strings.Contains(message, "Что нужно уточнить") {
		t.Fatalf("expected clarification sections, got %q", message)
	}
}

func TestFilterRelevantTestSuggestionsKeepsOnlyChangedStack(t *testing.T) {
	suggestions := []checks.Suggestion{
		{Command: "go test ./...", Reason: "Go"},
		{Command: "python3 check.py", Reason: "Python"},
	}
	applied := []changes.ProposedChange{
		{FilePath: "check.go", Status: changes.StatusApplied},
		{FilePath: "main.go", Status: changes.StatusApplied},
	}

	got := filterRelevantTestSuggestions(suggestions, applied)
	if len(got) != 1 || got[0].Command != "go test ./..." {
		t.Fatalf("expected only Go test suggestion, got %#v", got)
	}
}

func TestLatestTestRunsByCommandKeepsNewestAttempt(t *testing.T) {
	items := []checks.TestRun{
		{Command: "go test ./...", Status: checks.StatusPassed, CreatedAt: "2026-08-30T12:02:00Z"},
		{Command: "go test ./...", Status: checks.StatusFailed, CreatedAt: "2026-08-30T12:01:00Z"},
		{Command: "npm run build", WorkingDir: "frontend", Status: checks.StatusPassed, CreatedAt: "2026-08-30T12:00:00Z"},
	}

	got := latestTestRunsByCommand(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 latest command results, got %#v", got)
	}
	if got[0].Command != "go test ./..." || got[0].Status != checks.StatusPassed {
		t.Fatalf("expected latest Go result to be passed, got %#v", got[0])
	}
}

func TestWantsSavedTaskSpec(t *testing.T) {
	if !wantsSavedTaskSpec("выведи спеку проекта по которой ты написал этот код") {
		t.Fatal("expected saved task spec request to be detected")
	}
	if wantsSavedTaskSpec("распиши спеку для следующего шага") {
		t.Fatal("new spec authoring request should not be treated as saved spec output")
	}
}

func TestSavedTaskSpecAnswerUsesSpecStore(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	projectPath := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateProject(ctx, "Spec project", projectPath)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := db.CreateTask(ctx, project.ID, "Сделай CLI")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := db.UpsertTaskSpec(ctx, taskspec.Spec{
		ProjectID:          project.ID,
		TaskID:             task.ID,
		WorkflowRunID:      "run_1",
		UserRequest:        "Сделай CLI на Go",
		Goal:               "Собрать CLI с параметром адреса сайта",
		Requirements:       []string{"Go 1.25+", "Принимать domain и URL"},
		AcceptanceCriteria: []string{"`go test ./...` проходит успешно"},
		Decisions:          []string{"Использовать стандартную библиотеку Go"},
		AcceptedAnswers: []taskspec.AcceptedAnswer{{
			QuestionID: "q1",
			Question:   "Какой runtime?",
			Answer:     "Go 1.25+",
		}},
		Status: taskspec.StatusDone,
		Source: "test",
	}); err != nil {
		t.Fatalf("upsert spec: %v", err)
	}
	service := &Service{store: db, agentStatuses: map[string]agents.Status{}}

	got := service.savedTaskSpecAnswer(ctx, project, &task)
	for _, want := range []string{"## Спека задачи", "Собрать CLI", "Go 1.25+", "Принимать domain", "Какой runtime?", "Go 1.25+"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in saved spec answer:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Источник fallback") {
		t.Fatalf("expected direct spec store answer, got fallback:\n%s", got)
	}
}

func TestProjectMemoryAnswerUsesMemoryStore(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	projectPath := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateProject(ctx, "Memory project", projectPath)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := db.UpsertProjectMemory(ctx, projectmemory.Memory{
		ProjectID:     project.ID,
		Architecture:  "Wails app with Go backend and React frontend",
		Stack:         "go, frontend",
		Runtime:       "Go 1.25+",
		BuildCommands: []string{"make build"},
		TestCommands:  []string{"go test ./..."},
		StyleGuide:    []string{"gofmt"},
		Decisions:     []string{"Не отправлять память проекта в модель без явного разрешения"},
		Environment:   []string{"DMG собирается через scripts/package-dmg.sh"},
	}); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	service := &Service{store: db}

	got := service.projectMemoryAnswer(ctx, project)
	for _, want := range []string{"## Память проекта", "Wails app", "Go 1.25+", "make build", "Не отправлять память"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in project memory answer:\n%s", want, got)
		}
	}
}

func TestProjectMemoryFromFilesystemDetectsGoAndMakefile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/app\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "Makefile"), []byte("build:\n\tgo build ./...\n\ntest:\n\tgo test ./...\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := projectMemoryFromFilesystem(projectmemoryTestProject("project_1", tmp))
	if got.Stack != "go" || got.Runtime != "Go 1.25+" {
		t.Fatalf("expected Go memory, got %#v", got)
	}
	if !containsTestString(got.BuildCommands, "make build") || !containsTestString(got.TestCommands, "make test") {
		t.Fatalf("expected Makefile commands, got build=%#v test=%#v", got.BuildCommands, got.TestCommands)
	}
}

func projectmemoryTestProject(id string, path string) project.Project {
	return project.Project{
		ID:   id,
		Name: "Memory project",
		Path: path,
	}
}

func containsTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasDraftPathForTest(items []changes.Draft, path string) bool {
	for _, item := range items {
		if item.FilePath == path {
			return true
		}
	}
	return false
}

func TestDeterministicBlockedFinalIncludesReviewDetails(t *testing.T) {
	message := deterministicBlockedFinal(autopilotResult{
		AppliedFiles:   2,
		TestsPassed:    1,
		ReviewStatus:   reviews.StatusNeedsWork,
		ReviewReturnTo: reviews.ReturnToDeveloper,
		ReviewSummary:  "не выполнено требование по CLI параметру",
		ReviewRequired: []string{"Добавить обработку аргумента адреса сайта"},
		ReviewFindings: []reviews.Finding{
			{FilePath: "main.go", Message: "адрес не передается в CheckSite", Suggestion: "прочитать os.Args[1]"},
		},
		Iterations:  3,
		Blocked:     true,
		BlockReason: "исчерпан лимит repair-итераций: последнее ревью все еще требует доработку",
	})

	for _, want := range []string{"Причина ревью", "Что не принято ревьюером", "Добавить обработку аргумента", "main.go"} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in blocked final message, got %q", want, message)
		}
	}
}

func TestPythonRequirementsContentFromBlueprintDependencies(t *testing.T) {
	got := pythonRequirementsContent([]string{"python-telegram-bot", "requests", "python-telegram-bot"})
	if got != "python-telegram-bot\nrequests\n" {
		t.Fatalf("unexpected requirements content: %q", got)
	}
}

func TestEnsureBlueprintRequiredDraftsOverwritesRequirementsFromDependencies(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	projectPath := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	project, err := db.CreateProject(ctx, "Python project", projectPath)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := db.CreateTask(ctx, project.ID, "Python task")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	run, err := db.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := db.CreateTaskBlueprint(ctx, blueprint.Blueprint{
		ProjectID:     project.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		Stack:         blueprint.StackPython,
		ExpectedFiles: []blueprint.ExpectedFile{{Path: "requirements.txt"}},
		Dependencies: blueprint.DependencyPolicy{
			Policy: "external",
			Items:  []string{"python-telegram-bot", "requests"},
		},
		RawJSON: "{}",
	}); err != nil {
		t.Fatalf("create blueprint: %v", err)
	}
	service := &Service{store: db}

	got := service.ensureBlueprintRequiredDrafts(ctx, project, run.ID, []changes.Draft{{
		FilePath: "requirements.txt",
		Action:   changes.ActionCreate,
		Content:  "# wrong\n",
	}})
	requirements := ""
	for _, draft := range got {
		if draft.FilePath == "requirements.txt" {
			requirements = draft.Content
		}
	}
	if requirements != "python-telegram-bot\nrequests\n" {
		t.Fatalf("expected enforced requirements, got %#v", got)
	}
	for _, path := range []string{".gitignore", "Makefile", "README.md", ".github/workflows/ci.yml"} {
		if !hasDraftPathForTest(got, path) {
			t.Fatalf("expected dev workspace draft %s, got %#v", path, got)
		}
	}
}

func TestEnsureGoModVersionUsesGo125(t *testing.T) {
	got := ensureGoModVersion("module example.com/app\n\ngo 1.21\n")
	if !strings.Contains(got, "go 1.25") || strings.Contains(got, "go 1.21") {
		t.Fatalf("expected go 1.25 directive, got %q", got)
	}
	got = ensureGoModVersion("module example.com/app\n\ngo 1.26\n")
	if !strings.Contains(got, "go 1.26") || strings.Contains(got, "go 1.25") {
		t.Fatalf("expected newer go directive to stay unchanged, got %q", got)
	}
}

func TestRunReviewNormalizesFixableBlockedToNeedsWork(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	projectPath := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	currentProject, err := db.CreateProject(ctx, "Review project", projectPath)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := db.CreateTask(ctx, currentProject.ID, "Review task")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	run, err := db.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	service := &Service{store: db, agentStatuses: map[string]agents.Status{}}
	model := llm.ModelConfig{ID: "qwen-remote", Name: "Qwen", ModelName: "qwen"}
	provider := staticProvider{content: `{
		"status": "blocked",
		"summary": "Синтаксическая ошибка в check.go, go test ./... падает",
		"return_to": "user",
		"blocking_reason": "",
		"findings": [],
		"required_changes": ["исправить синтаксис"],
		"recommended_next_step": "вернуть разработчику"
	}`}

	parsed, err := service.runReviewNow(ctx, currentProject, task, &run, provider, model, 1)
	if err != nil {
		t.Fatalf("run review: %v", err)
	}
	if parsed.Status != reviews.StatusNeedsWork || parsed.ReturnTo != reviews.ReturnToDeveloper {
		t.Fatalf("expected normalized developer repair, got %#v", parsed)
	}
	reviewRuns, err := db.ListReviewRuns(ctx, currentProject.ID, run.ID, 10)
	if err != nil {
		t.Fatalf("list reviews: %v", err)
	}
	if len(reviewRuns) != 1 || reviewRuns[0].Status != reviews.StatusNeedsWork || reviewRuns[0].ReturnTo != reviews.ReturnToDeveloper {
		t.Fatalf("expected stored normalized review, got %#v", reviewRuns)
	}
}

func TestRunReviewGateRejectsAcceptedReviewWithSecurityFinding(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	projectPath := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	currentProject, err := db.CreateProject(ctx, "Security review project", projectPath)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := db.CreateTask(ctx, currentProject.ID, "Security review task")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	run, err := db.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	if _, err := db.UpsertTaskSpec(ctx, taskspec.Spec{
		ProjectID:          currentProject.ID,
		TaskID:             task.ID,
		WorkflowRunID:      run.ID,
		Goal:               "Создать Telegram bot",
		Requirements:       []string{"Токен должен передаваться через env"},
		AcceptanceCriteria: []string{"Секретов в коде нет"},
		Status:             taskspec.StatusActive,
	}); err != nil {
		t.Fatalf("task spec: %v", err)
	}
	if _, err := db.CreateTaskBlueprint(ctx, blueprint.Blueprint{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		Stack:         blueprint.StackPython,
		ExpectedFiles: []blueprint.ExpectedFile{{Path: "bot.py", Action: changes.ActionCreate}},
		RawJSON:       "{}",
	}); err != nil {
		t.Fatalf("blueprint: %v", err)
	}
	after := "BOT_TOKEN=\"123\"\n"
	if _, err := db.CreateProposedChange(ctx, changes.ProposedChange{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		FilePath:      "bot.py",
		Action:        changes.ActionCreate,
		Status:        changes.StatusApplied,
		AfterContent:  after,
		DiffText:      changes.GenerateUnifiedDiff("bot.py", "", after),
	}); err != nil {
		t.Fatalf("change: %v", err)
	}
	service := &Service{store: db, agentStatuses: map[string]agents.Status{}}
	model := llm.ModelConfig{ID: "qwen-remote", Name: "Qwen", ModelName: "qwen"}
	provider := staticProvider{content: `{"status":"accepted","summary":"ок","findings":[],"required_changes":[]}`}

	parsed, err := service.runReviewNow(ctx, currentProject, task, &run, provider, model, 1)
	if err != nil {
		t.Fatalf("run review: %v", err)
	}
	if parsed.Status != reviews.StatusNeedsWork || parsed.ReturnTo != reviews.ReturnToDeveloper {
		t.Fatalf("expected gate to reject accepted review, got %#v", parsed)
	}
	if len(parsed.Findings) == 0 || parsed.Findings[0].Category == "" {
		t.Fatalf("expected categorized gate finding, got %#v", parsed.Findings)
	}
	reviewRuns, err := db.ListReviewRuns(ctx, currentProject.ID, run.ID, 10)
	if err != nil {
		t.Fatalf("list review runs: %v", err)
	}
	if len(reviewRuns) != 1 || reviewRuns[0].Status != reviews.StatusNeedsWork || reviewRuns[0].ReturnTo != reviews.ReturnToDeveloper {
		t.Fatalf("expected stored enforced review, got %#v", reviewRuns)
	}
}

func TestRollbackWorkflowChangesRestoresAppliedFiles(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	projectPath := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	currentProject, err := db.CreateProject(ctx, "Rollback project", projectPath)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := db.CreateTask(ctx, currentProject.ID, "Rollback task")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	run, err := db.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	before := "package main\n"
	after := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(projectPath, "main.go"), []byte(after), 0o644); err != nil {
		t.Fatal(err)
	}
	proposed, err := db.CreateProposedChange(ctx, changes.ProposedChange{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		FilePath:      "main.go",
		Action:        changes.ActionReplace,
		Content:       after,
		Reason:        "test rollback",
		Status:        changes.StatusPending,
	})
	if err != nil {
		t.Fatalf("create proposed change: %v", err)
	}
	if err := db.MarkProposedChangeApplied(ctx, proposed.ID, ".zavod/backups/test/main.go", before, after, changes.GenerateUnifiedDiff("main.go", before, after)); err != nil {
		t.Fatalf("mark applied: %v", err)
	}
	service := &Service{store: db, agentStatuses: map[string]agents.Status{}}

	if _, err := service.RollbackWorkflowChanges(ctx, RollbackWorkflowChangesInput{
		ProjectID:     currentProject.ID,
		WorkflowRunID: run.ID,
	}); err != nil {
		t.Fatalf("rollback workflow: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(projectPath, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != before {
		t.Fatalf("expected file rollback, got %q", string(content))
	}
	items, err := db.ListProposedChanges(ctx, currentProject.ID, run.ID, 10)
	if err != nil {
		t.Fatalf("list changes: %v", err)
	}
	if len(items) != 1 || items[0].Status != changes.StatusRolledBack || !strings.Contains(items[0].DiffText, "-func main()") {
		t.Fatalf("expected rolled back change with rollback diff, got %#v", items)
	}
}

func TestCreateProjectBindsSelectedAgentGroup(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	service := &Service{
		store: db,
		paths: config.Paths{ProjectsDir: filepath.Join(tmp, "projects")},
	}

	created, err := service.CreateProject(ctx, CreateProjectInput{
		Name:    "CTF project",
		GroupID: "group_ctf_cell",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	binding, err := db.ProjectGroupBinding(ctx, created.ID)
	if err != nil {
		t.Fatalf("project binding: %v", err)
	}
	if binding.GroupID != "group_ctf_cell" || binding.LifecycleID != "lifecycle_ctf_default" {
		t.Fatalf("expected selected CTF binding, got %#v", binding)
	}
}

func TestListAgentGroupTemplates(t *testing.T) {
	service := &Service{}
	templates, err := service.ListAgentGroupTemplates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 5 {
		t.Fatalf("expected 5 templates, got %#v", templates)
	}
	seen := map[string]bool{}
	for _, template := range templates {
		seen[template.ID] = template.AgentCount > 0 && template.StepCount > 0
	}
	for _, id := range []string{"dev_squad", "ctf_cell", "research_desk", "security_audit", "solo_lumen"} {
		if !seen[id] {
			t.Fatalf("expected template %s with agents and steps, got %#v", id, templates)
		}
	}
}

func TestAgentLibraryAddsDuplicatesAndReplacesSoul(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	service := &Service{
		store: db,
		paths: config.Paths{AgentsDir: filepath.Join(tmp, "agents")},
	}

	library, err := service.ListAgentLibrary(ctx)
	if err != nil {
		t.Fatalf("list library: %v", err)
	}
	if len(library) < 10 {
		t.Fatalf("expected local agent library, got %#v", library)
	}

	profiles, err := service.AddAgentFromLibrary(ctx, AddAgentFromLibraryInput{
		GroupID:        "group_dev_squad",
		LibraryAgentID: "lib_python_developer",
		ModelID:        "qwen-remote",
	})
	if err != nil {
		t.Fatalf("add from library: %v", err)
	}
	var added agentgroups.Profile
	for _, profile := range profiles {
		if strings.HasPrefix(profile.Name, "Python-разработчик") {
			added = profile
			break
		}
	}
	if added.ID == "" || len(added.Capabilities) == 0 || added.ToolProfileID != "tool_python_dev" {
		t.Fatalf("expected added Python developer with contract, got %#v", added)
	}

	duplicated, err := service.DuplicateAgentProfile(ctx, DuplicateAgentProfileInput{ProfileID: added.ID})
	if err != nil {
		t.Fatalf("duplicate profile: %v", err)
	}
	if len(duplicated) != len(profiles)+1 {
		t.Fatalf("expected duplicated profile, before=%d after=%d", len(profiles), len(duplicated))
	}

	before := strings.Join(added.Capabilities, "\n")
	soul, err := service.ReplaceAgentSoulFromLibrary(ctx, ReplaceAgentSoulFromLibraryInput{
		ProfileID:      added.ID,
		LibraryAgentID: "lib_reviewer",
	})
	if err != nil {
		t.Fatalf("replace soul from library: %v", err)
	}
	if !strings.Contains(soul.Content, "Ревьюер") {
		t.Fatalf("expected reviewer soul content, got %q", soul.Content)
	}
	after, err := db.GetAgentProfile(ctx, added.ID)
	if err != nil {
		t.Fatalf("get after soul replace: %v", err)
	}
	if strings.Join(after.Capabilities, "\n") != before {
		t.Fatalf("soul replacement must not change capabilities: before=%#v after=%#v", added.Capabilities, after.Capabilities)
	}
}

func TestCreateAgentGroupFromTemplateCreatesEditableCopy(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	service := &Service{
		store: db,
		paths: config.Paths{AgentsDir: filepath.Join(tmp, "agents")},
	}

	groups, err := service.CreateAgentGroupFromTemplate(ctx, CreateAgentGroupFromTemplateInput{
		TemplateID:     "ctf_cell",
		Name:           "My CTF Team",
		DefaultModelID: "qwen-remote",
	})
	if err != nil {
		t.Fatalf("create from template: %v", err)
	}
	var created agentgroups.Group
	for _, group := range groups {
		if group.Name == "My CTF Team" {
			created = group
		}
	}
	if created.ID == "" || created.ID == "group_ctf_cell" || created.Kind != agentgroups.GroupKindCTF {
		t.Fatalf("expected custom CTF group copy, got %#v", created)
	}
	profiles, err := db.ListAgentProfiles(ctx, created.ID)
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(profiles) != 11 {
		t.Fatalf("expected 11 CTF template profiles, got %#v", profiles)
	}
	if profiles[0].SoulPath == "" {
		t.Fatalf("expected soul path to be created, got %#v", profiles[0])
	}
	if _, err := os.Stat(profiles[0].SoulPath); err != nil {
		t.Fatalf("expected soul file: %v", err)
	}
	steps, err := db.ListLifecycleSteps(ctx, created.DefaultLifecycleID)
	if err != nil {
		t.Fatalf("list lifecycle steps: %v", err)
	}
	if len(steps) != 8 || steps[0].StepKey != "intake" || steps[len(steps)-1].StepKey != "writeup" {
		t.Fatalf("expected CTF lifecycle steps, got %#v", steps)
	}
}

func TestCTFRequestDoesNotAutoSwitchDevProjectGroup(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	project, err := db.CreateProject(ctx, "Dev project", filepath.Join(tmp, "project"))
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := db.BindProjectToAgentGroup(ctx, project.ID, "group_dev_squad", "lifecycle_dev_default"); err != nil {
		t.Fatalf("bind dev group: %v", err)
	}
	service := &Service{store: db}

	if service.shouldUseCTFWorkflow(ctx, project.ID, "реши CTF LFI challenge") {
		t.Fatal("dev project must not switch to CTF workflow implicitly")
	}
	binding, err := db.ProjectGroupBinding(ctx, project.ID)
	if err != nil {
		t.Fatalf("project binding: %v", err)
	}
	if binding.GroupID != "group_dev_squad" {
		t.Fatalf("expected group to stay Dev Squad, got %#v", binding)
	}
}

func TestBuildCTFWorkspaceStateAggregatesFilesAndSteps(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	projectPath := filepath.Join(tmp, "project")
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatal(err)
	}
	workspace, err := ctf.PrepareWorkspace(projectPath, "Baby SQLi", "CTF SQLi challenge", ctf.CategorySQLi, time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("prepare ctf workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, workspace.SolveDir, "solve.py"), []byte("print('flag')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	currentProject, err := db.CreateProject(ctx, "CTF project", projectPath)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := db.CreateTask(ctx, currentProject.ID, "Baby SQLi")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	run, err := db.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	for _, item := range []artifacts.Artifact{
		{Kind: "ctf_challenge", Title: "CTF challenge", RelativePath: workspace.ChallengeYAML, Path: filepath.Join(projectPath, workspace.ChallengeYAML)},
		{Kind: "ctf_scope", Title: "CTF scope", RelativePath: workspace.ScopeMD, Path: filepath.Join(projectPath, workspace.ScopeMD)},
		{Kind: "ctf_writeup", Title: "CTF writeup", RelativePath: workspace.WriteupMD, Path: filepath.Join(projectPath, workspace.WriteupMD)},
	} {
		item.ProjectID = currentProject.ID
		item.TaskID = task.ID
		item.WorkflowRunID = run.ID
		item.AgentID = agents.ManagerID
		if _, err := db.CreateArtifact(ctx, item); err != nil {
			t.Fatalf("create artifact: %v", err)
		}
	}
	step, err := db.CreateWorkflowStep(ctx, run.ID, zw.StepCTFHypothesisBoard, agents.ManagerID, "")
	if err != nil {
		t.Fatalf("create step: %v", err)
	}
	if _, err := db.FinishWorkflowStep(ctx, step.ID, zw.StepStatusDone, "## Гипотезы\n\n- injectable id", ""); err != nil {
		t.Fatalf("finish step: %v", err)
	}
	steps, err := db.ListWorkflowSteps(ctx, run.ID)
	if err != nil {
		t.Fatalf("list steps: %v", err)
	}
	artifactItems, err := db.ListArtifacts(ctx, currentProject.ID, 20)
	if err != nil {
		t.Fatalf("list artifacts: %v", err)
	}
	service := &Service{store: db}

	state := service.buildCTFWorkspaceState(currentProject, &task, &run, steps, artifactItems)
	if state == nil {
		t.Fatal("expected ctf workspace state")
	}
	if state.Category != ctf.CategorySQLi || state.Root != workspace.RelativeRoot || state.WriteupPath != workspace.WriteupMD {
		t.Fatalf("unexpected ctf workspace summary: %#v", state)
	}
	if !strings.Contains(state.Hypotheses.Content, "injectable id") {
		t.Fatalf("expected hypothesis step content, got %#v", state.Hypotheses)
	}
	if !hasCTFWorkspaceFileForTest(state.Files, filepath.ToSlash(filepath.Join(workspace.SolveDir, "solve.py"))) {
		t.Fatalf("expected solver file in workspace files, got %#v", state.Files)
	}
}

func hasCTFWorkspaceFileForTest(files []CTFWorkspaceFile, relativePath string) bool {
	for _, file := range files {
		if file.RelativePath == relativePath {
			return true
		}
	}
	return false
}

func TestApplyDevPipelineChangeFormatsGoFiles(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt is not available in PATH")
	}
	tmp := t.TempDir()
	service := &Service{}
	result, err := service.applyDevPipelineChange(context.Background(), tmp, changes.ProposedChange{
		ID:       "change_go",
		FilePath: "main.go",
		Action:   changes.ActionCreate,
		Content:  "package main\nfunc main(){println(\"ok\")}\n",
	})
	if err != nil {
		t.Fatalf("apply go change: %v", err)
	}
	if !strings.Contains(result.AfterContent, "func main()") || strings.Contains(result.AfterContent, "func main(){") {
		t.Fatalf("expected gofmt output, got %q", result.AfterContent)
	}
	if !strings.Contains(result.DiffText, "func main()") {
		t.Fatalf("expected formatted diff, got %q", result.DiffText)
	}
}

func TestBlueprintExpectedPath(t *testing.T) {
	items := []blueprint.ExpectedFile{{Path: "requirements.txt"}}
	if !blueprintExpectedPath(items, "requirements.txt") {
		t.Fatal("expected requirements.txt to be detected")
	}
}

func TestAgentSoulStoredAsMarkdownFile(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	db, err := store.New(filepath.Join(tmp, "zavod.db"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer db.Close()
	if err := db.EnsureDefaultModels(ctx); err != nil {
		t.Fatalf("ensure models: %v", err)
	}
	if err := db.EnsureDefaultAgentGroups(ctx, "qwen-remote"); err != nil {
		t.Fatalf("ensure groups: %v", err)
	}
	service := &Service{
		store: db,
		paths: config.Paths{
			CodeDir:   tmp,
			AgentsDir: filepath.Join(tmp, "agents"),
			DBPath:    filepath.Join(tmp, "zavod.db"),
		},
	}
	if err := service.ensureDefaultAgentSouls(ctx); err != nil {
		t.Fatalf("ensure souls: %v", err)
	}
	profiles, err := db.ListAgentProfiles(ctx, "group_dev_squad")
	if err != nil {
		t.Fatalf("list profiles: %v", err)
	}
	if len(profiles) == 0 || profiles[0].SoulPath == "" {
		t.Fatalf("expected soul path on profile, got %#v", profiles)
	}
	content, err := os.ReadFile(profiles[0].SoulPath)
	if err != nil {
		t.Fatalf("read soul file: %v", err)
	}
	if !strings.Contains(string(content), "# Soul") {
		t.Fatalf("expected markdown soul content, got %q", string(content))
	}

	saved, err := service.SaveAgentSoul(ctx, SaveAgentSoulInput{
		ProfileID: profiles[0].ID,
		Content:   "# Soul\n\n## Кто я\n\nНовая версия без scope.\n",
	})
	if err != nil {
		t.Fatalf("save soul: %v", err)
	}
	if len(saved.Warnings) == 0 {
		t.Fatalf("expected warning for suspicious soul content")
	}
	content, err = os.ReadFile(saved.Path)
	if err != nil {
		t.Fatalf("read saved soul file: %v", err)
	}
	if !strings.Contains(string(content), "Новая версия") {
		t.Fatalf("expected saved soul content, got %q", string(content))
	}
}
