package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"zavod_ai/internal/agents"
	"zavod_ai/internal/artifacts"
	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/config"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/providers/openaiapi"
	"zavod_ai/internal/reviews"
	"zavod_ai/internal/router"
	"zavod_ai/internal/store"
	"zavod_ai/internal/webresearch"
	zw "zavod_ai/internal/workflow"
)

const (
	selectedProjectSetting   = "selected_project_id"
	webSettingsKey           = "web_research_settings"
	managerMaxAnswerBytes    = 12 * 1024
	managerMaxStreamEvents   = 1800
	repetitionLoopMinRepeats = 8
	maxAutoRepairIterations  = 2
	modelHealthFastInterval  = 5 * time.Second
	modelHealthBaseInterval  = 10 * time.Second
	modelHealthSlowInterval  = 30 * time.Second
	modelHealthStableChecks  = 6
)

type EventSink interface {
	Emit(name string, data any)
}

type Service struct {
	store *store.Store
	paths config.Paths
	sink  EventSink

	mu            sync.Mutex
	agentStatuses map[string]agents.Status
}

type BootstrapState struct {
	Paths             config.Paths     `json:"paths"`
	Projects          []ProjectDTO     `json:"projects"`
	SelectedProjectID string           `json:"selectedProjectId"`
	Chat              ProjectState     `json:"chat"`
	Agents            []agents.Status  `json:"agents"`
	Models            []ModelConfigDTO `json:"models"`
	ActiveModelID     string           `json:"activeModelId"`
	WebSettings       WebSettingsDTO   `json:"webSettings"`
}

type ProjectState struct {
	Project       ProjectDTO            `json:"project"`
	Task          *TaskDTO              `json:"task,omitempty"`
	Messages      []MessageDTO          `json:"messages"`
	WorkflowRun   *WorkflowRunDTO       `json:"workflowRun,omitempty"`
	WorkflowSteps []WorkflowStepDTO     `json:"workflowSteps"`
	WorkflowPlan  *WorkflowPlanDTO      `json:"workflowPlan,omitempty"`
	PlanSteps     []WorkflowPlanStepDTO `json:"planSteps"`
	Artifacts     []ArtifactDTO         `json:"artifacts"`
	Blueprint     *BlueprintDTO         `json:"blueprint,omitempty"`
	Clarification *ClarificationDTO     `json:"clarification,omitempty"`
	Changes       []ProposedChangeDTO   `json:"changes"`
	TestRuns      []TestRunDTO          `json:"testRuns"`
	Reviews       []ReviewRunDTO        `json:"reviews"`
	WebSources    []WebSourceDTO        `json:"webSources"`
}

type ChatState struct {
	ProjectState
	Agents []agents.Status `json:"agents"`
	Error  string          `json:"error,omitempty"`
}

type AgentMessageDelta struct {
	TaskID  string `json:"taskId"`
	AgentID string `json:"agentId"`
	Delta   string `json:"delta"`
	Done    bool   `json:"done"`
	Error   string `json:"error,omitempty"`
}

type WorkflowRunChanged struct {
	Run WorkflowRunDTO `json:"run"`
}

type WorkflowStepChanged struct {
	Run  WorkflowRunDTO  `json:"run"`
	Step WorkflowStepDTO `json:"step"`
}

type CreateProjectInput struct {
	Name string `json:"name"`
}

type AddExistingProjectInput struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type UpdateProjectInput struct {
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

type DeleteProjectInput struct {
	ProjectID string `json:"projectId"`
}

type SendMessageInput struct {
	ProjectID string `json:"projectId"`
	Content   string `json:"content"`
}

type SubmitClarificationInput struct {
	ProjectID     string                `json:"projectId"`
	WorkflowRunID string                `json:"workflowRunId"`
	Answers       []ClarificationAnswer `json:"answers"`
}

type ClarificationDTO struct {
	WorkflowRunID string                  `json:"workflowRunId"`
	Summary       string                  `json:"summary"`
	Goal          string                  `json:"goal"`
	Questions     []ClarificationQuestion `json:"questions"`
}

type ClarificationQuestion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type ClarificationAnswer struct {
	QuestionID string `json:"questionId"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

type ApplyWorkflowChangesInput struct {
	ProjectID     string `json:"projectId"`
	WorkflowRunID string `json:"workflowRunId"`
}

type RunTestCommandInput struct {
	ProjectID string `json:"projectId"`
	TestRunID string `json:"testRunId"`
}

type RunReviewInput struct {
	ProjectID     string `json:"projectId"`
	WorkflowRunID string `json:"workflowRunId"`
}

type SaveModelConfigInput struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"baseUrl"`
	APIKeyRef string `json:"apiKeyRef"`
	ModelName string `json:"modelName"`
	IsActive  bool   `json:"isActive"`
}

type SaveWebSettingsInput struct {
	Enabled             bool     `json:"enabled"`
	MaxResults          int      `json:"maxResults"`
	MaxPagesPerWorkflow int      `json:"maxPagesPerWorkflow"`
	TimeoutSeconds      int      `json:"timeoutSeconds"`
	AllowedDomains      []string `json:"allowedDomains"`
	BlockedDomains      []string `json:"blockedDomains"`
}

type ProjectDTO = project.Project
type TaskDTO = chat.Task
type MessageDTO = chat.Message
type ModelConfigDTO = llm.ModelConfig
type WorkflowRunDTO = zw.Run
type WorkflowStepDTO = zw.Step
type WorkflowPlanDTO = zw.Plan
type WorkflowPlanStepDTO = zw.PlanStep
type ArtifactDTO = artifacts.Artifact
type BlueprintDTO = blueprint.Blueprint
type PendingClarificationDTO = ClarificationDTO
type ProposedChangeDTO = changes.ProposedChange
type TestRunDTO = checks.TestRun
type ReviewRunDTO = reviews.ReviewRun
type WebSourceDTO = webresearch.Source
type WebSettingsDTO = webresearch.Settings

func NewService(ctx context.Context, sink EventSink) (*Service, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, err
	}
	if err := config.EnsureBaseDirs(paths); err != nil {
		return nil, err
	}

	db, err := store.New(paths.DBPath)
	if err != nil {
		return nil, err
	}
	if err := db.EnsureDefaultModels(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	service := &Service{
		store:         db,
		paths:         paths,
		sink:          sink,
		agentStatuses: map[string]agents.Status{},
	}
	if err := service.ensureDefaultProject(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	activeModel, err := db.ActiveModelConfig(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	service.resetAgentStatuses(activeModel.ID)
	service.startModelHealthMonitor(ctx)
	return service, nil
}

func (s *Service) Bootstrap(ctx context.Context) (BootstrapState, error) {
	projects, err := s.store.ListProjects(ctx, "")
	if err != nil {
		return BootstrapState{}, err
	}
	models, err := s.store.ListModelConfigs(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	activeModel, err := s.store.ActiveModelConfig(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	webSettings := s.webSettings(ctx)

	selectedID, _, _ := s.store.GetSetting(ctx, selectedProjectSetting)
	if selectedID == "" && len(projects) > 0 {
		selectedID = projects[0].ID
	}

	state := ProjectState{}
	if selectedID != "" {
		projectState, err := s.projectState(ctx, selectedID)
		if err == nil {
			state = projectState
		} else if len(projects) > 0 {
			selectedID = projects[0].ID
			state, err = s.projectState(ctx, selectedID)
			if err != nil {
				return BootstrapState{}, err
			}
		}
	}

	return BootstrapState{
		Paths:             s.paths,
		Projects:          projects,
		SelectedProjectID: selectedID,
		Chat:              state,
		Agents:            s.AgentStatuses(),
		Models:            models,
		ActiveModelID:     activeModel.ID,
		WebSettings:       webSettings,
	}, nil
}

func (s *Service) ListProjects(ctx context.Context, query string) ([]ProjectDTO, error) {
	return s.store.ListProjects(ctx, query)
}

func (s *Service) CreateProject(ctx context.Context, input CreateProjectInput) (ProjectDTO, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Новый проект"
	}

	path := uniqueProjectPath(s.paths.ProjectsDir, safeProjectDirName(name))
	if err := os.MkdirAll(path, 0o755); err != nil {
		return ProjectDTO{}, err
	}

	item, err := s.store.CreateProject(ctx, name, path)
	if err != nil {
		return ProjectDTO{}, err
	}
	_ = s.store.SetSetting(ctx, selectedProjectSetting, item.ID)
	return item, nil
}

func (s *Service) AddExistingProject(ctx context.Context, input AddExistingProjectInput) (ProjectDTO, error) {
	path, err := expandPath(input.Path)
	if err != nil {
		return ProjectDTO{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return ProjectDTO{}, fmt.Errorf("каталог проекта не найден: %w", err)
	}
	if !info.IsDir() {
		return ProjectDTO{}, fmt.Errorf("путь проекта должен быть каталогом")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(path)
	}

	item, err := s.store.CreateProject(ctx, name, path)
	if err != nil {
		return ProjectDTO{}, err
	}
	_ = s.store.SetSetting(ctx, selectedProjectSetting, item.ID)
	return item, nil
}

func (s *Service) UpdateProject(ctx context.Context, input UpdateProjectInput) (ProjectDTO, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return ProjectDTO{}, fmt.Errorf("project_id пустой")
	}
	path, err := expandPath(input.Path)
	if err != nil {
		return ProjectDTO{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return ProjectDTO{}, fmt.Errorf("каталог проекта не найден: %w", err)
	}
	if !info.IsDir() {
		return ProjectDTO{}, fmt.Errorf("путь проекта должен быть каталогом")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(path)
	}
	item, err := s.store.UpdateProject(ctx, projectID, name, path)
	if err != nil {
		return ProjectDTO{}, err
	}
	return item, nil
}

func (s *Service) DeleteProject(ctx context.Context, input DeleteProjectInput) (BootstrapState, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	if projectID == "" {
		return BootstrapState{}, fmt.Errorf("project_id пустой")
	}
	if err := s.store.DeleteProject(ctx, projectID); err != nil {
		return BootstrapState{}, err
	}
	selectedID, _, _ := s.store.GetSetting(ctx, selectedProjectSetting)
	if selectedID == projectID {
		_ = s.store.SetSetting(ctx, selectedProjectSetting, "")
	}
	if err := s.ensureDefaultProject(ctx); err != nil {
		return BootstrapState{}, err
	}
	return s.Bootstrap(ctx)
}

func (s *Service) SelectProject(ctx context.Context, projectID string) (ProjectState, error) {
	if strings.TrimSpace(projectID) == "" {
		return ProjectState{}, fmt.Errorf("project_id пустой")
	}
	if err := s.store.TouchProject(ctx, projectID); err != nil {
		return ProjectState{}, err
	}
	if err := s.store.SetSetting(ctx, selectedProjectSetting, projectID); err != nil {
		return ProjectState{}, err
	}
	return s.projectState(ctx, projectID)
}

func (s *Service) SendMessage(ctx context.Context, input SendMessageInput) (ChatState, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return ChatState{}, fmt.Errorf("сообщение пустое")
	}

	currentProject, err := s.store.GetProject(ctx, input.ProjectID)
	if err != nil {
		return ChatState{}, err
	}
	_ = s.store.TouchProject(ctx, input.ProjectID)
	_ = s.store.SetSetting(ctx, selectedProjectSetting, input.ProjectID)

	task, err := s.store.GetActiveTask(ctx, input.ProjectID)
	if err != nil {
		return ChatState{}, err
	}
	if task == nil {
		created, err := s.store.CreateTask(ctx, input.ProjectID, titleFromContent(content))
		if err != nil {
			return ChatState{}, err
		}
		task = &created
	}

	if _, err := s.store.AddMessage(ctx, task.ID, "user", "", content); err != nil {
		return ChatState{}, err
	}
	_ = s.store.TouchTask(ctx, task.ID)
	s.emitChatState(ctx, input.ProjectID, "")

	model, err := s.store.ActiveModelConfig(ctx)
	if err != nil {
		return ChatState{}, err
	}
	provider := openaiapi.NewClient(model.BaseURL, model.APIKeyRef)

	latestRun, err := s.store.LatestWorkflowRun(ctx, task.ID)
	if err != nil {
		return ChatState{}, err
	}
	decision := router.Route(content, router.Context{
		HasActiveClarification: latestRun != nil && latestRun.Status == zw.StatusWaitingUser,
	})
	if decision.Confidence == "low" {
		decision = s.classifyIntentWithModel(ctx, provider, model, currentProject, *task, latestRun, content, decision)
	}
	if !decision.NeedsWorkflow {
		return s.answerDirect(ctx, currentProject, *task, latestRun, provider, model, content, decision)
	}

	run, err := s.store.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		return ChatState{}, err
	}
	s.emitWorkflowRun(run)
	_ = s.createDynamicWorkflowPlan(ctx, currentProject, *task, run.ID, provider, model, content, decision.Intent)

	history, err := s.store.ListMessages(ctx, task.ID)
	if err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, run.CurrentStep, err.Error())
		return ChatState{}, err
	}
	if decision.Intent == router.IntentPentestTask {
		return s.runSecurityWorkflow(ctx, input.ProjectID, currentProject, *task, &run, history, provider, model, content)
	}
	if decision.Intent == router.IntentResearchTask {
		return s.runWebResearchWorkflow(ctx, input.ProjectID, currentProject, *task, &run, history, provider, model, content)
	}

	result, err := s.runV03Workflow(ctx, currentProject, task.ID, &run, history, provider, model)
	if err != nil {
		return s.handleWorkflowError(ctx, input.ProjectID, task.ID, run.ID, run.CurrentStep, model.ID, err), nil
	}
	if result.NeedsClarification {
		if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusWaitingUser, zw.StepManagerIntake, ""); err != nil {
			return ChatState{}, err
		}
		run.Status = zw.StatusWaitingUser
		run.CurrentStep = zw.StepManagerIntake
		run.FinishedAt = nowString()
		s.emitWorkflowRun(run)
		s.setAgentStatus(agents.ManagerID, "waiting_user", "Ждет уточнение пользователя", model.ID)
		if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, clarificationNotice(result.Clarification)); err != nil {
			return ChatState{}, err
		}
		_ = s.store.TouchTask(ctx, task.ID)
		return s.emitChatState(ctx, input.ProjectID, ""), nil
	}

	pendingChanges, err := s.saveProposedChanges(ctx, currentProject, *task, run.ID, result.Developer)
	autoResult := autopilotResult{}
	if err != nil || len(pendingChanges) == 0 && blueprintRequiresCodeChanges(result.Blueprint) {
		repairReason := developerChangesRepairReason(err, len(pendingChanges), result.Blueprint)
		repairInput := buildDeveloperStructuredChangesRepairInput(result, currentProject, repairReason)
		repairedDeveloper, repairErr := s.runWorkflowStep(ctx, input.ProjectID, task.ID, &run, provider, model, zw.StepDeveloperPlan, repairInput)
		if repairErr != nil {
			autoResult.Blocked = true
			autoResult.BlockReason = "repair proposed changes не выполнен: " + repairErr.Error()
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, autoResult.BlockReason)
		} else {
			result.Developer = repairedDeveloper
			pendingChanges, err = s.saveProposedChanges(ctx, currentProject, *task, run.ID, result.Developer)
			if err != nil {
				autoResult.Blocked = true
				autoResult.BlockReason = "не удалось разобрать proposed changes Разработчика после repair: " + err.Error()
				_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, autoResult.BlockReason)
			} else if len(pendingChanges) == 0 && blueprintRequiresCodeChanges(result.Blueprint) {
				autoResult.Blocked = true
				autoResult.BlockReason = "Разработчик не вернул применимые proposed changes после repair, хотя Task Blueprint ожидает файлы"
				_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, autoResult.BlockReason)
			}
		}
	}
	if !autoResult.Blocked {
		autoResult = s.runAutopilot(ctx, currentProject, *task, &run, provider, model, &result, len(pendingChanges))
	}
	if autoResult.Blocked {
		result.Final = deterministicBlockedFinal(autoResult)
	} else {
		finalInput := s.buildAutopilotFinalInput(ctx, result, currentProject, run.ID, autoResult)
		final, err := s.runWorkflowStep(ctx, input.ProjectID, task.ID, &run, provider, model, zw.StepManagerFinal, finalInput)
		if err != nil {
			return s.handleWorkflowError(ctx, input.ProjectID, task.ID, run.ID, zw.StepManagerFinal, model.ID, err), nil
		}
		result.Final = final
	}

	s.setAgentStatus(agents.ManagerID, "writing_files", "Сохраняет артефакты в проект", model.ID)
	writtenArtifacts, err := s.saveWorkflowArtifacts(ctx, currentProject, *task, run.ID, result)
	if err != nil {
		return s.handleWorkflowError(ctx, input.ProjectID, task.ID, run.ID, zw.StepManagerFinal, model.ID, err), nil
	}

	_ = writtenArtifacts
	finalMessage := result.Final
	if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, finalMessage); err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepManagerFinal, err.Error())
		s.setAgentStatus(agents.ManagerID, "failed", "Не удалось сохранить итоговый ответ", model.ID)
		return ChatState{}, err
	}
	_ = s.store.TouchTask(ctx, task.ID)
	finalStatus := zw.StatusDone
	finalError := ""
	if autoResult.Blocked {
		finalStatus = zw.StatusBlocked
		finalError = autoResult.BlockReason
	}
	if err := s.store.UpdateWorkflowRun(ctx, run.ID, finalStatus, zw.StepManagerFinal, finalError); err != nil {
		return ChatState{}, err
	}
	if autoResult.Blocked {
		_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusBlocked, finalError)
	} else {
		_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusDone, "")
	}

	run.Status = finalStatus
	run.CurrentStep = zw.StepManagerFinal
	run.FinishedAt = nowString()
	run.Error = finalError
	s.emitWorkflowRun(run)
	s.resetAgentStatuses(model.ID)
	return s.emitChatState(ctx, input.ProjectID, ""), nil
}

func (s *Service) answerDirect(
	ctx context.Context,
	currentProject project.Project,
	task chat.Task,
	latestRun *zw.Run,
	provider llm.Provider,
	model llm.ModelConfig,
	content string,
	decision router.Decision,
) (ChatState, error) {
	s.setAgentStatus(agents.ManagerID, "answering", directAnswerActivity(decision.Intent), model.ID)
	s.emitChatState(ctx, currentProject.ID, "")

	if wantsSavedTaskSpec(content) {
		answer := s.savedTaskSpecAnswer(ctx, currentProject)
		if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, answer); err != nil {
			s.setAgentStatus(agents.ManagerID, "failed", "Не удалось сохранить спеку", model.ID)
			return ChatState{}, err
		}
		_ = s.store.TouchTask(ctx, task.ID)
		s.resetAgentStatuses(model.ID)
		return s.emitChatState(ctx, currentProject.ID, ""), nil
	}

	answerCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	resp, err := provider.Generate(answerCtx, llm.Request{
		Model: model.ModelName,
		Messages: []llm.Message{
			{
				Role: "system",
				Content: agents.WithDefaultSkills(`
Ты Люмен, входной агент локального AI-завода.
Отвечай на русском языке. Говори о себе в женском роде: "поняла", "проверила", "собрала".
Сейчас режим Direct Answer: пользователь задал вопрос или попросил объяснение, а не запуск полного workflow.
Не утверждай, что ты изменила файлы, запустила тесты или провела пентест, если этого нет в контексте.
Не выводи сырой JSON, если можно объяснить человечески.
Если вопрос про безопасность или пентест, сначала фиксируй scope и разрешение; не предлагай атаковать внешние цели без явного разрешения.
Ответ должен быть коротким, практичным и привязанным к фактам из контекста.
`),
			},
			{
				Role:    "user",
				Content: s.buildDirectAnswerInput(ctx, currentProject, task, latestRun, content, decision),
			},
		},
		Temperature: 0.2,
		MaxTokens:   1200,
	})
	if err != nil {
		s.setAgentStatus(agents.ManagerID, "failed", "Не смогла ответить напрямую", model.ID)
		return s.emitChatState(ctx, currentProject.ID, "Люмен не смогла ответить напрямую: "+err.Error()), nil
	}

	answer := strings.TrimSpace(resp.Content)
	if answer == "" {
		s.setAgentStatus(agents.ManagerID, "failed", "Модель вернула пустой ответ", model.ID)
		return s.emitChatState(ctx, currentProject.ID, "Модель вернула пустой ответ."), nil
	}
	if looksLikeRepetitionLoop(answer) || len(answer) > managerMaxAnswerBytes {
		answer = "Ответ остановлен: модель начала повторять текст или сгенерировала слишком длинный ответ. Попробуй переформулировать вопрос короче."
	}
	if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, answer); err != nil {
		s.setAgentStatus(agents.ManagerID, "failed", "Не удалось сохранить ответ", model.ID)
		return ChatState{}, err
	}
	_ = s.store.TouchTask(ctx, task.ID)
	s.resetAgentStatuses(model.ID)
	return s.emitChatState(ctx, currentProject.ID, ""), nil
}

type dynamicPlanOutput struct {
	Title string `json:"title"`
	Steps []struct {
		StepKey     string `json:"step_key"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Agent       string `json:"agent"`
	} `json:"steps"`
}

func (s *Service) createDynamicWorkflowPlan(
	ctx context.Context,
	currentProject project.Project,
	task chat.Task,
	workflowRunID string,
	provider llm.Provider,
	model llm.ModelConfig,
	userMessage string,
	intent router.Intent,
) error {
	plan, steps := fallbackWorkflowPlan(currentProject, task, workflowRunID, userMessage, intent)

	planCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	input := fmt.Sprintf("Intent: %s\nПроект: %s\nЗадача: %s", intent, currentProject.Name, userMessage)
	if resp, err := provider.Generate(planCtx, agents.RequestForSpec(model.ModelName, agents.SpecForStep(zw.StepUserPlan), input)); err == nil && resp != nil {
		if parsedPlan, parsedSteps, ok := parseDynamicWorkflowPlan(resp.Content, currentProject, task, workflowRunID, intent); ok {
			plan = parsedPlan
			steps = parsedSteps
		}
	}

	_, _, err := s.store.CreateWorkflowPlan(ctx, plan, steps)
	if err == nil {
		s.emitChatState(ctx, currentProject.ID, "")
	}
	return err
}

func parseDynamicWorkflowPlan(raw string, currentProject project.Project, task chat.Task, workflowRunID string, intent router.Intent) (zw.Plan, []zw.PlanStep, bool) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			trimmed = trimmed[start : end+1]
		}
	}
	var parsed dynamicPlanOutput
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return zw.Plan{}, nil, false
	}
	plan, fallbackSteps := fallbackWorkflowPlan(currentProject, task, workflowRunID, "", intent)
	if strings.TrimSpace(parsed.Title) != "" {
		plan.Title = strings.TrimSpace(parsed.Title)
	}
	steps := make([]zw.PlanStep, 0, len(parsed.Steps))
	seen := map[string]bool{}
	for _, item := range parsed.Steps {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		stepKey := sanitizePlanStepKey(item.StepKey, title, len(steps)+1)
		if seen[stepKey] {
			stepKey = fmt.Sprintf("%s_%d", stepKey, len(steps)+1)
		}
		seen[stepKey] = true
		steps = append(steps, zw.PlanStep{
			StepKey:     stepKey,
			Title:       title,
			Description: strings.TrimSpace(item.Description),
			AgentID:     normalizePlanAgent(item.Agent),
			Status:      zw.StepStatusQueued,
			SortOrder:   len(steps),
		})
		if len(steps) >= 8 {
			break
		}
	}
	if len(steps) == 0 {
		return plan, fallbackSteps, false
	}
	return plan, steps, true
}

func sanitizePlanStepKey(value string, title string, index int) string {
	key := strings.ToLower(strings.TrimSpace(value))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(title))
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, char := range key {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	key = strings.Trim(builder.String(), "_")
	if key == "" {
		key = fmt.Sprintf("step_%d", index)
	}
	return key
}

func normalizePlanAgent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case agents.ProductID:
		return agents.ProductID
	case agents.ArchitectID:
		return agents.ArchitectID
	case agents.DeveloperID:
		return agents.DeveloperID
	case agents.TesterID:
		return agents.TesterID
	case agents.ReviewerID:
		return agents.ReviewerID
	case agents.SecurityID:
		return agents.SecurityID
	default:
		return agents.ManagerID
	}
}

func fallbackWorkflowPlan(currentProject project.Project, task chat.Task, workflowRunID string, userMessage string, intent router.Intent) (zw.Plan, []zw.PlanStep) {
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = titleFromContent(userMessage)
	}
	if title == "" {
		title = "Выполнение задачи"
	}
	plan := zw.Plan{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRunID,
		Title:         title,
		Status:        zw.StatusRunning,
	}
	var specs []zw.PlanStep
	switch intent {
	case router.IntentResearchTask:
		specs = []zw.PlanStep{
			{StepKey: zw.StepWebResearch, Title: "Найти источники", Description: "Собрать актуальную информацию и проверить страницы", AgentID: agents.ManagerID},
			{StepKey: zw.StepManagerFinal, Title: "Собрать ответ", Description: "Сформировать выводы со ссылками на источники", AgentID: agents.ManagerID},
		}
	case router.IntentPentestTask:
		specs = []zw.PlanStep{
			{StepKey: zw.StepSecurityAnalysis, Title: "Проверить scope", Description: "Разобрать разрешение, цели и безопасные границы задачи", AgentID: agents.SecurityID},
			{StepKey: zw.StepManagerFinal, Title: "Собрать итог", Description: "Выдать defensive-рекомендации или вопросы на уточнение", AgentID: agents.ManagerID},
		}
	default:
		specs = []zw.PlanStep{
			{StepKey: zw.StepManagerIntake, Title: "Понять задачу", Description: "Собрать цель, ограничения и недостающий контекст", AgentID: agents.ManagerID},
			{StepKey: zw.StepProductRequirements, Title: "Сформировать требования", Description: "Зафиксировать ожидаемое поведение и критерии готовности", AgentID: agents.ProductID},
			{StepKey: zw.StepTaskBlueprint, Title: "Зафиксировать контракт", Description: "Определить стек, файлы, entrypoint и проверки", AgentID: agents.ArchitectID},
			{StepKey: zw.StepArchitectPlan, Title: "Спроектировать решение", Description: "Определить технический подход и риски", AgentID: agents.ArchitectID},
			{StepKey: zw.StepDeveloperPlan, Title: "Написать код", Description: "Подготовить и применить изменения в проекте", AgentID: agents.DeveloperID},
			{StepKey: zw.StepTesterCommands, Title: "Проверить", Description: "Запустить релевантные команды проверки", AgentID: agents.TesterID},
			{StepKey: zw.StepReview, Title: "Провести ревью", Description: "Проверить diff, тесты и соответствие задаче", AgentID: agents.ReviewerID},
			{StepKey: zw.StepManagerFinal, Title: "Собрать итог", Description: "Коротко объяснить результат и важные детали", AgentID: agents.ManagerID},
		}
	}
	for index := range specs {
		specs[index].Status = zw.StepStatusQueued
		specs[index].SortOrder = index
	}
	return plan, specs
}

func (s *Service) runWebResearchWorkflow(
	ctx context.Context,
	projectID string,
	currentProject project.Project,
	task chat.Task,
	run *zw.Run,
	history []chat.Message,
	provider llm.Provider,
	model llm.ModelConfig,
	userMessage string,
) (ChatState, error) {
	settings := s.webSettings(ctx)
	if !settings.Enabled {
		message := "## Web research выключен\n\nВключи поиск в настройках, и я смогу искать актуальную информацию в интернете с сохранением источников."
		_, _ = s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, message)
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepWebResearch, "web research выключен")
		_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusBlocked, "web research выключен")
		s.setAgentStatus(agents.ManagerID, "failed", "Web research выключен", model.ID)
		return s.emitChatState(ctx, projectID, ""), nil
	}

	planOutput, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepWebResearch, buildWebResearchPlanInput(userMessage, currentProject, history))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepWebResearch, model.ID, err), nil
	}
	plan := webresearch.ParsePlan(planOutput, userMessage)

	s.setAgentStatus(agents.ManagerID, "searching_web", "Ищет источники в интернете", model.ID)
	searchCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.TimeoutSeconds*settings.MaxPagesPerWorkflow+6)*time.Second)
	sources, searchErr := webresearch.NewClient(time.Duration(settings.TimeoutSeconds)*time.Second).Research(searchCtx, plan, settings)
	cancel()
	if searchErr != nil {
		message := fmt.Sprintf("## Не нашла источники\n\n%s", searchErr.Error())
		_, _ = s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, message)
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepWebResearch, searchErr.Error())
		_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusBlocked, searchErr.Error())
		s.setAgentStatus(agents.ManagerID, "failed", "Не нашла источники", model.ID)
		return s.emitChatState(ctx, projectID, ""), nil
	}

	for _, source := range sources {
		source.ProjectID = currentProject.ID
		source.TaskID = task.ID
		source.WorkflowRunID = run.ID
		source.AgentID = agents.ManagerID
		_, _ = s.store.CreateWebSource(ctx, source)
	}

	answerCtx, answerCancel := context.WithTimeout(ctx, 90*time.Second)
	defer answerCancel()
	s.setAgentStatus(agents.ManagerID, "answering", "Собирает ответ по источникам", model.ID)
	resp, err := provider.Generate(answerCtx, llm.Request{
		Model: model.ModelName,
		Messages: []llm.Message{
			{
				Role: "system",
				Content: agents.WithDefaultSkills(`
Ты Люмен, входной агент локального AI-завода.
Отвечай на русском языке. Говори о себе в женском роде: "нашла", "проверила", "собрала".
Сейчас режим Web Research: отвечай только на основе найденных источников и явно отделяй выводы от фактов.
Не выводи сырой JSON. Не утверждай, что меняла файлы или запускала проверки.
Если источников мало или они слабые, скажи об этом прямо.
Формат:
## Коротко
## Детали
## Источники
`),
			},
			{
				Role:    "user",
				Content: buildWebResearchAnswerInput(userMessage, currentProject, plan, sources),
			},
		},
		Temperature: 0.2,
		MaxTokens:   1600,
	})
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepWebResearch, model.ID, err), nil
	}
	answer := strings.TrimSpace(resp.Content)
	if answer == "" {
		answer = webResearchFallbackAnswer(sources)
	}
	if looksLikeRepetitionLoop(answer) || len(answer) > managerMaxAnswerBytes {
		answer = webResearchFallbackAnswer(sources)
	}
	if !strings.Contains(strings.ToLower(answer), "источник") {
		answer += "\n\n" + webResearchSourcesSection(sources)
	}
	if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, answer); err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepWebResearch, err.Error())
		return ChatState{}, err
	}
	if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusDone, zw.StepWebResearch, ""); err != nil {
		return ChatState{}, err
	}
	_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusDone, "")
	run.Status = zw.StatusDone
	run.CurrentStep = zw.StepWebResearch
	run.FinishedAt = nowString()
	run.Error = ""
	s.emitWorkflowRun(*run)
	_ = s.store.TouchTask(ctx, task.ID)
	s.resetAgentStatuses(model.ID)
	return s.emitChatState(ctx, projectID, ""), nil
}

func (s *Service) runSecurityWorkflow(
	ctx context.Context,
	projectID string,
	currentProject project.Project,
	task chat.Task,
	run *zw.Run,
	history []chat.Message,
	provider llm.Provider,
	model llm.ModelConfig,
	userMessage string,
) (ChatState, error) {
	input := buildSecurityAnalysisInput(userMessage, currentProject, history)
	output, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepSecurityAnalysis, input)
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepSecurityAnalysis, model.ID, err), nil
	}
	if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.SecurityID, output); err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepSecurityAnalysis, err.Error())
		s.setAgentStatus(agents.SecurityID, "failed", "Не удалось сохранить ИБ-ответ", model.ID)
		return ChatState{}, err
	}
	_ = s.store.TouchTask(ctx, task.ID)
	if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusDone, zw.StepSecurityAnalysis, ""); err != nil {
		return ChatState{}, err
	}
	_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusDone, "")
	run.Status = zw.StatusDone
	run.CurrentStep = zw.StepSecurityAnalysis
	run.FinishedAt = nowString()
	run.Error = ""
	s.emitWorkflowRun(*run)
	s.resetAgentStatuses(model.ID)
	return s.emitChatState(ctx, projectID, ""), nil
}

func (s *Service) classifyIntentWithModel(
	ctx context.Context,
	provider llm.Provider,
	model llm.ModelConfig,
	currentProject project.Project,
	task chat.Task,
	latestRun *zw.Run,
	content string,
	fallback router.Decision,
) router.Decision {
	s.setAgentStatus(agents.ManagerID, "thinking", "Определяет тип запроса", model.ID)
	classifyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := provider.Generate(classifyCtx, llm.Request{
		Model: model.ModelName,
		Messages: []llm.Message{
			{
				Role: "system",
				Content: strings.TrimSpace(`
Классифицируй сообщение пользователя для локального AI-завода.
Верни строго JSON:
{
  "intent": "direct_answer|project_analysis|coding_task|clarification_answer|workflow_control|pentest_task|research_task|general_chat",
  "confidence": "high|medium|low",
  "reason": "короткая причина",
  "needs_project_context": true,
  "needs_workflow": false,
  "needs_clarification": false
}

Правила:
- coding_task если пользователь просит написать, создать, изменить, исправить, применить или сгенерировать программу, скрипт, файлы или код.
- Фразы "напиши на Go/Python/JS", "сделай скрипт", "создай программу", "реализуй" всегда coding_task, даже если можно ответить примером в чат.
- direct_answer если пользователь спрашивает "что/почему/как/опиши/объясни" по уже существующим данным.
- project_analysis если нужно читать проект, но не менять файлы.
- workflow_control если пользователь управляет текущим workflow.
- pentest_task если запрос про безопасность/пентест/уязвимости.
- research_task если пользователь явно просит найти, проверить или актуализировать информацию в интернете.
- general_chat если вопрос общий и не про проект.
`),
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Проект: %s\nЕсть последний workflow: %t\nСообщение: %s", currentProject.Name, latestRun != nil, content),
			},
		},
		Temperature: 0,
		MaxTokens:   220,
	})
	if err != nil || resp == nil {
		fallback.Reason += "; LLM classification unavailable"
		return fallback
	}
	decision, err := parseIntentDecision(resp.Content)
	if err != nil {
		fallback.Reason += "; LLM classification invalid"
		return fallback
	}
	decision.Source = "llm"
	decision = normalizeIntentDecision(decision)
	if decision.Intent == "" {
		return fallback
	}
	_ = task
	return decision
}

func (s *Service) SubmitClarification(ctx context.Context, input SubmitClarificationInput) (ChatState, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	workflowRunID := strings.TrimSpace(input.WorkflowRunID)
	if projectID == "" || workflowRunID == "" {
		return ChatState{}, fmt.Errorf("project_id и workflow_run_id обязательны")
	}
	answers := normalizeClarificationAnswers(input.Answers)
	if len(answers) == 0 {
		return ChatState{}, fmt.Errorf("нужен хотя бы один ответ")
	}

	task, err := s.store.GetActiveTask(ctx, projectID)
	if err != nil {
		return ChatState{}, err
	}
	if task == nil {
		return ChatState{}, fmt.Errorf("активная задача не найдена")
	}
	run, err := s.store.LatestWorkflowRun(ctx, task.ID)
	if err != nil {
		return ChatState{}, err
	}
	if run == nil || run.ID != workflowRunID || run.Status != zw.StatusWaitingUser {
		return ChatState{}, fmt.Errorf("активного уточнения не найдено")
	}
	return s.SendMessage(ctx, SendMessageInput{
		ProjectID: projectID,
		Content:   clarificationAnswersMessage(answers),
	})
}

func (s *Service) ApplyWorkflowChanges(ctx context.Context, input ApplyWorkflowChangesInput) (ChatState, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	workflowRunID := strings.TrimSpace(input.WorkflowRunID)
	if projectID == "" || workflowRunID == "" {
		return ChatState{}, fmt.Errorf("project_id и workflow_run_id обязательны")
	}

	currentProject, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return ChatState{}, err
	}
	pending, err := s.store.ListPendingProposedChanges(ctx, workflowRunID)
	if err != nil {
		return ChatState{}, err
	}
	if len(pending) == 0 {
		return s.emitChatState(ctx, projectID, ""), nil
	}

	model, modelErr := s.store.ActiveModelConfig(ctx)
	if modelErr == nil {
		s.setAgentStatus(agents.DeveloperID, "writing_files", "Применяет подтвержденные изменения", model.ID)
	}

	applied := 0
	failed := 0
	taskID := pending[0].TaskID
	for _, change := range pending {
		if change.ProjectID != projectID {
			failed++
			_ = s.store.MarkProposedChangeFailed(ctx, change.ID, "изменение относится к другому проекту")
			continue
		}
		result, err := changes.Apply(currentProject.Path, change)
		if err != nil {
			failed++
			_ = s.store.MarkProposedChangeFailed(ctx, change.ID, err.Error())
			continue
		}
		applied++
		_ = s.store.MarkProposedChangeApplied(ctx, change.ID, result.BackupPath, result.BeforeContent, result.AfterContent, result.DiffText)
	}

	if taskID != "" {
		_, _ = s.store.AddMessage(ctx, taskID, "agent", agents.DeveloperID, applyChangesMessage(applied, failed))
		_ = s.store.TouchTask(ctx, taskID)
	}
	if modelErr == nil {
		if failed > 0 {
			s.setAgentStatus(agents.DeveloperID, "failed", "Часть изменений не применена", model.ID)
		} else {
			s.setAgentStatus(agents.DeveloperID, "done", "Изменения применены", model.ID)
		}
	}
	state := s.emitChatState(ctx, projectID, "")
	if applied > 0 && modelErr == nil && taskID != "" {
		go func() {
			if err := s.suggestTestCommands(ctx, currentProject, taskID, workflowRunID, model); err != nil {
				s.setAgentStatus(agents.TesterID, "failed", "Не удалось подготовить проверки: "+err.Error(), model.ID)
				s.emitChatState(ctx, projectID, "")
			}
		}()
	}
	return state, nil
}

type autopilotResult struct {
	AppliedFiles   int
	FailedFiles    int
	TestsPassed    int
	TestsFailed    int
	TestsBlocked   int
	ReviewStatus   string
	ReviewReturnTo string
	ReviewSummary  string
	ReviewFindings []reviews.Finding
	ReviewRequired []string
	Iterations     int
	Blocked        bool
	BlockReason    string
}

func (s *Service) runAutopilot(ctx context.Context, project project.Project, task chat.Task, run *zw.Run, provider llm.Provider, model llm.ModelConfig, workflow *v03WorkflowResult, initialChanges int) autopilotResult {
	result := autopilotResult{}
	if initialChanges == 0 {
		return result
	}

	previousDeveloperOutput := ""
	for iteration := 0; iteration <= maxAutoRepairIterations; iteration++ {
		result.Iterations = iteration + 1
		applied, failed, err := s.applyPendingChangesNow(ctx, project, task, run.ID, model.ID, true)
		result.AppliedFiles += applied
		result.FailedFiles += failed
		if err != nil {
			result.Blocked = true
			result.BlockReason = err.Error()
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, run.CurrentStep, result.BlockReason)
			return result
		}
		if failed > 0 {
			if iteration >= maxAutoRepairIterations {
				result.Blocked = true
				result.BlockReason = "часть изменений не применена, лимит repair-итераций исчерпан"
				_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, run.CurrentStep, result.BlockReason)
				return result
			}
			parsed := reviews.ParsedReview{
				Status:  reviews.StatusNeedsWork,
				Summary: "Часть proposed changes не применена. Исправь ошибки применения: если файл уже существует и его нужно изменить, верни action=replace с полным содержимым файла; action=create используй только для новых файлов.",
				RequiredChanges: []string{
					"Проанализировать ошибки применения в списке изменений.",
					"Вернуть новый блок Proposed changes только для файлов, которые нужно исправить.",
				},
			}
			repairInput := s.buildDeveloperRepairInput(ctx, result, project, run.ID, parsed)
			developer, err := s.runWorkflowStep(ctx, project.ID, task.ID, run, provider, model, zw.StepDeveloperPlan, repairInput)
			if err != nil {
				result.Blocked = true
				result.BlockReason = "repair-итерация разработчика не выполнена: " + err.Error()
				_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, result.BlockReason)
				return result
			}
			if strings.TrimSpace(developer) == strings.TrimSpace(previousDeveloperOutput) {
				result.Blocked = true
				result.BlockReason = "repair-итерация повторила тот же ответ разработчика"
				_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, result.BlockReason)
				return result
			}
			previousDeveloperOutput = developer
			newChanges, err := s.saveProposedChanges(ctx, project, task, run.ID, developer)
			if err != nil {
				result.Blocked = true
				result.BlockReason = "не удалось сохранить repair-изменения: " + err.Error()
				_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, result.BlockReason)
				return result
			}
			if len(newChanges) == 0 {
				result.Blocked = true
				result.BlockReason = "разработчик не вернул structured changes для исправления"
				_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, result.BlockReason)
				return result
			}
			continue
		}
		if err := s.suggestTestCommands(ctx, project, task.ID, run.ID, model); err != nil {
			result.Blocked = true
			result.BlockReason = "не удалось подготовить проверки: " + err.Error()
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepTesterCommands, result.BlockReason)
			return result
		}
		passed, failedTests, blockedTests := s.runPendingTestsNow(ctx, project, run.ID, model.ID)
		result.TestsPassed = passed
		result.TestsFailed = failedTests
		result.TestsBlocked = blockedTests

		parsed, err := s.runReviewNow(ctx, project, task, run, provider, model, iteration+1)
		if err != nil {
			result.Blocked = true
			result.BlockReason = "ревью не выполнено: " + err.Error()
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepReview, result.BlockReason)
			return result
		}
		result.ReviewStatus = parsed.Status
		result.ReviewReturnTo = parsed.ReturnTo
		result.ReviewSummary = strings.TrimSpace(parsed.Summary)
		result.ReviewFindings = parsed.Findings
		result.ReviewRequired = parsed.RequiredChanges
		if parsed.Status == reviews.StatusAccepted {
			return result
		}
		if parsed.Status == reviews.StatusBlocked || parsed.ReturnTo == reviews.ReturnToUser {
			result.Blocked = true
			result.BlockReason = reviewBlockedReason(parsed)
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepReview, result.BlockReason)
			return result
		}
		if iteration >= maxAutoRepairIterations {
			result.Blocked = true
			result.BlockReason = "исчерпан лимит repair-итераций: последнее ревью все еще требует доработку"
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepReview, result.BlockReason)
			return result
		}

		developer, err := s.rerunFromReview(ctx, result, project, task, run, provider, model, workflow, parsed)
		if err != nil {
			result.Blocked = true
			result.BlockReason = err.Error()
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepReview, result.BlockReason)
			return result
		}
		if developer == "" {
			continue
		}
		if strings.TrimSpace(developer) == strings.TrimSpace(previousDeveloperOutput) {
			result.Blocked = true
			result.BlockReason = "repair-итерация повторила тот же ответ разработчика"
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, result.BlockReason)
			return result
		}
		previousDeveloperOutput = developer
		newChanges, err := s.saveProposedChanges(ctx, project, task, run.ID, developer)
		if err != nil {
			result.Blocked = true
			result.BlockReason = "не удалось сохранить repair-изменения: " + err.Error()
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, result.BlockReason)
			return result
		}
		if len(newChanges) == 0 {
			result.Blocked = true
			result.BlockReason = "разработчик не вернул structured changes для исправления"
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepDeveloperPlan, result.BlockReason)
			return result
		}
	}
	return result
}

func (s *Service) rerunFromReview(
	ctx context.Context,
	auto autopilotResult,
	project project.Project,
	task chat.Task,
	run *zw.Run,
	provider llm.Provider,
	model llm.ModelConfig,
	workflow *v03WorkflowResult,
	review reviews.ParsedReview,
) (string, error) {
	switch review.ReturnTo {
	case reviews.ReturnToProduct:
		product, err := s.runWorkflowStep(ctx, project.ID, task.ID, run, provider, model, zw.StepProductRequirements, buildProductRepairInput(*workflow, review))
		if err != nil {
			return "", fmt.Errorf("repair-итерация продакта не выполнена: %w", err)
		}
		workflow.Product = product
		return s.rerunFromBlueprint(ctx, project, task, run, provider, model, workflow, review)
	case reviews.ReturnToArchitect:
		return s.rerunFromBlueprint(ctx, project, task, run, provider, model, workflow, review)
	case reviews.ReturnToTester:
		return "", nil
	default:
		repairInput := s.buildDeveloperRepairInput(ctx, auto, project, run.ID, review)
		developer, err := s.runWorkflowStep(ctx, project.ID, task.ID, run, provider, model, zw.StepDeveloperPlan, repairInput)
		if err != nil {
			return "", fmt.Errorf("repair-итерация разработчика не выполнена: %w", err)
		}
		return developer, nil
	}
}

func (s *Service) rerunFromBlueprint(
	ctx context.Context,
	project project.Project,
	task chat.Task,
	run *zw.Run,
	provider llm.Provider,
	model llm.ModelConfig,
	workflow *v03WorkflowResult,
	review reviews.ParsedReview,
) (string, error) {
	blueprintOutput, err := s.runWorkflowStep(ctx, project.ID, task.ID, run, provider, model, zw.StepTaskBlueprint, buildBlueprintRepairInput(*workflow, project, review))
	if err != nil {
		return "", fmt.Errorf("repair-итерация Task Blueprint не выполнена: %w", err)
	}
	parsedBlueprint, err := blueprint.Parse(blueprintOutput)
	if err != nil {
		return "", fmt.Errorf("repair-итерация Task Blueprint вернула невалидный JSON: %w", err)
	}
	parsedBlueprint = blueprint.NormalizeForProject(parsedBlueprint, project.Path)
	parsedBlueprint.ProjectID = project.ID
	parsedBlueprint.TaskID = task.ID
	parsedBlueprint.WorkflowRunID = run.ID
	savedBlueprint, err := s.store.CreateTaskBlueprint(ctx, parsedBlueprint)
	if err != nil {
		return "", fmt.Errorf("не удалось сохранить repair Task Blueprint: %w", err)
	}
	workflow.Blueprint = savedBlueprint

	architect, err := s.runWorkflowStep(ctx, project.ID, task.ID, run, provider, model, zw.StepArchitectPlan, buildArchitectRepairInput(*workflow, review))
	if err != nil {
		return "", fmt.Errorf("repair-итерация архитектора не выполнена: %w", err)
	}
	workflow.Architect = architect

	developer, err := s.runWorkflowStep(ctx, project.ID, task.ID, run, provider, model, zw.StepDeveloperPlan, buildDeveloperInput(workflow.Intake, workflow.Product, &workflow.Blueprint, workflow.Architect, projectSourceSnapshot(project.Path)))
	if err != nil {
		return "", fmt.Errorf("repair-итерация разработчика не выполнена: %w", err)
	}
	workflow.Developer = developer
	return developer, nil
}

func (s *Service) applyPendingChangesNow(ctx context.Context, project project.Project, task chat.Task, workflowRunID string, modelID string, automatic bool) (int, int, error) {
	pending, err := s.store.ListPendingProposedChanges(ctx, workflowRunID)
	if err != nil {
		return 0, 0, err
	}
	if len(pending) == 0 {
		return 0, 0, nil
	}

	activity := "Автоматически применяет безопасные изменения"
	if !automatic {
		activity = "Применяет подтвержденные изменения"
	}
	s.setAgentStatus(agents.DeveloperID, "writing_files", activity, modelID)

	applied := 0
	failed := 0
	for _, change := range pending {
		if change.ProjectID != project.ID {
			failed++
			_ = s.store.MarkProposedChangeFailed(ctx, change.ID, "изменение относится к другому проекту")
			continue
		}
		result, err := changes.Apply(project.Path, change)
		if err != nil {
			failed++
			_ = s.store.MarkProposedChangeFailed(ctx, change.ID, err.Error())
			continue
		}
		applied++
		_ = s.store.MarkProposedChangeApplied(ctx, change.ID, result.BackupPath, result.BeforeContent, result.AfterContent, result.DiffText)
	}

	if !automatic {
		_, _ = s.store.AddMessage(ctx, task.ID, "agent", agents.DeveloperID, applyChangesMessage(applied, failed))
	}
	_ = s.store.TouchTask(ctx, task.ID)
	if failed > 0 {
		s.setAgentStatus(agents.DeveloperID, "failed", "Часть изменений не применена", modelID)
	} else {
		s.setAgentStatus(agents.DeveloperID, "done", "Изменения применены", modelID)
	}
	s.emitChatState(ctx, project.ID, "")
	return applied, failed, nil
}

func (s *Service) RunTestCommand(ctx context.Context, input RunTestCommandInput) (ChatState, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	testRunID := strings.TrimSpace(input.TestRunID)
	if projectID == "" || testRunID == "" {
		return ChatState{}, fmt.Errorf("project_id и test_run_id обязательны")
	}

	currentProject, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return ChatState{}, err
	}
	testRun, err := s.store.GetTestRun(ctx, testRunID)
	if err != nil {
		return ChatState{}, err
	}
	if testRun.ProjectID != projectID {
		return ChatState{}, fmt.Errorf("проверка относится к другому проекту")
	}

	modelID := ""
	if model, err := s.store.ActiveModelConfig(ctx); err == nil {
		modelID = model.ID
	}
	s.setAgentStatus(agents.TesterID, "running", "Запускает проверку: "+testRun.Command, modelID)
	_ = s.store.StartWorkflowPlanStep(ctx, testRun.WorkflowRunID, zw.StepTesterCommands, agents.TesterID)

	if err := s.store.MarkTestRunRunning(ctx, testRun.ID); err != nil {
		return ChatState{}, err
	}
	s.emitChatState(ctx, projectID, "")

	result := checks.Run(ctx, currentProject.Path, testRun.Command, testRun.WorkingDir)
	if err := s.store.FinishTestRun(ctx, testRun.ID, result); err != nil {
		return ChatState{}, err
	}
	_ = testRun.TaskID
	if result.Status == checks.StatusPassed {
		s.setAgentStatus(agents.TesterID, "done", "Проверка прошла", modelID)
		_ = s.store.FinishWorkflowPlanStep(ctx, testRun.WorkflowRunID, zw.StepTesterCommands, agents.TesterID, zw.StepStatusDone, "")
	} else if result.Status == checks.StatusBlocked {
		s.setAgentStatus(agents.TesterID, "failed", "Команда заблокирована allowlist", modelID)
		_ = s.store.FinishWorkflowPlanStep(ctx, testRun.WorkflowRunID, zw.StepTesterCommands, agents.TesterID, zw.StepStatusFailed, "команда заблокирована allowlist")
	} else {
		s.setAgentStatus(agents.TesterID, "failed", "Проверка завершилась ошибкой", modelID)
		_ = s.store.FinishWorkflowPlanStep(ctx, testRun.WorkflowRunID, zw.StepTesterCommands, agents.TesterID, zw.StepStatusFailed, result.Error)
	}
	return s.emitChatState(ctx, projectID, ""), nil
}

func (s *Service) runPendingTestsNow(ctx context.Context, project project.Project, workflowRunID string, modelID string) (int, int, int) {
	testRuns, err := s.store.ListTestRuns(ctx, project.ID, workflowRunID, 30)
	if err != nil {
		return 0, 0, 0
	}
	passed := 0
	failed := 0
	blocked := 0
	for _, testRun := range testRuns {
		if testRun.Status != checks.StatusPending {
			continue
		}
		_ = s.store.StartWorkflowPlanStep(ctx, workflowRunID, zw.StepTesterCommands, agents.TesterID)
		s.setAgentStatus(agents.TesterID, "running", "Запускает проверку: "+testRun.Command, modelID)
		if err := s.store.MarkTestRunRunning(ctx, testRun.ID); err != nil {
			failed++
			continue
		}
		s.emitChatState(ctx, project.ID, "")
		result := checks.Run(ctx, project.Path, testRun.Command, testRun.WorkingDir)
		if err := s.store.FinishTestRun(ctx, testRun.ID, result); err != nil {
			failed++
			continue
		}
		switch result.Status {
		case checks.StatusPassed:
			passed++
		case checks.StatusBlocked:
			blocked++
		default:
			failed++
		}
		s.emitChatState(ctx, project.ID, "")
	}
	if failed > 0 {
		s.setAgentStatus(agents.TesterID, "failed", "Есть упавшие проверки", modelID)
	} else if blocked > 0 && passed == 0 {
		s.setAgentStatus(agents.TesterID, "done", "Часть проверок не применима", modelID)
	} else {
		s.setAgentStatus(agents.TesterID, "done", "Проверки завершены", modelID)
	}
	if passed+failed+blocked > 0 {
		status := zw.StepStatusDone
		errText := ""
		if failed+blocked > 0 {
			status = zw.StepStatusFailed
			errText = "часть проверок завершилась ошибкой или была заблокирована"
		}
		_ = s.store.FinishWorkflowPlanStep(ctx, workflowRunID, zw.StepTesterCommands, agents.TesterID, status, errText)
	}
	return passed, failed, blocked
}

func latestTestRunsByCommand(items []checks.TestRun) []checks.TestRun {
	latest := make([]checks.TestRun, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		key := strings.TrimSpace(item.WorkingDir) + "\x00" + strings.TrimSpace(item.Command)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		latest = append(latest, item)
	}
	return latest
}

func (s *Service) RunReview(ctx context.Context, input RunReviewInput) (ChatState, error) {
	projectID := strings.TrimSpace(input.ProjectID)
	workflowRunID := strings.TrimSpace(input.WorkflowRunID)
	if projectID == "" || workflowRunID == "" {
		return ChatState{}, fmt.Errorf("project_id и workflow_run_id обязательны")
	}

	currentProject, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return ChatState{}, err
	}
	task, err := s.store.GetActiveTask(ctx, projectID)
	if err != nil {
		return ChatState{}, err
	}
	if task == nil {
		return ChatState{}, fmt.Errorf("активная задача не найдена")
	}
	run, err := s.store.LatestWorkflowRun(ctx, task.ID)
	if err != nil {
		return ChatState{}, err
	}
	if run == nil || run.ID != workflowRunID {
		return ChatState{}, fmt.Errorf("workflow run не найден")
	}

	model, err := s.store.ActiveModelConfig(ctx)
	if err != nil {
		return ChatState{}, err
	}
	provider := openaiapi.NewClient(model.BaseURL, model.APIKeyRef)
	_, err = s.runReviewNow(ctx, currentProject, *task, run, provider, model, 0)
	if err != nil {
		return s.emitChatState(ctx, projectID, err.Error()), nil
	}
	return s.emitChatState(ctx, projectID, ""), nil
}

func (s *Service) runReviewNow(ctx context.Context, currentProject project.Project, task chat.Task, run *zw.Run, provider llm.Provider, model llm.ModelConfig, iteration int) (reviews.ParsedReview, error) {
	reviewRun, err := s.store.CreateReviewRun(ctx, reviews.ReviewRun{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		Status:        reviews.StatusRunning,
		Iteration:     iteration,
	})
	if err != nil {
		return reviews.ParsedReview{}, err
	}

	s.setAgentStatus(agents.ReviewerID, "thinking", "Смотрит diff и результаты проверок", model.ID)
	s.emitChatState(ctx, currentProject.ID, "")

	output, stepErr := s.runWorkflowStep(ctx, currentProject.ID, task.ID, run, provider, model, zw.StepReview, s.buildReviewInput(ctx, currentProject, run.ID))
	if stepErr != nil {
		_ = s.store.FinishReviewRun(ctx, reviewRun.ID, reviews.ParsedReview{}, stepErr.Error())
		s.setAgentStatus(agents.ReviewerID, "failed", "Ревью не выполнено", model.ID)
		return reviews.ParsedReview{}, stepErr
	}

	parsed, parseErr := reviews.Parse(output)
	if parseErr != nil {
		_ = s.store.FinishReviewRun(ctx, reviewRun.ID, reviews.ParsedReview{}, parseErr.Error())
		s.setAgentStatus(agents.ReviewerID, "failed", "Ревьюер вернул невалидный JSON", model.ID)
		return reviews.ParsedReview{}, parseErr
	}

	if err := s.store.FinishReviewRun(ctx, reviewRun.ID, parsed, ""); err != nil {
		return reviews.ParsedReview{}, err
	}
	if parsed.Status == reviews.StatusNeedsWork {
		s.setAgentStatus(agents.ReviewerID, "done", "Нашел замечания", model.ID)
		s.setAgentStatus(reviewReturnAgentID(parsed.ReturnTo), "needs_work", reviewReturnActivity(parsed.ReturnTo), model.ID)
	} else if parsed.Status == reviews.StatusBlocked {
		s.setAgentStatus(agents.ReviewerID, "failed", "Остановил workflow", model.ID)
	} else {
		s.setAgentStatus(agents.ReviewerID, "done", "Принял работу", model.ID)
	}
	s.emitChatState(ctx, currentProject.ID, "")
	return parsed, nil
}

func (s *Service) SaveModelConfig(ctx context.Context, input SaveModelConfigInput) ([]ModelConfigDTO, error) {
	model := llm.ModelConfig{
		ID:        strings.TrimSpace(input.ID),
		Name:      strings.TrimSpace(input.Name),
		Provider:  strings.TrimSpace(input.Provider),
		BaseURL:   strings.TrimSpace(input.BaseURL),
		APIKeyRef: strings.TrimSpace(input.APIKeyRef),
		ModelName: strings.TrimSpace(input.ModelName),
		IsActive:  input.IsActive,
	}
	if model.Name == "" {
		return nil, fmt.Errorf("название модели пустое")
	}
	if model.Provider == "" {
		model.Provider = "openai-compatible"
	}
	if model.BaseURL == "" {
		return nil, fmt.Errorf("base_url модели пустой")
	}
	if model.ModelName == "" {
		return nil, fmt.Errorf("model_name модели пустой")
	}

	saved, err := s.store.SaveModelConfig(ctx, model)
	if err != nil {
		return nil, err
	}
	if input.IsActive {
		if err := s.store.SetActiveModel(ctx, saved.ID); err != nil {
			return nil, err
		}
	}
	models, err := s.store.ListModelConfigs(ctx)
	if err != nil {
		return nil, err
	}
	s.emitModels(models)
	return models, nil
}

func (s *Service) SetActiveModel(ctx context.Context, modelID string) ([]ModelConfigDTO, error) {
	if err := s.store.SetActiveModel(ctx, modelID); err != nil {
		return nil, err
	}
	model, err := s.store.ActiveModelConfig(ctx)
	if err != nil {
		return nil, err
	}
	s.resetAgentStatuses(model.ID)
	models, err := s.store.ListModelConfigs(ctx)
	if err != nil {
		return nil, err
	}
	s.emitModels(models)
	return models, nil
}

func (s *Service) CheckModel(ctx context.Context, modelID string) ([]ModelConfigDTO, error) {
	model, err := s.store.GetModelConfig(ctx, modelID)
	if err != nil {
		return nil, err
	}

	model.Status = "checking"
	models, err := s.store.ListModelConfigs(ctx)
	if err == nil {
		for index := range models {
			if models[index].ID == modelID {
				models[index].Status = "checking"
				models[index].LastError = ""
			}
		}
		s.emitModels(models)
	}

	client := openaiapi.NewClient(model.BaseURL, model.APIKeyRef)
	result := client.Check(ctx, model.ModelName)
	result.ModelID = model.ID
	if err := s.store.UpdateModelCheck(ctx, result); err != nil {
		return nil, err
	}

	models, err = s.store.ListModelConfigs(ctx)
	if err != nil {
		return nil, err
	}
	s.emitModels(models)
	return models, nil
}

func (s *Service) SaveWebSettings(ctx context.Context, input SaveWebSettingsInput) (WebSettingsDTO, error) {
	settings := webresearch.NormalizeSettings(webresearch.Settings{
		Enabled:             input.Enabled,
		MaxResults:          input.MaxResults,
		MaxPagesPerWorkflow: input.MaxPagesPerWorkflow,
		TimeoutSeconds:      input.TimeoutSeconds,
		AllowedDomains:      input.AllowedDomains,
		BlockedDomains:      input.BlockedDomains,
	})
	data, err := json.Marshal(settings)
	if err != nil {
		return WebSettingsDTO{}, err
	}
	if err := s.store.SetSetting(ctx, webSettingsKey, string(data)); err != nil {
		return WebSettingsDTO{}, err
	}
	return settings, nil
}

func (s *Service) webSettings(ctx context.Context) webresearch.Settings {
	raw, ok, err := s.store.GetSetting(ctx, webSettingsKey)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return webresearch.DefaultSettings()
	}
	var settings webresearch.Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return webresearch.DefaultSettings()
	}
	return webresearch.NormalizeSettings(settings)
}

func (s *Service) startModelHealthMonitor(ctx context.Context) {
	go func() {
		stableOnlineChecks := 0
		interval := time.Second
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}

			model, err := s.store.ActiveModelConfig(ctx)
			if err != nil || strings.TrimSpace(model.ID) == "" {
				interval = modelHealthBaseInterval
				continue
			}

			checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			result := openaiapi.NewClient(model.BaseURL, model.APIKeyRef).Check(checkCtx, model.ModelName)
			cancel()
			result.ModelID = model.ID

			previousStatus := model.Status
			if err := s.store.UpdateModelCheck(ctx, result); err != nil {
				interval = modelHealthBaseInterval
				continue
			}
			models, err := s.store.ListModelConfigs(ctx)
			if err == nil {
				s.emitModels(models)
			}

			if result.Status == "online" {
				stableOnlineChecks++
				if stableOnlineChecks >= modelHealthStableChecks {
					interval = modelHealthSlowInterval
				} else {
					interval = modelHealthBaseInterval
				}
				if previousStatus == "offline" {
					s.setAgentStatus(agents.ManagerID, "idle", "Модель снова доступна", model.ID)
				}
				continue
			}

			stableOnlineChecks = 0
			interval = modelHealthFastInterval
			if previousStatus != "offline" {
				s.setAgentStatus(agents.ManagerID, "failed", "Активная модель недоступна", model.ID)
			}
		}
	}()
}

type v03WorkflowResult struct {
	Intake             string
	Clarification      string
	Product            string
	Blueprint          blueprint.Blueprint
	Architect          string
	Developer          string
	Final              string
	NeedsClarification bool
}

func (s *Service) runV03Workflow(
	ctx context.Context,
	project project.Project,
	taskID string,
	run *zw.Run,
	history []chat.Message,
	provider llm.Provider,
	model llm.ModelConfig,
) (v03WorkflowResult, error) {
	projectID := project.ID
	result := v03WorkflowResult{}

	intakeInput := buildManagerIntakeInput(history)
	intake, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepManagerIntake, intakeInput)
	if err != nil {
		return result, err
	}
	result.Intake = intake
	intakeResult, hasStructuredIntake := parseManagerIntake(intake)
	if managerNeedsClarification(intake) {
		result.NeedsClarification = true
		if hasStructuredIntake {
			result.Clarification = intake
			_ = intakeResult
		} else {
			result.Clarification = intake
		}
		return result, nil
	}

	productInput := buildProductInput(intake)
	product, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepProductRequirements, productInput)
	if err != nil {
		return result, err
	}
	result.Product = product

	blueprintInput := buildBlueprintInput(intake, product, projectCheckSignals(project.Path))
	blueprintOutput, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepTaskBlueprint, blueprintInput)
	if err != nil {
		return result, err
	}
	parsedBlueprint, err := blueprint.Parse(blueprintOutput)
	if err != nil {
		return result, err
	}
	parsedBlueprint = blueprint.NormalizeForProject(parsedBlueprint, project.Path)
	parsedBlueprint.ProjectID = projectID
	parsedBlueprint.TaskID = taskID
	parsedBlueprint.WorkflowRunID = run.ID
	savedBlueprint, err := s.store.CreateTaskBlueprint(ctx, parsedBlueprint)
	if err != nil {
		return result, err
	}
	result.Blueprint = savedBlueprint

	architectInput := buildArchitectInput(intake, product, &savedBlueprint)
	architect, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepArchitectPlan, architectInput)
	if err != nil {
		return result, err
	}
	result.Architect = architect

	developerInput := buildDeveloperInput(intake, product, &savedBlueprint, architect, projectSourceSnapshot(project.Path))
	developer, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepDeveloperPlan, developerInput)
	if err != nil {
		return result, err
	}
	result.Developer = developer

	return result, nil
}

func (s *Service) runWorkflowStep(
	ctx context.Context,
	projectID string,
	taskID string,
	run *zw.Run,
	provider llm.Provider,
	model llm.ModelConfig,
	stepKey string,
	input string,
) (string, error) {
	spec := agents.SpecForStep(stepKey)
	_ = s.store.StartWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID)
	run.CurrentStep = stepKey
	run.Status = zw.StatusRunning
	if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusRunning, stepKey, ""); err != nil {
		return "", err
	}
	s.emitWorkflowRun(*run)

	step, err := s.store.CreateWorkflowStep(ctx, run.ID, stepKey, spec.ID, input)
	if err != nil {
		return "", err
	}
	s.emitWorkflowStep(*run, step)
	s.emitChatState(ctx, projectID, "")

	s.setAgentStatus(spec.ID, "thinking", stepThinkingActivity(stepKey), model.ID)
	s.setAgentStatus(spec.ID, "calling_model", "Отправляет шаг в "+model.Name, model.ID)

	stepCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := provider.Generate(stepCtx, agents.RequestForSpec(model.ModelName, spec, input))
	if err != nil {
		failed, finishErr := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusFailed, "", err.Error())
		if finishErr == nil {
			s.emitWorkflowStep(*run, failed)
		}
		_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID, zw.StepStatusFailed, err.Error())
		s.setAgentStatus(spec.ID, "failed", "Ошибка вызова модели", model.ID)
		return "", err
	}

	output := strings.TrimSpace(resp.Content)
	if output == "" {
		err := errors.New("model api вернул пустой ответ")
		failed, finishErr := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusFailed, "", err.Error())
		if finishErr == nil {
			s.emitWorkflowStep(*run, failed)
		}
		_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID, zw.StepStatusFailed, err.Error())
		s.setAgentStatus(spec.ID, "failed", "Пустой ответ модели", model.ID)
		return "", err
	}
	if len(output) > managerMaxAnswerBytes || looksLikeRepetitionLoop(output) {
		err := errors.New("ответ остановлен: модель начала повторять или генерировать слишком длинный текст")
		failed, finishErr := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusFailed, output, err.Error())
		if finishErr == nil {
			s.emitWorkflowStep(*run, failed)
		}
		_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID, zw.StepStatusFailed, err.Error())
		s.setAgentStatus(spec.ID, "failed", "Ответ модели остановлен", model.ID)
		return "", err
	}

	s.setAgentStatus(spec.ID, "answering", "Сохраняет результат шага", model.ID)
	done, err := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusDone, output, "")
	if err != nil {
		s.setAgentStatus(spec.ID, "failed", "Не удалось сохранить шаг", model.ID)
		return "", err
	}
	s.emitWorkflowStep(*run, done)
	_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID, zw.StepStatusDone, "")
	s.setAgentStatus(spec.ID, "done", stepDoneActivity(stepKey), model.ID)
	s.emitChatState(ctx, projectID, "")
	_ = taskID
	return output, nil
}

func (s *Service) AgentStatuses() []agents.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := []string{agents.ManagerID, agents.ProductID, agents.ArchitectID, agents.DeveloperID, agents.TesterID, agents.ReviewerID}
	out := make([]agents.Status, 0, len(order))
	for _, id := range order {
		if status, ok := s.agentStatuses[id]; ok {
			out = append(out, status)
		}
	}
	return out
}

func (s *Service) projectState(ctx context.Context, projectID string) (ProjectState, error) {
	item, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return ProjectState{}, err
	}
	task, err := s.store.GetActiveTask(ctx, projectID)
	if err != nil {
		return ProjectState{}, err
	}
	messages := []chat.Message{}
	var workflowRun *zw.Run
	workflowSteps := []zw.Step{}
	var workflowPlan *zw.Plan
	planSteps := []zw.PlanStep{}
	artifactsList := []artifacts.Artifact{}
	var taskBlueprint *blueprint.Blueprint
	var clarification *ClarificationDTO
	proposedChanges := []changes.ProposedChange{}
	testRuns := []checks.TestRun{}
	reviewRuns := []reviews.ReviewRun{}
	webSources := []webresearch.Source{}
	if task != nil {
		messages, err = s.store.ListMessages(ctx, task.ID)
		if err != nil {
			return ProjectState{}, err
		}
		workflowRun, err = s.store.LatestWorkflowRun(ctx, task.ID)
		if err != nil {
			return ProjectState{}, err
		}
		if workflowRun != nil {
			workflowSteps, err = s.store.ListWorkflowSteps(ctx, workflowRun.ID)
			if err != nil {
				return ProjectState{}, err
			}
			workflowPlan, planSteps, err = s.store.LatestWorkflowPlan(ctx, workflowRun.ID)
			if err != nil {
				return ProjectState{}, err
			}
			clarification = pendingClarificationFromSteps(workflowRun, workflowSteps)
			proposedChanges, err = s.store.ListProposedChanges(ctx, projectID, workflowRun.ID, 30)
			if err != nil {
				return ProjectState{}, err
			}
			taskBlueprint, err = s.store.LatestTaskBlueprint(ctx, workflowRun.ID)
			if err != nil {
				return ProjectState{}, err
			}
			testRuns, err = s.store.ListTestRuns(ctx, projectID, workflowRun.ID, 30)
			if err != nil {
				return ProjectState{}, err
			}
			testRuns = s.refreshUnsupportedTestRuns(ctx, item, testRuns)
			reviewRuns, err = s.store.ListReviewRuns(ctx, projectID, workflowRun.ID, 10)
			if err != nil {
				return ProjectState{}, err
			}
			webSources, err = s.store.ListWebSources(ctx, projectID, workflowRun.ID, 30)
			if err != nil {
				return ProjectState{}, err
			}
		}
	}
	artifactsList, err = s.store.ListArtifacts(ctx, projectID, 20)
	if err != nil {
		return ProjectState{}, err
	}
	return ProjectState{
		Project:       item,
		Task:          task,
		Messages:      messages,
		WorkflowRun:   workflowRun,
		WorkflowSteps: workflowSteps,
		WorkflowPlan:  workflowPlan,
		PlanSteps:     planSteps,
		Artifacts:     artifactsList,
		Blueprint:     taskBlueprint,
		Clarification: clarification,
		Changes:       proposedChanges,
		TestRuns:      testRuns,
		Reviews:       reviewRuns,
		WebSources:    webSources,
	}, nil
}

func (s *Service) suggestTestCommands(ctx context.Context, project project.Project, taskID string, workflowRunID string, model llm.ModelConfig) error {
	run, err := s.store.LatestWorkflowRun(ctx, taskID)
	if err != nil {
		return err
	}
	if run == nil || run.ID != workflowRunID {
		return fmt.Errorf("workflow run не найден")
	}

	provider := openaiapi.NewClient(model.BaseURL, model.APIKeyRef)
	input := s.buildTesterInput(ctx, project, workflowRunID)
	output, err := s.runWorkflowStep(ctx, project.ID, taskID, run, provider, model, zw.StepTesterCommands, input)

	blueprintSuggestions := s.blueprintTestSuggestions(ctx, workflowRunID)
	suggestions := blueprintSuggestions
	if len(suggestions) == 0 {
		suggestions = checks.ExtractSuggestions(output)
	}
	if err != nil {
		if len(blueprintSuggestions) > 0 {
			suggestions = blueprintSuggestions
		} else {
			suggestions = nil
		}
	}
	appliedChanges := s.appliedWorkflowChanges(ctx, project.ID, workflowRunID)
	suggestions = checks.FilterSupportedSuggestions(project.Path, suggestions)
	suggestions = filterRelevantTestSuggestions(suggestions, appliedChanges)
	if len(suggestions) == 0 {
		defaultSuggestions := checks.FilterSupportedSuggestions(project.Path, checks.DefaultSuggestions(project.Path))
		suggestions = filterRelevantTestSuggestions(defaultSuggestions, appliedChanges)
	}

	created := make([]checks.TestRun, 0, len(suggestions))
	for _, suggestion := range suggestions {
		status := checks.StatusPending
		errText := ""
		if err := checks.ValidateCommand(project.Path, suggestion.Command, suggestion.WorkingDir); err != nil {
			status = checks.StatusBlocked
			errText = err.Error()
		}
		testRun, createErr := s.store.CreateTestRun(ctx, checks.TestRun{
			ProjectID:     project.ID,
			TaskID:        taskID,
			WorkflowRunID: workflowRunID,
			Command:       suggestion.Command,
			WorkingDir:    suggestion.WorkingDir,
			Reason:        suggestion.Reason,
			Status:        status,
			ExitCode:      -1,
			Error:         errText,
		})
		if createErr != nil {
			return createErr
		}
		created = append(created, testRun)
	}

	if updateErr := s.store.UpdateWorkflowRun(ctx, workflowRunID, zw.StatusDone, zw.StepTesterCommands, ""); updateErr == nil {
		run.Status = zw.StatusDone
		run.CurrentStep = zw.StepTesterCommands
		run.FinishedAt = nowString()
		s.emitWorkflowRun(*run)
	}
	s.setAgentStatus(agents.TesterID, "done", "Подготовил команды проверки", model.ID)
	return nil
}

func (s *Service) appliedWorkflowChanges(ctx context.Context, projectID string, workflowRunID string) []changes.ProposedChange {
	items, err := s.store.ListProposedChanges(ctx, projectID, workflowRunID, 50)
	if err != nil {
		return nil
	}
	out := make([]changes.ProposedChange, 0, len(items))
	for _, item := range items {
		if item.Status == changes.StatusApplied {
			out = append(out, item)
		}
	}
	return out
}

func filterRelevantTestSuggestions(suggestions []checks.Suggestion, applied []changes.ProposedChange) []checks.Suggestion {
	if len(applied) == 0 {
		return suggestions
	}
	changedKinds := changedFileKinds(applied)
	filtered := make([]checks.Suggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		if testSuggestionMatchesKinds(suggestion, changedKinds) {
			filtered = append(filtered, suggestion)
		}
	}
	return filtered
}

func changedFileKinds(applied []changes.ProposedChange) map[string]bool {
	kinds := map[string]bool{}
	for _, change := range applied {
		path := strings.TrimSpace(change.FilePath)
		base := filepath.Base(path)
		ext := strings.ToLower(filepath.Ext(path))
		switch {
		case ext == ".go" || base == "go.mod" || base == "go.sum":
			kinds["go"] = true
		case ext == ".py" || base == "pyproject.toml" || strings.HasPrefix(base, "requirements"):
			kinds["python"] = true
		case base == "package.json" || base == "package-lock.json" || base == "pnpm-lock.yaml" || base == "yarn.lock" ||
			ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".css":
			kinds["npm"] = true
		default:
			kinds["generic"] = true
		}
	}
	return kinds
}

func testSuggestionMatchesKinds(suggestion checks.Suggestion, kinds map[string]bool) bool {
	args := strings.Fields(strings.TrimSpace(suggestion.Command))
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "go":
		return kinds["go"]
	case "npm":
		return kinds["npm"]
	case "python", "python3":
		return kinds["python"]
	default:
		return kinds["generic"]
	}
}

func (s *Service) blueprintTestSuggestions(ctx context.Context, workflowRunID string) []checks.Suggestion {
	taskBlueprint, err := s.store.LatestTaskBlueprint(ctx, workflowRunID)
	if err != nil || taskBlueprint == nil {
		return nil
	}
	suggestions := make([]checks.Suggestion, 0, len(taskBlueprint.TestCommands))
	for _, command := range taskBlueprint.TestCommands {
		suggestions = append(suggestions, checks.Suggestion{
			Command:    command.Command,
			WorkingDir: command.WorkingDir,
			Reason:     command.Reason,
		})
	}
	return suggestions
}

func (s *Service) refreshUnsupportedTestRuns(ctx context.Context, project project.Project, testRuns []checks.TestRun) []checks.TestRun {
	for i := range testRuns {
		if testRuns[i].Status == checks.StatusPassed {
			continue
		}
		err := checks.ValidateCommand(project.Path, testRuns[i].Command, testRuns[i].WorkingDir)
		if err == nil {
			continue
		}
		testRuns[i].Status = checks.StatusBlocked
		testRuns[i].ExitCode = -1
		testRuns[i].Stdout = ""
		testRuns[i].Stderr = ""
		testRuns[i].Error = err.Error()
		if updateErr := s.store.FinishTestRun(ctx, testRuns[i].ID, checks.RunResult{
			Status:   checks.StatusBlocked,
			ExitCode: -1,
			Error:    err.Error(),
		}); updateErr != nil {
			continue
		}
	}
	return testRuns
}

func (s *Service) saveWorkflowArtifacts(ctx context.Context, project project.Project, task chat.Task, runID string, result v03WorkflowResult) ([]artifacts.Artifact, error) {
	files, err := artifacts.WriteWorkflowOutput(artifacts.WorkflowOutput{
		ProjectPath:   project.Path,
		TaskTitle:     task.Title,
		WorkflowRunID: runID,
		Intake:        result.Intake,
		Product:       result.Product,
		Blueprint:     blueprint.ToPrompt(&result.Blueprint),
		Architect:     result.Architect,
		Developer:     result.Developer,
		Final:         result.Final,
		CreatedAt:     time.Now(),
	})
	if err != nil {
		return nil, err
	}

	saved := make([]artifacts.Artifact, 0, len(files))
	for _, file := range files {
		artifact, err := s.store.CreateArtifact(ctx, artifacts.Artifact{
			ProjectID:     project.ID,
			TaskID:        task.ID,
			WorkflowRunID: runID,
			AgentID:       file.AgentID,
			Kind:          file.Kind,
			Title:         file.Title,
			Path:          file.Path,
			RelativePath:  file.RelativePath,
		})
		if err != nil {
			return nil, err
		}
		saved = append(saved, artifact)
	}
	return saved, nil
}

func (s *Service) saveProposedChanges(ctx context.Context, project project.Project, task chat.Task, runID string, developerOutput string) ([]changes.ProposedChange, error) {
	drafts, err := changes.ExtractDraftsWithError(developerOutput)
	if err != nil {
		return nil, err
	}
	drafts = s.ensureBlueprintRequiredDrafts(ctx, project, runID, drafts)
	saved := make([]changes.ProposedChange, 0, len(drafts))
	for _, draft := range drafts {
		change, err := s.store.CreateProposedChange(ctx, changes.ProposedChange{
			ProjectID:     project.ID,
			TaskID:        task.ID,
			WorkflowRunID: runID,
			AgentID:       agents.DeveloperID,
			FilePath:      draft.FilePath,
			Action:        draft.Action,
			Content:       draft.Content,
			Reason:        draft.Reason,
			Status:        changes.StatusPending,
		})
		if err != nil {
			return nil, err
		}
		saved = append(saved, change)
	}
	return saved, nil
}

func (s *Service) ensureBlueprintRequiredDrafts(ctx context.Context, project project.Project, runID string, drafts []changes.Draft) []changes.Draft {
	taskBlueprint, err := s.store.LatestTaskBlueprint(ctx, runID)
	if err != nil || taskBlueprint == nil || taskBlueprint.Stack != blueprint.StackPython {
		return drafts
	}
	if hasDraftPath(drafts, "requirements.txt") || !blueprintExpectedPath(taskBlueprint.ExpectedFiles, "requirements.txt") {
		return drafts
	}
	content := pythonRequirementsContent(taskBlueprint.Dependencies.Items)
	return append(drafts, changes.Draft{
		FilePath: "requirements.txt",
		Action:   actionForProjectFile(project.Path, "requirements.txt"),
		Content:  content,
		Reason:   "Python dependencies for project virtualenv",
	})
}

func hasDraftPath(drafts []changes.Draft, path string) bool {
	path = filepath.ToSlash(strings.Trim(path, "/"))
	for _, draft := range drafts {
		if filepath.ToSlash(strings.Trim(draft.FilePath, "/")) == path {
			return true
		}
	}
	return false
}

func blueprintExpectedPath(items []blueprint.ExpectedFile, path string) bool {
	path = filepath.ToSlash(strings.Trim(path, "/"))
	for _, item := range items {
		if filepath.ToSlash(strings.Trim(item.Path, "/")) == path {
			return true
		}
	}
	return false
}

func pythonRequirementsContent(items []string) string {
	var lines []string
	seen := map[string]struct{}{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		lines = append(lines, item)
	}
	if len(lines) == 0 {
		return "# standard library only\n"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func actionForProjectFile(projectPath string, relativePath string) string {
	if fileExists(filepath.Join(projectPath, relativePath)) {
		return changes.ActionReplace
	}
	return changes.ActionCreate
}

func (s *Service) emitChatState(ctx context.Context, projectID string, errorText string) ChatState {
	state, err := s.projectState(ctx, projectID)
	if err != nil {
		errorText = err.Error()
	}
	result := ChatState{
		ProjectState: state,
		Agents:       s.AgentStatuses(),
		Error:        errorText,
	}
	if s.sink != nil {
		s.sink.Emit("chat_state_changed", result)
	}
	return result
}

func (s *Service) resetAgentStatuses(modelID string) {
	s.mu.Lock()
	s.agentStatuses = map[string]agents.Status{}
	statuses := agents.IdleStatuses(modelID, nowString())
	for _, status := range statuses {
		s.agentStatuses[status.ID] = status
	}
	s.mu.Unlock()

	for _, status := range statuses {
		if s.sink != nil {
			s.sink.Emit("agent_status_changed", status)
		}
	}
}

func (s *Service) setAgentStatus(agentID string, status string, activity string, modelID string) {
	role, name := agents.Describe(agentID)
	value := agents.Status{
		ID:        agentID,
		Role:      role,
		Name:      name,
		Status:    status,
		Activity:  activity,
		ModelID:   modelID,
		UpdatedAt: nowString(),
	}

	s.mu.Lock()
	s.agentStatuses[agentID] = value
	s.mu.Unlock()

	if s.sink != nil {
		s.sink.Emit("agent_status_changed", value)
	}
}

func (s *Service) handleModelError(ctx context.Context, projectID string, taskID string, runID string, modelID string, err error) ChatState {
	errorText := "Не получилось вызвать модель: " + err.Error() + "\n\nПроверь base_url, model и ключ API в настройках модели."
	_, _ = s.store.AddMessage(ctx, taskID, "agent", agents.ManagerID, errorText)
	_ = s.store.FinishAgentRun(ctx, runID, "failed", err.Error())
	s.setAgentStatus(agents.ManagerID, "failed", "Ошибка вызова модели", modelID)
	s.emitAgentMessageDelta(taskID, agents.ManagerID, "", true, err.Error())
	return s.emitChatState(ctx, projectID, err.Error())
}

func (s *Service) handleWorkflowError(ctx context.Context, projectID string, taskID string, runID string, currentStep string, modelID string, err error) ChatState {
	errorText := "Workflow остановлен: " + err.Error() + "\n\nПроверь настройки модели или уточни задачу и попробуй снова."
	_, _ = s.store.AddMessage(ctx, taskID, "agent", agents.ManagerID, errorText)
	_ = s.store.UpdateWorkflowRun(ctx, runID, zw.StatusFailed, currentStep, err.Error())
	_ = s.store.FinishWorkflowPlan(ctx, runID, zw.StatusFailed, err.Error())
	s.setAgentStatus(agents.ManagerID, "failed", "Workflow остановлен", modelID)
	return s.emitChatState(ctx, projectID, err.Error())
}

func (s *Service) emitAgentMessageDelta(taskID string, agentID string, delta string, done bool, errorText string) {
	if s.sink == nil {
		return
	}
	s.sink.Emit("agent_message_delta", AgentMessageDelta{
		TaskID:  taskID,
		AgentID: agentID,
		Delta:   delta,
		Done:    done,
		Error:   errorText,
	})
}

func (s *Service) emitWorkflowRun(run zw.Run) {
	if s.sink == nil {
		return
	}
	s.sink.Emit("workflow_run_changed", WorkflowRunChanged{Run: run})
}

func (s *Service) emitWorkflowStep(run zw.Run, step zw.Step) {
	if s.sink == nil {
		return
	}
	s.sink.Emit("workflow_step_changed", WorkflowStepChanged{
		Run:  run,
		Step: step,
	})
}

func (s *Service) emitModels(models []llm.ModelConfig) {
	if s.sink == nil {
		return
	}
	s.sink.Emit("models_changed", models)
}

func (s *Service) ensureDefaultProject(ctx context.Context) error {
	count, err := s.store.CountProjects(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	path := uniqueProjectPath(s.paths.ProjectsDir, "project-1")
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	item, err := s.store.CreateProject(ctx, "Первый проект", path)
	if err != nil {
		return err
	}
	return s.store.SetSetting(ctx, selectedProjectSetting, item.ID)
}

func safeProjectDirName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "project"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}

	result := strings.Trim(builder.String(), "-")
	if result == "" {
		return "project"
	}
	return result
}

func uniqueProjectPath(root string, name string) string {
	base := filepath.Join(root, name)
	path := base
	for i := 2; ; i++ {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path
		}
		path = fmt.Sprintf("%s-%d", base, i)
	}
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("путь проекта пустой")
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return filepath.Abs(path)
}

func titleFromContent(content string) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) == 0 {
		return "Новая задача"
	}
	if len(runes) > 80 {
		runes = runes[:80]
	}
	title := strings.TrimSpace(string(runes))
	if title == "" {
		return "Новая задача"
	}
	return title
}

func buildManagerIntakeInput(history []chat.Message) string {
	var builder strings.Builder
	builder.WriteString("История задачи:\n")
	for _, message := range recentMessages(history, 8) {
		role := "Пользователь"
		if message.Role == "agent" {
			role = "Ассистент"
		}
		builder.WriteString("\n")
		builder.WriteString(role)
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(message.Content))
		builder.WriteString("\n")
	}
	builder.WriteString("\nИнструкция: если последнее сообщение пользователя отвечает на предыдущие уточняющие вопросы, объедини эти ответы с исходной постановкой и не задавай уже отвеченные вопросы повторно.\n")
	return builder.String()
}

func buildProductInput(intake string) string {
	return strings.TrimSpace(`
Task brief от Люмен:
` + intake + `

Сделай требования для реализации. Не добавляй технический план глубже продуктового уровня.
`)
}

func buildProductRepairInput(result v03WorkflowResult, review reviews.ParsedReview) string {
	return strings.TrimSpace(`
Это repair-итерация после обязательного ревью.

Task brief от Люмен:
` + result.Intake + `

Предыдущие требования:
` + shortenForPrompt(result.Product, 1400) + `

Замечания Ревьюера:
` + reviewFeedbackForPrompt(review) + `

Пересобери требования так, чтобы нижние роли могли исправить задачу без дополнительных вопросов. Не добавляй технический план глубже продуктового уровня.
`)
}

func buildBlueprintInput(intake string, product string, projectSignals string) string {
	return strings.TrimSpace(`
Task brief от Люмен:
` + intake + `

Требования от Продакта:
` + product + `

` + projectSignals + `

Создай Task Blueprint. Он должен зафиксировать стек, scaffold, ожидаемые файлы, запрещенные файлы и команды проверки.
Если пользователь уже явно указал язык или файл, это важнее общих предположений.
Если выбран stack=go, runtime должен быть строго "Go 1.25+".
`)
}

func buildBlueprintRepairInput(result v03WorkflowResult, project project.Project, review reviews.ParsedReview) string {
	return strings.TrimSpace(`
Это repair-итерация после обязательного ревью.

Task brief от Люмен:
` + result.Intake + `

Требования от Продакта:
` + result.Product + `

Предыдущий Task Blueprint:
` + blueprint.ToPrompt(&result.Blueprint) + `

` + projectCheckSignals(project.Path) + `

Замечания Ревьюера:
` + reviewFeedbackForPrompt(review) + `

Создай исправленный Task Blueprint. Он должен зафиксировать стек, scaffold, ожидаемые файлы, запрещенные файлы и команды проверки.
Если пользователь уже явно указал язык или файл, это важнее общих предположений.
Если выбран stack=go, runtime должен быть строго "Go 1.25+".
`)
}

func buildArchitectInput(intake string, product string, taskBlueprint *blueprint.Blueprint) string {
	return strings.TrimSpace(`
Task brief от Люмен:
` + intake + `

Требования от Продакта:
` + product + `

Task Blueprint:
` + blueprint.ToPrompt(taskBlueprint) + `

Сделай технический план реализации строго внутри Task Blueprint. Не выбирай другой стек и не утверждай, что файлы проекта уже прочитаны или изменены.
`)
}

func buildArchitectRepairInput(result v03WorkflowResult, review reviews.ParsedReview) string {
	return strings.TrimSpace(`
Это repair-итерация после обязательного ревью.

Task brief от Люмен:
` + result.Intake + `

Требования от Продакта:
` + result.Product + `

Исправленный Task Blueprint:
` + blueprint.ToPrompt(&result.Blueprint) + `

Предыдущий архитектурный план:
` + shortenForPrompt(result.Architect, 1400) + `

Замечания Ревьюера:
` + reviewFeedbackForPrompt(review) + `

Сделай исправленный технический план реализации строго внутри Task Blueprint. Не выбирай другой стек и не утверждай, что файлы проекта уже изменены.
`)
}

func buildSecurityAnalysisInput(userMessage string, project project.Project, history []chat.Message) string {
	var builder strings.Builder
	builder.WriteString("# User security request\n")
	builder.WriteString(userMessage)
	builder.WriteString("\n\n# Project\n")
	builder.WriteString(fmt.Sprintf("- name: %s\n- path: %s\n", project.Name, project.Path))
	builder.WriteString("\n# Project signals\n")
	builder.WriteString(projectCheckSignals(project.Path))
	if len(history) > 0 {
		builder.WriteString("\n# Recent chat\n")
		for _, item := range recentMessages(history, 8) {
			role := item.Role
			if item.AgentID != "" {
				role = item.AgentID
			}
			builder.WriteString("- ")
			builder.WriteString(role)
			builder.WriteString(": ")
			builder.WriteString(shortenForPrompt(item.Content, 360))
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n# Source snapshot\n")
	builder.WriteString(projectSourceSnapshot(project.Path))
	builder.WriteString("\n\nСделай защитный ИБ-анализ по доступному контексту. Если для активного пентеста не хватает scope или подтверждения разрешения, явно попроси это как следующий шаг.")
	return strings.TrimSpace(builder.String())
}

func buildWebResearchPlanInput(userMessage string, project project.Project, history []chat.Message) string {
	var builder strings.Builder
	builder.WriteString("Проект: ")
	builder.WriteString(project.Name)
	builder.WriteString("\nЛокальный путь проекта не отправляй в интернет: ")
	builder.WriteString(project.Path)
	builder.WriteString("\n\nЗапрос пользователя:\n")
	builder.WriteString(userMessage)
	if len(history) > 0 {
		builder.WriteString("\n\nНедавний контекст диалога:\n")
		for _, item := range recentMessages(history, 6) {
			role := "Пользователь"
			if item.Role == "agent" {
				_, role = agents.Describe(item.AgentID)
			}
			builder.WriteString("- ")
			builder.WriteString(role)
			builder.WriteString(": ")
			builder.WriteString(shortenForPrompt(item.Content, 350))
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func buildWebResearchAnswerInput(userMessage string, project project.Project, plan webresearch.Plan, sources []webresearch.Source) string {
	var builder strings.Builder
	builder.WriteString("Проект: ")
	builder.WriteString(project.Name)
	builder.WriteString("\nЗапрос пользователя:\n")
	builder.WriteString(userMessage)
	builder.WriteString("\n\nПлан поиска:\n")
	if planJSON, err := json.MarshalIndent(plan, "", "  "); err == nil {
		builder.Write(planJSON)
	}
	builder.WriteString("\n\nНайденные источники:\n")
	builder.WriteString(webresearch.FormatSourcesForPrompt(sources))
	builder.WriteString("\n\nТребования к ответу:\n")
	builder.WriteString("- Ответь по сути запроса.\n")
	builder.WriteString("- Добавь ссылки в разделе источников.\n")
	builder.WriteString("- Ссылки пиши строго как обычный Markdown: [название](https://example.com). Не вкладывай одну ссылку внутрь другой и не экранируй круглые скобки.\n")
	builder.WriteString("- Не придумывай факты, которых нет в источниках.\n")
	return strings.TrimSpace(builder.String())
}

func webResearchFallbackAnswer(sources []webresearch.Source) string {
	var builder strings.Builder
	builder.WriteString("## Нашла источники\n\n")
	builder.WriteString("Я собрала материалы, но модель не смогла надежно сформировать итоговый текст. Ниже список источников для ручной проверки.\n\n")
	builder.WriteString(webResearchSourcesSection(sources))
	return strings.TrimSpace(builder.String())
}

func webResearchSourcesSection(sources []webresearch.Source) string {
	var builder strings.Builder
	builder.WriteString("## Источники\n\n")
	for index, source := range sources {
		title := strings.TrimSpace(source.Title)
		if title == "" {
			title = source.URL
		}
		builder.WriteString(fmt.Sprintf("%d. [%s](%s)", index+1, title, source.URL))
		if source.Snippet != "" {
			builder.WriteString(" — ")
			builder.WriteString(shortenForPrompt(source.Snippet, 180))
		}
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func buildDeveloperInput(intake string, product string, taskBlueprint *blueprint.Blueprint, architect string, sourceSnapshot string) string {
	return strings.TrimSpace(`
Task brief от Люмен:
` + intake + `

Требования от Продакта:
` + product + `

Task Blueprint:
` + blueprint.ToPrompt(taskBlueprint) + `

Архитектурный план:
` + architect + `

Файлы проекта:
` + sourceSnapshot + `

Подготовь developer plan и предлагаемые кодовые изменения строго по Task Blueprint. Создай все expected_files, если они нужны, и не создавай forbidden_files. Для action=replace используй содержимое файла из snapshot как основу и верни полный новый content. Не утверждай, что файлы уже изменены.
`)
}

func buildDeveloperStructuredChangesRepairInput(result v03WorkflowResult, project project.Project, reason string) string {
	return strings.TrimSpace(`
Предыдущий ответ Разработчика не может быть применен backend.
Причина:
` + reason + `

Задача: верни исправленный блок proposed changes. Не повторяй полный план, ревью, требования и длинные пояснения.

Task brief от Люмен:
` + shortenForPrompt(result.Intake, 900) + `

Task Blueprint:
` + blueprint.ToPrompt(&result.Blueprint) + `

Файлы проекта:
` + projectSourceSnapshot(project.Path) + `

Предыдущий ответ Разработчика:
` + shortenForPrompt(result.Developer, 1600) + `

Верни только:
## Proposed changes
[
  {
    "file_path": "relative/path/from/project",
    "action": "create или replace",
    "reason": "короткая причина",
    "content": "полное содержимое файла с JSON-экранированием кавычек и переводов строк"
  }
]

Правила:
- Ответ после заголовка должен быть валидным JSON-массивом.
- Для существующих файлов используй action="replace" и полный content.
- Для новых файлов используй action="create".
- Не добавляй markdown code fences вокруг JSON.
- Не добавляй текст после JSON-массива.
`)
}

func developerChangesRepairReason(err error, pendingChanges int, taskBlueprint blueprint.Blueprint) string {
	if err != nil {
		return "не удалось разобрать proposed changes Разработчика: " + err.Error()
	}
	if pendingChanges == 0 && blueprintRequiresCodeChanges(taskBlueprint) {
		return "Task Blueprint ожидает файлы, но Разработчик не вернул применимые proposed changes."
	}
	return "developer output не содержит применимых proposed changes."
}

func buildManagerFinalInput(intake string, product string, architect string, developer string) string {
	return strings.TrimSpace(`
Task brief:
` + intake + `

Требования:
` + product + `

Архитектурный план:
` + architect + `

План разработки:
` + developer + `

Собери итоговый ответ пользователю. Он должен быть коротким и практичным.
`)
}

func (s *Service) buildAutopilotFinalInput(ctx context.Context, result v03WorkflowResult, project project.Project, workflowRunID string, auto autopilotResult) string {
	changesList, _ := s.store.ListProposedChanges(ctx, project.ID, workflowRunID, 30)
	testRuns, _ := s.store.ListTestRuns(ctx, project.ID, workflowRunID, 30)
	testRuns = latestTestRunsByCommand(testRuns)
	reviewRuns, _ := s.store.ListReviewRuns(ctx, project.ID, workflowRunID, 5)

	var builder strings.Builder
	builder.WriteString("Task brief:\n")
	builder.WriteString(result.Intake)
	builder.WriteString("\n\nТребования:\n")
	builder.WriteString(result.Product)
	builder.WriteString("\n\nTask Blueprint:\n")
	builder.WriteString(blueprint.ToPrompt(&result.Blueprint))
	builder.WriteString("\n\nАрхитектурный план:\n")
	builder.WriteString(result.Architect)
	builder.WriteString("\n\nПлан разработки:\n")
	builder.WriteString(result.Developer)
	builder.WriteString("\n\nAutopilot result:\n")
	builder.WriteString(fmt.Sprintf("- applied files: %d\n", auto.AppliedFiles))
	builder.WriteString(fmt.Sprintf("- failed files: %d\n", auto.FailedFiles))
	builder.WriteString(fmt.Sprintf("- final tests passed: %d\n", auto.TestsPassed))
	builder.WriteString(fmt.Sprintf("- final tests failed: %d\n", auto.TestsFailed))
	builder.WriteString(fmt.Sprintf("- final tests blocked/not applicable: %d\n", auto.TestsBlocked))
	builder.WriteString(fmt.Sprintf("- review status: %s\n", auto.ReviewStatus))
	if auto.Blocked {
		builder.WriteString("- blocked reason: ")
		builder.WriteString(auto.BlockReason)
		builder.WriteString("\n")
	}
	if auto.ReviewReturnTo != "" {
		builder.WriteString("- review returned to: ")
		builder.WriteString(auto.ReviewReturnTo)
		builder.WriteString("\n")
	}
	builder.WriteString("\nИзменения:\n")
	for _, change := range changesList {
		builder.WriteString("- ")
		builder.WriteString(change.FilePath)
		builder.WriteString(" => ")
		builder.WriteString(change.Status)
		if change.Error != "" {
			builder.WriteString("; ")
			builder.WriteString(change.Error)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nФинальные проверки:\n")
	for _, testRun := range testRuns {
		builder.WriteString("- ")
		if testRun.WorkingDir != "" {
			builder.WriteString(testRun.WorkingDir)
			builder.WriteString(" $ ")
		}
		builder.WriteString(testRun.Command)
		builder.WriteString(" => ")
		builder.WriteString(testRun.Status)
		if testRun.Error != "" {
			builder.WriteString("; ")
			builder.WriteString(testRun.Error)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nРевью:\n")
	for _, review := range reviewRuns {
		builder.WriteString("- ")
		builder.WriteString(review.Status)
		if review.Summary != "" {
			builder.WriteString(": ")
			builder.WriteString(review.Summary)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nСобери финальный отчет пользователю: что сделано, какие файлы по задаче изменены, какие проверки прошли, статус ревью. Не упоминай служебные .zavod/runs артефакты. Не проси нажимать apply/test/review.")
	return builder.String()
}

func (s *Service) buildDeveloperRepairInput(ctx context.Context, auto autopilotResult, project project.Project, workflowRunID string, review reviews.ParsedReview) string {
	steps, _ := s.store.ListWorkflowSteps(ctx, workflowRunID)
	changesList, _ := s.store.ListProposedChanges(ctx, project.ID, workflowRunID, 30)
	testRuns, _ := s.store.ListTestRuns(ctx, project.ID, workflowRunID, 30)
	testRuns = latestTestRunsByCommand(testRuns)
	taskBlueprint, _ := s.store.LatestTaskBlueprint(ctx, workflowRunID)

	var builder strings.Builder
	builder.WriteString("Это repair-итерация автопилота. Исправь только замечания ревью и/или упавшие проверки.\n")
	builder.WriteString(fmt.Sprintf("Итерация: %d из %d.\n", auto.Iterations, maxAutoRepairIterations+1))
	builder.WriteString("\nTask Blueprint:\n")
	builder.WriteString(blueprint.ToPrompt(taskBlueprint))
	builder.WriteString("\n\nКонтекст предыдущих шагов:\n")
	for _, step := range steps {
		if step.StepKey == zw.StepDeveloperPlan {
			continue
		}
		builder.WriteString("\n## ")
		builder.WriteString(step.StepKey)
		builder.WriteString("\n")
		builder.WriteString(shortenForPrompt(step.Output, 1000))
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Ревьюер требует исправить\n")
	builder.WriteString(review.Summary)
	for _, item := range review.RequiredChanges {
		builder.WriteString("\n- ")
		builder.WriteString(item)
	}
	for _, finding := range review.Findings {
		builder.WriteString("\n- ")
		if finding.FilePath != "" {
			builder.WriteString(finding.FilePath)
			builder.WriteString(": ")
		}
		builder.WriteString(finding.Message)
		if finding.Suggestion != "" {
			builder.WriteString(" / ")
			builder.WriteString(finding.Suggestion)
		}
	}
	builder.WriteString("\n\n## Последние изменения и diff\n")
	for _, change := range changesList {
		builder.WriteString("\n### ")
		builder.WriteString(change.FilePath)
		builder.WriteString(" (")
		builder.WriteString(change.Status)
		builder.WriteString(")\n")
		if change.Error != "" {
			builder.WriteString("Ошибка применения: ")
			builder.WriteString(change.Error)
			builder.WriteString("\n")
		}
		builder.WriteString(shortenForPrompt(change.DiffText, 1600))
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Проверки\n")
	for _, testRun := range testRuns {
		builder.WriteString("- ")
		builder.WriteString(testRun.Command)
		builder.WriteString(" => ")
		builder.WriteString(testRun.Status)
		if testRun.Error != "" {
			builder.WriteString("; ")
			builder.WriteString(testRun.Error)
		}
		output := strings.TrimSpace(testRun.Stdout + "\n" + testRun.Stderr)
		if output != "" {
			builder.WriteString("\n")
			builder.WriteString(shortenForPrompt(output, 1000))
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Текущие файлы проекта после примененных изменений\n")
	builder.WriteString(projectSourceSnapshot(project.Path))
	builder.WriteString("\nВерни developer plan и новый блок ## Proposed changes. Для уже созданных файлов используй action=replace с полным содержимым файла.")
	return builder.String()
}

func (s *Service) buildTesterInput(ctx context.Context, project project.Project, workflowRunID string) string {
	steps, _ := s.store.ListWorkflowSteps(ctx, workflowRunID)
	changesList, _ := s.store.ListProposedChanges(ctx, project.ID, workflowRunID, 20)

	var builder strings.Builder
	builder.WriteString("Проект: ")
	builder.WriteString(project.Name)
	builder.WriteString("\n")
	builder.WriteString(projectCheckSignals(project.Path))
	builder.WriteString("\n")

	builder.WriteString("Контекст workflow:\n")
	for _, step := range steps {
		if step.StepKey == zw.StepTesterCommands {
			continue
		}
		builder.WriteString("\n## ")
		builder.WriteString(step.StepKey)
		builder.WriteString("\n")
		builder.WriteString(shortenForPrompt(step.Output, 1600))
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Примененные изменения\n")
	applied := 0
	for _, change := range changesList {
		if change.Status != changes.StatusApplied {
			continue
		}
		applied++
		builder.WriteString("- ")
		builder.WriteString(change.Action)
		builder.WriteString(" `")
		builder.WriteString(change.FilePath)
		builder.WriteString("`")
		if change.Reason != "" {
			builder.WriteString(": ")
			builder.WriteString(change.Reason)
		}
		builder.WriteString("\n")
	}
	if applied == 0 {
		builder.WriteString("Примененных изменений нет.\n")
	}
	builder.WriteString("\nПредложи минимальные команды проверки только из allowlist, только если они подходят структуре проекта и только если они связаны с примененными изменениями текущего workflow. Не запускай Python-проверки, если в текущем workflow не менялись Python-файлы. Не запускай npm-проверки, если не менялись frontend/package файлы. Для Go-изменений предпочитай go test ./... и не добавляй проверки другого стека.")
	return builder.String()
}

func projectCheckSignals(projectPath string) string {
	var builder strings.Builder
	builder.WriteString("Структура проекта для выбора проверок:\n")
	builder.WriteString("- go.mod: ")
	builder.WriteString(yesNo(fileExists(filepath.Join(projectPath, "go.mod"))))
	builder.WriteString("\n- Go-файлы в корне: ")
	goFiles := rootFilesWithSuffix(projectPath, ".go")
	if len(goFiles) == 0 {
		builder.WriteString("нет")
	} else {
		builder.WriteString(strings.Join(goFiles, ", "))
	}
	builder.WriteString("\n- package.json: ")
	builder.WriteString(yesNo(fileExists(filepath.Join(projectPath, "package.json"))))
	builder.WriteString("\n- frontend/package.json: ")
	builder.WriteString(yesNo(fileExists(filepath.Join(projectPath, "frontend", "package.json"))))
	builder.WriteString("\n- requirements.txt: ")
	builder.WriteString(yesNo(fileExists(filepath.Join(projectPath, "requirements.txt"))))
	builder.WriteString("\n- Python-файлы в корне: ")
	pythonFiles := rootFilesWithSuffix(projectPath, ".py")
	if len(pythonFiles) == 0 {
		builder.WriteString("нет")
	} else {
		builder.WriteString(strings.Join(pythonFiles, ", "))
	}
	builder.WriteString("\n")
	return builder.String()
}

func rootFilesWithSuffix(projectPath string, suffix string) []string {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil
	}
	files := make([]string, 0, 8)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		files = append(files, entry.Name())
		if len(files) == 8 {
			break
		}
	}
	sort.Strings(files)
	return files
}

func projectSourceSnapshot(projectPath string) string {
	paths := rootSourceSnapshotPaths(projectPath)
	if len(paths) == 0 {
		return "Релевантных корневых файлов для чтения не найдено."
	}

	var builder strings.Builder
	totalBytes := 0
	for _, relativePath := range paths {
		if totalBytes >= 48*1024 {
			builder.WriteString("\n...[source snapshot truncated]\n")
			break
		}
		fullPath := filepath.Join(projectPath, relativePath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		if len(content) > 16*1024 {
			builder.WriteString("\n## ")
			builder.WriteString(relativePath)
			builder.WriteString("\n[файл больше 16KB, содержимое не включено]\n")
			continue
		}
		totalBytes += len(content)
		builder.WriteString("\n## ")
		builder.WriteString(relativePath)
		builder.WriteString("\n```")
		builder.WriteString(sourceFenceLanguage(relativePath))
		builder.WriteString("\n")
		builder.Write(content)
		if len(content) == 0 || content[len(content)-1] != '\n' {
			builder.WriteString("\n")
		}
		builder.WriteString("```\n")
	}

	return strings.TrimSpace(builder.String())
}

func rootSourceSnapshotPaths(projectPath string) []string {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	add := func(path string) {
		if _, ok := seen[path]; ok {
			return
		}
		if !fileExists(filepath.Join(projectPath, path)) {
			return
		}
		seen[path] = struct{}{}
	}
	for _, path := range []string{"go.mod", "package.json", "requirements.txt"} {
		add(path)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".py") {
			add(name)
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) > 12 {
		paths = paths[:12]
	}
	return paths
}

func sourceFenceLanguage(path string) string {
	switch {
	case strings.HasSuffix(path, ".go"):
		return "go"
	case strings.HasSuffix(path, ".py"):
		return "python"
	case strings.HasSuffix(path, ".json"):
		return "json"
	default:
		return ""
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func yesNo(value bool) string {
	if value {
		return "да"
	}
	return "нет"
}

func (s *Service) buildReviewInput(ctx context.Context, project project.Project, workflowRunID string) string {
	steps, _ := s.store.ListWorkflowSteps(ctx, workflowRunID)
	changesList, _ := s.store.ListProposedChanges(ctx, project.ID, workflowRunID, 30)
	testRuns, _ := s.store.ListTestRuns(ctx, project.ID, workflowRunID, 30)
	testRuns = s.refreshUnsupportedTestRuns(ctx, project, testRuns)
	testRuns = latestTestRunsByCommand(testRuns)
	taskBlueprint, _ := s.store.LatestTaskBlueprint(ctx, workflowRunID)

	var builder strings.Builder
	builder.WriteString("Проект: ")
	builder.WriteString(project.Name)
	builder.WriteString("\nПуть проекта не используй как источник фактов, ревью делай только по данным ниже.\n")
	builder.WriteString("\n# Task Blueprint\n")
	builder.WriteString(blueprint.ToPrompt(taskBlueprint))
	builder.WriteString("\n")

	builder.WriteString("\n# Workflow context\n")
	for _, step := range steps {
		if step.StepKey == zw.StepReview {
			continue
		}
		builder.WriteString("\n## ")
		builder.WriteString(step.StepKey)
		builder.WriteString("\n")
		if step.Error != "" {
			builder.WriteString("Ошибка шага: ")
			builder.WriteString(step.Error)
			builder.WriteString("\n")
		}
		builder.WriteString(shortenForPrompt(step.Output, 1600))
		builder.WriteString("\n")
	}

	builder.WriteString("\n# Applied changes and diff\n")
	applied := 0
	for _, change := range changesList {
		if change.Status != changes.StatusApplied {
			continue
		}
		applied++
		builder.WriteString("\n## ")
		builder.WriteString(change.FilePath)
		builder.WriteString("\nAction: ")
		builder.WriteString(change.Action)
		if change.Reason != "" {
			builder.WriteString("\nReason: ")
			builder.WriteString(change.Reason)
		}
		builder.WriteString("\nDiff:\n")
		builder.WriteString(shortenForPrompt(change.DiffText, 2600))
		builder.WriteString("\n")
	}
	if applied == 0 {
		builder.WriteString("Нет примененных изменений.\n")
	}

	builder.WriteString("\n# Final test results\n")
	if len(testRuns) == 0 {
		builder.WriteString("Проверки не запускались и не предлагались.\n")
	} else {
		for _, testRun := range testRuns {
			builder.WriteString("- ")
			if testRun.WorkingDir != "" {
				builder.WriteString(testRun.WorkingDir)
				builder.WriteString(" $ ")
			}
			builder.WriteString(testRun.Command)
			builder.WriteString(" => ")
			builder.WriteString(testRun.Status)
			if testRun.ExitCode != 0 {
				builder.WriteString(fmt.Sprintf(" (exit %d)", testRun.ExitCode))
			}
			if testRun.Error != "" {
				builder.WriteString("; error: ")
				builder.WriteString(testRun.Error)
			}
			output := strings.TrimSpace(testRun.Stdout + "\n" + testRun.Stderr)
			if output != "" {
				builder.WriteString("\nOutput:\n")
				builder.WriteString(shortenForPrompt(output, 1200))
			}
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\nВерни итог ревью строго JSON.")
	return builder.String()
}

func recentMessages(messages []chat.Message, limit int) []chat.Message {
	if len(messages) <= limit {
		return messages
	}
	return messages[len(messages)-limit:]
}

func managerNeedsClarification(output string) bool {
	trimmed := strings.TrimSpace(output)
	var decoded managerIntakeResult
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		return decoded.NeedsClarification
	}
	lower := strings.ToLower(trimmed)
	return strings.Contains(lower, `"needs_clarification": true`) ||
		strings.Contains(lower, "needs_clarification: true") ||
		strings.Contains(lower, "needs_clarification=true")
}

type managerIntakeResult struct {
	Summary            string   `json:"summary"`
	Goal               string   `json:"goal"`
	Constraints        []string `json:"constraints"`
	OpenQuestions      []string `json:"open_questions"`
	NeedsClarification bool     `json:"needs_clarification"`
}

func parseManagerIntake(output string) (managerIntakeResult, bool) {
	var result managerIntakeResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &result); err != nil {
		return managerIntakeResult{}, false
	}
	return result, true
}

func pendingClarificationFromSteps(run *zw.Run, steps []zw.Step) *ClarificationDTO {
	if run == nil || run.Status != zw.StatusWaitingUser || run.CurrentStep != zw.StepManagerIntake {
		return nil
	}
	for index := len(steps) - 1; index >= 0; index-- {
		step := steps[index]
		if step.StepKey != zw.StepManagerIntake || step.Output == "" {
			continue
		}
		intake, ok := parseManagerIntake(step.Output)
		if !ok || !intake.NeedsClarification || len(intake.OpenQuestions) == 0 {
			return nil
		}
		questions := make([]ClarificationQuestion, 0, len(intake.OpenQuestions))
		for questionIndex, question := range intake.OpenQuestions {
			question = strings.TrimSpace(question)
			if question == "" {
				continue
			}
			questions = append(questions, ClarificationQuestion{
				ID:   fmt.Sprintf("q%d", questionIndex+1),
				Text: question,
			})
		}
		if len(questions) == 0 {
			return nil
		}
		return &ClarificationDTO{
			WorkflowRunID: run.ID,
			Summary:       strings.TrimSpace(intake.Summary),
			Goal:          strings.TrimSpace(intake.Goal),
			Questions:     questions,
		}
	}
	return nil
}

func normalizeClarificationAnswers(items []ClarificationAnswer) []ClarificationAnswer {
	out := make([]ClarificationAnswer, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		item.QuestionID = strings.TrimSpace(item.QuestionID)
		item.Question = strings.TrimSpace(item.Question)
		item.Answer = strings.TrimSpace(item.Answer)
		if item.Answer == "" {
			continue
		}
		key := item.QuestionID
		if key == "" {
			key = item.Question
		}
		if key == "" {
			key = item.Answer
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func clarificationAnswersMessage(items []ClarificationAnswer) string {
	var builder strings.Builder
	builder.WriteString("Ответы на уточнения:\n")
	for index, item := range items {
		builder.WriteString(fmt.Sprintf("%d. ", index+1))
		if item.Question != "" {
			builder.WriteString(item.Question)
			builder.WriteString("\nОтвет: ")
		}
		builder.WriteString(item.Answer)
		if index != len(items)-1 {
			builder.WriteString("\n\n")
		}
	}
	return builder.String()
}

func clarificationNotice(raw string) string {
	intake, ok := parseManagerIntake(raw)
	if ok {
		summary := strings.TrimSpace(intake.Summary)
		if summary == "" {
			summary = "нужно уточнить несколько деталей перед запуском завода."
		}
		return "## Нужно уточнение\n" + summary + "\n\nОтветь в форме уточнений ниже. Вопросы сохранены структурированно, цитировать их вручную не нужно."
	}
	return "## Нужно уточнение\nОтветь в форме уточнений ниже. Вопросы сохранены структурированно, цитировать их вручную не нужно."
}

func formatClarificationMessage(intake managerIntakeResult) string {
	var builder strings.Builder
	summary := strings.TrimSpace(intake.Summary)
	goal := strings.TrimSpace(intake.Goal)
	if summary == "" {
		summary = "нужно уточнить детали задачи перед передачей Продакту и Архитектору."
	}

	builder.WriteString("Поняла задачу: ")
	builder.WriteString(summary)
	if goal != "" {
		builder.WriteString("\n\nЦель: ")
		builder.WriteString(goal)
	}
	if len(intake.OpenQuestions) > 0 {
		builder.WriteString("\n\nЧто нужно уточнить:")
		for index, question := range intake.OpenQuestions {
			question = strings.TrimSpace(question)
			if question == "" {
				continue
			}
			builder.WriteString(fmt.Sprintf("\n%d. %s", index+1, question))
		}
	}
	builder.WriteString("\n\nСледующий шаг: ответь на вопросы, и я передам задачу Продакту и Архитектору.")
	return builder.String()
}

func artifactSummary(items []artifacts.Artifact) string {
	if len(items) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("\n\n## Артефакты сохранены\n")
	for _, item := range items {
		if item.Kind != "task_spec" {
			continue
		}
		builder.WriteString("- `")
		builder.WriteString(item.RelativePath)
		builder.WriteString("`")
		if item.Title != "" {
			builder.WriteString(" — ")
			builder.WriteString(item.Title)
		}
		builder.WriteString("\n")
	}
	for _, item := range items {
		if item.Kind == "task_spec" {
			continue
		}
		builder.WriteString("- `")
		builder.WriteString(item.RelativePath)
		builder.WriteString("`")
		if item.Title != "" {
			builder.WriteString(" — ")
			builder.WriteString(item.Title)
		}
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func proposedChangesSummary(items []changes.ProposedChange) string {
	if len(items) == 0 {
		return "\n\n## Изменения\nРазработчик не вернул структурированные изменения для применения. Кодовые предложения сохранены в developer plan."
	}

	var builder strings.Builder
	builder.WriteString("\n\n## Изменения ожидают подтверждения\n")
	for _, item := range items {
		builder.WriteString("- `")
		builder.WriteString(item.FilePath)
		builder.WriteString("` — ")
		builder.WriteString(item.Action)
		if item.Reason != "" {
			builder.WriteString("; ")
			builder.WriteString(item.Reason)
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nКодовые файлы пока не записаны. Нажми \"Применить изменения\", чтобы создать или изменить их в проекте.")
	return strings.TrimRight(builder.String(), "\n")
}

func autopilotSummary(result autopilotResult) string {
	var builder strings.Builder
	builder.WriteString("\n\n## Autopilot\n")
	if result.AppliedFiles > 0 {
		builder.WriteString(fmt.Sprintf("- Изменения применены автоматически: %d\n", result.AppliedFiles))
	}
	if result.FailedFiles > 0 {
		builder.WriteString(fmt.Sprintf("- Не применено файлов: %d\n", result.FailedFiles))
	}
	builder.WriteString(fmt.Sprintf("- Финальные проверки: успешно %d, ошибок %d, не применимо %d\n", result.TestsPassed, result.TestsFailed, result.TestsBlocked))
	if result.ReviewStatus != "" {
		builder.WriteString("- Ревью: ")
		builder.WriteString(result.ReviewStatus)
		builder.WriteString("\n")
	}
	if result.ReviewReturnTo != "" {
		builder.WriteString("- Возврат от ревью: ")
		builder.WriteString(reviewReturnLabel(result.ReviewReturnTo))
		builder.WriteString("\n")
	}
	builder.WriteString(fmt.Sprintf("- Итераций: %d\n", result.Iterations))
	if result.Blocked {
		builder.WriteString("- Остановка: ")
		builder.WriteString(result.BlockReason)
		builder.WriteString("\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}

func deterministicBlockedFinal(result autopilotResult) string {
	var builder strings.Builder
	builder.WriteString("## Workflow остановлен\n")
	if result.BlockReason != "" {
		builder.WriteString(result.BlockReason)
		builder.WriteString("\n")
	}
	builder.WriteString("\n## Что важно\n")
	builder.WriteString(fmt.Sprintf("- Файлы: применено %d, не применено %d\n", result.AppliedFiles, result.FailedFiles))
	builder.WriteString(fmt.Sprintf("- Последние проверки: успешно %d, ошибок %d, не применимо %d\n", result.TestsPassed, result.TestsFailed, result.TestsBlocked))
	if result.ReviewStatus != "" {
		builder.WriteString("- Ревью: ")
		builder.WriteString(result.ReviewStatus)
		builder.WriteString("\n")
	}
	if result.ReviewSummary != "" {
		builder.WriteString("- Причина ревью: ")
		builder.WriteString(result.ReviewSummary)
		builder.WriteString("\n")
	}
	if result.ReviewReturnTo != "" {
		builder.WriteString("- Возврат от ревью: ")
		builder.WriteString(reviewReturnLabel(result.ReviewReturnTo))
		builder.WriteString("\n")
	}
	builder.WriteString(fmt.Sprintf("- Итераций: %d\n", result.Iterations))
	if len(result.ReviewRequired) > 0 || len(result.ReviewFindings) > 0 {
		builder.WriteString("\n## Что не принято ревьюером\n")
		for index, item := range result.ReviewRequired {
			if index >= 5 {
				break
			}
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
		for index, finding := range result.ReviewFindings {
			if index >= 5 {
				break
			}
			builder.WriteString("- ")
			if finding.FilePath != "" {
				builder.WriteString("`")
				builder.WriteString(finding.FilePath)
				builder.WriteString("`: ")
			}
			builder.WriteString(finding.Message)
			if finding.Suggestion != "" {
				builder.WriteString(" / ")
				builder.WriteString(finding.Suggestion)
			}
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n## Что дальше\n")
	builder.WriteString("Нужно устранить причину остановки и запустить задачу повторно.")
	return strings.TrimRight(builder.String(), "\n")
}

func (s *Service) buildDirectAnswerInput(ctx context.Context, project project.Project, task chat.Task, latestRun *zw.Run, userMessage string, decision router.Decision) string {
	var builder strings.Builder
	builder.WriteString("# User request\n")
	builder.WriteString(userMessage)
	builder.WriteString("\n\n# Router decision\n")
	builder.WriteString(fmt.Sprintf("- intent: %s\n- confidence: %s\n- reason: %s\n", decision.Intent, decision.Confidence, decision.Reason))
	builder.WriteString("\n# Project\n")
	builder.WriteString(fmt.Sprintf("- name: %s\n- path: %s\n", project.Name, project.Path))

	messages, _ := s.store.ListMessages(ctx, task.ID)
	if len(messages) > 0 {
		builder.WriteString("\n# Recent chat\n")
		for _, item := range recentMessages(messages, 12) {
			role := item.Role
			if item.AgentID != "" {
				role = item.AgentID
			}
			builder.WriteString("- ")
			builder.WriteString(role)
			builder.WriteString(": ")
			builder.WriteString(shortenForPrompt(item.Content, 450))
			builder.WriteString("\n")
		}
	}

	if latestRun != nil {
		builder.WriteString("\n# Latest workflow\n")
		builder.WriteString(fmt.Sprintf("- id: %s\n- status: %s\n- current_step: %s\n- error: %s\n", latestRun.ID, latestRun.Status, latestRun.CurrentStep, latestRun.Error))
		steps, _ := s.store.ListWorkflowSteps(ctx, latestRun.ID)
		if len(steps) > 0 {
			builder.WriteString("\n## Steps\n")
			for _, step := range steps {
				builder.WriteString("- ")
				builder.WriteString(step.StepKey)
				builder.WriteString(" / ")
				builder.WriteString(step.AgentID)
				builder.WriteString(" / ")
				builder.WriteString(step.Status)
				if step.Error != "" {
					builder.WriteString(" / error: ")
					builder.WriteString(step.Error)
				}
				if step.Output != "" {
					builder.WriteString("\n  output: ")
					builder.WriteString(shortenForPrompt(humanStepOutput(step.StepKey, step.Output), 650))
				}
				builder.WriteString("\n")
			}
		}

		taskBlueprint, _ := s.store.LatestTaskBlueprint(ctx, latestRun.ID)
		if taskBlueprint != nil {
			builder.WriteString("\n## Task Blueprint\n")
			builder.WriteString(blueprint.ToPrompt(taskBlueprint))
			builder.WriteString("\n")
		}

		changesList, _ := s.store.ListProposedChanges(ctx, project.ID, latestRun.ID, 30)
		if len(changesList) > 0 {
			builder.WriteString("\n## Code changes\n")
			for _, change := range changesList {
				builder.WriteString("- ")
				builder.WriteString(change.FilePath)
				builder.WriteString(" / ")
				builder.WriteString(change.Action)
				builder.WriteString(" / ")
				builder.WriteString(change.Status)
				if change.Error != "" {
					builder.WriteString(" / error: ")
					builder.WriteString(change.Error)
				}
				if change.Reason != "" {
					builder.WriteString(" / ")
					builder.WriteString(change.Reason)
				}
				builder.WriteString("\n")
			}
		}

		testRuns, _ := s.store.ListTestRuns(ctx, project.ID, latestRun.ID, 30)
		testRuns = latestTestRunsByCommand(testRuns)
		if len(testRuns) > 0 {
			builder.WriteString("\n## Last test results\n")
			for _, testRun := range testRuns {
				builder.WriteString("- ")
				if testRun.WorkingDir != "" {
					builder.WriteString(testRun.WorkingDir)
					builder.WriteString(" $ ")
				}
				builder.WriteString(testRun.Command)
				builder.WriteString(" => ")
				builder.WriteString(testRun.Status)
				if testRun.Error != "" {
					builder.WriteString("; ")
					builder.WriteString(testRun.Error)
				}
				builder.WriteString("\n")
			}
		}

		reviewRuns, _ := s.store.ListReviewRuns(ctx, project.ID, latestRun.ID, 5)
		if len(reviewRuns) > 0 {
			builder.WriteString("\n## Last reviews\n")
			for _, review := range reviewRuns {
				builder.WriteString("- ")
				builder.WriteString(review.Status)
				if review.Summary != "" {
					builder.WriteString(": ")
					builder.WriteString(shortenForPrompt(review.Summary, 500))
				}
				if review.ReturnTo != "" {
					builder.WriteString(" / return_to: ")
					builder.WriteString(review.ReturnTo)
				}
				builder.WriteString("\n")
			}
		}
	}

	artifactContext := s.latestArtifactContext(ctx, project)
	if artifactContext != "" {
		builder.WriteString("\n# Relevant saved artifacts\n")
		builder.WriteString(artifactContext)
	}

	if decision.NeedsProjectContext || decision.Intent == router.IntentProjectAnalysis || decision.Intent == router.IntentPentestTask {
		builder.WriteString("\n# Project source snapshot\n")
		builder.WriteString(projectSourceSnapshot(project.Path))
	}

	builder.WriteString("\n# Answering instructions\n")
	builder.WriteString("- Ответь на конкретный вопрос пользователя.\n")
	builder.WriteString("- Если пользователь спросил про спеку, опиши именно последнюю сохраненную/рабочую спеку и по каким требованиям работал завод.\n")
	builder.WriteString("- Если данных не хватает, скажи что именно не найдено, но не запускай новый workflow.\n")
	builder.WriteString("- Не перечисляй служебные `.zavod/runs` артефакты без явной просьбы.\n")
	return builder.String()
}

func (s *Service) latestArtifactContext(ctx context.Context, project project.Project) string {
	items, err := s.store.ListArtifacts(ctx, project.ID, 12)
	if err != nil {
		return ""
	}
	var builder strings.Builder
	includedTaskSpec := false
	included := 0
	for _, item := range items {
		if included >= 4 {
			break
		}
		if item.Kind != "task_spec" && item.Kind != "developer_plan" && item.Kind != "workflow_step" {
			continue
		}
		if item.Kind == "task_spec" && includedTaskSpec {
			continue
		}
		content := readProjectFileSnippet(project.Path, item.RelativePath, 1800)
		if content == "" {
			continue
		}
		if item.Kind == "task_spec" {
			includedTaskSpec = true
		}
		included++
		builder.WriteString("\n## ")
		builder.WriteString(item.Title)
		builder.WriteString(" (")
		builder.WriteString(item.Kind)
		builder.WriteString(")\n")
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func wantsSavedTaskSpec(message string) bool {
	text := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(message))), " ")
	if !strings.Contains(text, "спек") {
		return false
	}
	return strings.Contains(text, "выведи") ||
		strings.Contains(text, "покажи") ||
		strings.Contains(text, "напечатай") ||
		strings.Contains(text, "дай") ||
		strings.Contains(text, "по которой")
}

func (s *Service) savedTaskSpecAnswer(ctx context.Context, project project.Project) string {
	content, relativePath := s.latestTaskSpecContent(ctx, project)
	if strings.TrimSpace(content) == "" {
		return "## Спека не найдена\nЯ не нашла сохраненную `docs/task-spec.md` для последнего запуска. Похоже, workflow еще не сохранял task spec для этого проекта."
	}
	var builder strings.Builder
	builder.WriteString("## Спека, по которой работал завод\n\n")
	if relativePath != "" {
		builder.WriteString("Источник: `")
		builder.WriteString(relativePath)
		builder.WriteString("`\n\n")
	}
	builder.WriteString(strings.TrimSpace(content))
	return builder.String()
}

func (s *Service) latestTaskSpecContent(ctx context.Context, project project.Project) (string, string) {
	items, err := s.store.ListArtifacts(ctx, project.ID, 30)
	if err == nil {
		for _, item := range items {
			if item.Kind != "task_spec" {
				continue
			}
			content := readProjectFileSnippet(project.Path, item.RelativePath, 20*1024)
			if strings.TrimSpace(content) != "" {
				return content, item.RelativePath
			}
		}
	}
	content := readProjectFileSnippet(project.Path, filepath.Join("docs", "task-spec.md"), 20*1024)
	if strings.TrimSpace(content) != "" {
		return content, filepath.Join("docs", "task-spec.md")
	}
	return "", ""
}

func readProjectFileSnippet(projectPath string, relativePath string, limit int) string {
	if strings.TrimSpace(projectPath) == "" || strings.TrimSpace(relativePath) == "" {
		return ""
	}
	rootAbs, err := filepath.Abs(projectPath)
	if err != nil {
		return ""
	}
	targetAbs, err := filepath.Abs(filepath.Join(rootAbs, relativePath))
	if err != nil {
		return ""
	}
	if targetAbs != rootAbs && !strings.HasPrefix(targetAbs, rootAbs+string(os.PathSeparator)) {
		return ""
	}
	data, err := os.ReadFile(targetAbs)
	if err != nil {
		return ""
	}
	return shortenForPrompt(string(data), limit)
}

func humanStepOutput(stepKey string, output string) string {
	output = strings.TrimSpace(output)
	switch stepKey {
	case zw.StepManagerIntake:
		intake, ok := parseManagerIntake(output)
		if !ok {
			return output
		}
		parts := []string{}
		if intake.Summary != "" {
			parts = append(parts, "summary: "+intake.Summary)
		}
		if intake.Goal != "" {
			parts = append(parts, "goal: "+intake.Goal)
		}
		if len(intake.OpenQuestions) > 0 {
			parts = append(parts, "questions: "+strings.Join(intake.OpenQuestions, "; "))
		}
		return strings.Join(parts, " | ")
	case zw.StepTaskBlueprint:
		parsed, err := blueprint.Parse(output)
		if err != nil {
			return output
		}
		return blueprint.ToPrompt(&parsed)
	default:
		return output
	}
}

func parseIntentDecision(output string) (router.Decision, error) {
	trimmed := strings.TrimSpace(output)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			trimmed = trimmed[start : end+1]
		}
	}
	var decision router.Decision
	if err := json.Unmarshal([]byte(trimmed), &decision); err != nil {
		return router.Decision{}, err
	}
	return normalizeIntentDecision(decision), nil
}

func normalizeIntentDecision(decision router.Decision) router.Decision {
	decision.Intent = router.Intent(strings.TrimSpace(string(decision.Intent)))
	decision.Confidence = strings.TrimSpace(decision.Confidence)
	decision.Reason = strings.TrimSpace(decision.Reason)
	switch decision.Intent {
	case router.IntentCodingTask, router.IntentClarificationAnswer, router.IntentPentestTask, router.IntentResearchTask:
		decision.NeedsWorkflow = true
		decision.NeedsProjectContext = true
	case router.IntentDirectAnswer, router.IntentProjectAnalysis, router.IntentWorkflowControl, router.IntentGeneralChat:
		decision.NeedsWorkflow = false
	default:
		decision.Intent = router.IntentDirectAnswer
		decision.NeedsWorkflow = false
	}
	if decision.Confidence == "" {
		decision.Confidence = "medium"
	}
	return decision
}

func directAnswerActivity(intent router.Intent) string {
	switch intent {
	case router.IntentProjectAnalysis:
		return "Разбирает проект и отвечает без правок"
	case router.IntentWorkflowControl:
		return "Разбирает состояние workflow"
	case router.IntentPentestTask:
		return "Разбирает security-запрос"
	case router.IntentResearchTask:
		return "Ищет актуальную информацию"
	case router.IntentGeneralChat:
		return "Отвечает на общий вопрос"
	default:
		return "Отвечает по контексту проекта"
	}
}

func blueprintRequiresCodeChanges(taskBlueprint blueprint.Blueprint) bool {
	return len(taskBlueprint.ExpectedFiles) > 0
}

func autoApplyChangesMessage(applied int, failed int) string {
	var builder strings.Builder
	builder.WriteString("## Изменения применены автоматически\n")
	if applied > 0 {
		builder.WriteString(fmt.Sprintf("Применено файлов: %d.\n", applied))
	}
	if failed > 0 {
		builder.WriteString(fmt.Sprintf("Не применено файлов: %d. Workflow остановлен до исправления причины.\n", failed))
	}
	if applied == 0 && failed == 0 {
		builder.WriteString("Новых изменений для применения нет.")
	}
	return strings.TrimSpace(builder.String())
}

func applyChangesMessage(applied int, failed int) string {
	var builder strings.Builder
	builder.WriteString("## Применение изменений\n")
	if applied > 0 {
		builder.WriteString(fmt.Sprintf("Применено файлов: %d.\n", applied))
	}
	if failed > 0 {
		builder.WriteString(fmt.Sprintf("Не применено файлов: %d. Подробности видны в блоке изменений.\n", failed))
	}
	if applied == 0 && failed == 0 {
		builder.WriteString("Нет ожидающих изменений.")
	}
	return strings.TrimSpace(builder.String())
}

func testSuggestionsMessage(prefix string, items []checks.TestRun) string {
	var builder strings.Builder
	builder.WriteString(prefix)
	if len(items) == 0 {
		builder.WriteString("\n\nКоманды проверки не подготовлены.")
		return builder.String()
	}
	builder.WriteString("\n\n")
	for _, item := range items {
		builder.WriteString("- `")
		if item.WorkingDir != "" {
			builder.WriteString(item.WorkingDir)
			builder.WriteString(" $ ")
		}
		builder.WriteString(item.Command)
		builder.WriteString("`")
		if item.Reason != "" {
			builder.WriteString(" — ")
			builder.WriteString(item.Reason)
		}
		if item.Status == checks.StatusBlocked {
			builder.WriteString(" (заблокировано: ")
			builder.WriteString(item.Error)
			builder.WriteString(")")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("\nВ Autopilot проверки запускаются автоматически. Кнопки в блоке \"Проверки\" нужны для ручного повтора.")
	return strings.TrimRight(builder.String(), "\n")
}

func testRunResultMessage(item checks.TestRun, result checks.RunResult) string {
	var builder strings.Builder
	builder.WriteString("## Результат проверки\n")
	builder.WriteString("Команда: `")
	if item.WorkingDir != "" {
		builder.WriteString(item.WorkingDir)
		builder.WriteString(" $ ")
	}
	builder.WriteString(item.Command)
	builder.WriteString("`\n")
	switch result.Status {
	case checks.StatusPassed:
		builder.WriteString("Статус: успешно.")
	case checks.StatusBlocked:
		builder.WriteString("Статус: проверка не применима.")
	default:
		builder.WriteString("Статус: ошибка.")
	}
	if result.Error != "" {
		builder.WriteString("\n")
		builder.WriteString(result.Error)
	}
	return strings.TrimSpace(builder.String())
}

func reviewMessage(parsed reviews.ParsedReview) string {
	var builder strings.Builder
	builder.WriteString("## Ревью\n")
	switch parsed.Status {
	case reviews.StatusAccepted:
		builder.WriteString("Статус: принято.\n\n")
	case reviews.StatusBlocked:
		builder.WriteString("Статус: workflow остановлен.\n\n")
	default:
		builder.WriteString("Статус: нужна доработка.\n\n")
	}
	builder.WriteString(parsed.Summary)
	if parsed.ReturnTo != "" {
		builder.WriteString("\n\nВернуть к роли: ")
		builder.WriteString(reviewReturnLabel(parsed.ReturnTo))
	}
	if parsed.BlockingReason != "" {
		builder.WriteString("\n\nПричина остановки: ")
		builder.WriteString(parsed.BlockingReason)
	}
	if len(parsed.Findings) > 0 {
		builder.WriteString("\n\n## Замечания\n")
		for _, finding := range parsed.Findings {
			builder.WriteString("- ")
			builder.WriteString(finding.Severity)
			if finding.FilePath != "" {
				builder.WriteString(" · `")
				builder.WriteString(finding.FilePath)
				builder.WriteString("`")
			}
			if finding.Message != "" {
				builder.WriteString(" — ")
				builder.WriteString(finding.Message)
			}
			if finding.Suggestion != "" {
				builder.WriteString(" Предложение: ")
				builder.WriteString(finding.Suggestion)
			}
			builder.WriteString("\n")
		}
	}
	if len(parsed.RequiredChanges) > 0 {
		builder.WriteString("\n## Обязательные доработки\n")
		for _, item := range parsed.RequiredChanges {
			builder.WriteString("- ")
			builder.WriteString(item)
			builder.WriteString("\n")
		}
	}
	if parsed.RecommendedNextStep != "" {
		builder.WriteString("\n## Следующий шаг\n")
		builder.WriteString(parsed.RecommendedNextStep)
	}
	return strings.TrimSpace(builder.String())
}

func reviewBlockedReason(parsed reviews.ParsedReview) string {
	if parsed.BlockingReason != "" {
		return parsed.BlockingReason
	}
	if parsed.Summary != "" {
		return parsed.Summary
	}
	return "Ревьюер остановил workflow."
}

func reviewReturnAgentID(returnTo string) string {
	switch returnTo {
	case reviews.ReturnToProduct:
		return agents.ProductID
	case reviews.ReturnToArchitect:
		return agents.ArchitectID
	case reviews.ReturnToTester:
		return agents.TesterID
	case reviews.ReturnToUser:
		return agents.ManagerID
	default:
		return agents.DeveloperID
	}
}

func reviewReturnActivity(returnTo string) string {
	switch returnTo {
	case reviews.ReturnToProduct:
		return "Нужна доработка требований"
	case reviews.ReturnToArchitect:
		return "Нужна доработка архитектуры"
	case reviews.ReturnToTester:
		return "Нужно повторить проверки"
	case reviews.ReturnToUser:
		return "Нужно уточнение пользователя"
	default:
		return "Нужна доработка по ревью"
	}
}

func reviewReturnLabel(returnTo string) string {
	switch returnTo {
	case reviews.ReturnToProduct:
		return "Продакт"
	case reviews.ReturnToArchitect:
		return "Архитектор"
	case reviews.ReturnToTester:
		return "Тестировщик"
	case reviews.ReturnToUser:
		return "Пользователь"
	default:
		return "Разработчик"
	}
}

func reviewFeedbackForPrompt(review reviews.ParsedReview) string {
	var builder strings.Builder
	if review.Summary != "" {
		builder.WriteString(review.Summary)
	}
	if review.ReturnTo != "" {
		builder.WriteString("\nВернуть к роли: ")
		builder.WriteString(reviewReturnLabel(review.ReturnTo))
	}
	if review.BlockingReason != "" {
		builder.WriteString("\nПричина остановки: ")
		builder.WriteString(review.BlockingReason)
	}
	if len(review.RequiredChanges) > 0 {
		builder.WriteString("\nОбязательные доработки:")
		for _, item := range review.RequiredChanges {
			builder.WriteString("\n- ")
			builder.WriteString(item)
		}
	}
	if len(review.Findings) > 0 {
		builder.WriteString("\nЗамечания:")
		for _, finding := range review.Findings {
			builder.WriteString("\n- ")
			if finding.FilePath != "" {
				builder.WriteString(finding.FilePath)
				builder.WriteString(": ")
			}
			builder.WriteString(finding.Message)
			if finding.Suggestion != "" {
				builder.WriteString(" / ")
				builder.WriteString(finding.Suggestion)
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func shortenForPrompt(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n...[truncated]"
}

func stepThinkingActivity(stepKey string) string {
	switch stepKey {
	case zw.StepProductRequirements:
		return "Формирует требования"
	case zw.StepTaskBlueprint:
		return "Фиксирует стек и файлы"
	case zw.StepArchitectPlan:
		return "Проектирует технический план"
	case zw.StepSecurityAnalysis:
		return "Проводит ИБ-анализ"
	case zw.StepWebResearch:
		return "Планирует поиск в интернете"
	case zw.StepDeveloperPlan:
		return "Готовит план разработки"
	case zw.StepTesterCommands:
		return "Подбирает проверки"
	case zw.StepManagerFinal:
		return "Собирает итоговый ответ"
	case zw.StepReview:
		return "Проверяет результат"
	default:
		return "Разбирает задачу пользователя"
	}
}

func stepDoneActivity(stepKey string) string {
	switch stepKey {
	case zw.StepProductRequirements:
		return "Требования готовы"
	case zw.StepTaskBlueprint:
		return "Task Blueprint готов"
	case zw.StepArchitectPlan:
		return "Архитектурный план готов"
	case zw.StepSecurityAnalysis:
		return "ИБ-анализ готов"
	case zw.StepWebResearch:
		return "Источники собраны"
	case zw.StepDeveloperPlan:
		return "План разработки готов"
	case zw.StepTesterCommands:
		return "Команды проверки готовы"
	case zw.StepManagerFinal:
		return "Итог собран"
	case zw.StepReview:
		return "Ревью готово"
	default:
		return "Заявка принята"
	}
}

func looksLikeRepetitionLoop(text string) bool {
	words := strings.Fields(text)
	if len(words) < repetitionLoopMinRepeats*2 {
		return false
	}

	maxPatternLen := minInt(12, len(words)/repetitionLoopMinRepeats)
	for patternLen := 1; patternLen <= maxPatternLen; patternLen++ {
		patternStart := len(words) - patternLen
		repeats := 1
		for start := patternStart - patternLen; start >= 0; start -= patternLen {
			if !sameWords(words[start:start+patternLen], words[patternStart:]) {
				break
			}
			repeats++
			if repeats >= repetitionLoopMinRepeats {
				return true
			}
		}
	}
	return false
}

func sameWords(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
