package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"zavod_ai/internal/agents"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/config"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/providers/openaiapi"
	"zavod_ai/internal/store"
	zw "zavod_ai/internal/workflow"
)

const (
	selectedProjectSetting   = "selected_project_id"
	managerMaxAnswerBytes    = 12 * 1024
	managerMaxStreamEvents   = 1800
	repetitionLoopMinRepeats = 8
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
}

type ProjectState struct {
	Project       ProjectDTO        `json:"project"`
	Task          *TaskDTO          `json:"task,omitempty"`
	Messages      []MessageDTO      `json:"messages"`
	WorkflowRun   *WorkflowRunDTO   `json:"workflowRun,omitempty"`
	WorkflowSteps []WorkflowStepDTO `json:"workflowSteps"`
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

type SendMessageInput struct {
	ProjectID string `json:"projectId"`
	Content   string `json:"content"`
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

type ProjectDTO = project.Project
type TaskDTO = chat.Task
type MessageDTO = chat.Message
type ModelConfigDTO = llm.ModelConfig
type WorkflowRunDTO = zw.Run
type WorkflowStepDTO = zw.Step

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

	if _, err := s.store.GetProject(ctx, input.ProjectID); err != nil {
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

	run, err := s.store.CreateWorkflowRun(ctx, task.ID)
	if err != nil {
		return ChatState{}, err
	}
	s.emitWorkflowRun(run)

	history, err := s.store.ListMessages(ctx, task.ID)
	if err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, run.CurrentStep, err.Error())
		return ChatState{}, err
	}

	provider := openaiapi.NewClient(model.BaseURL, model.APIKeyRef)

	result, err := s.runV03Workflow(ctx, input.ProjectID, task.ID, &run, history, provider, model)
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
		if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, result.Clarification); err != nil {
			return ChatState{}, err
		}
		_ = s.store.TouchTask(ctx, task.ID)
		return s.emitChatState(ctx, input.ProjectID, ""), nil
	}

	if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, result.Final); err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepManagerFinal, err.Error())
		s.setAgentStatus(agents.ManagerID, "failed", "Не удалось сохранить итоговый ответ", model.ID)
		return ChatState{}, err
	}
	_ = s.store.TouchTask(ctx, task.ID)
	if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusDone, zw.StepManagerFinal, ""); err != nil {
		return ChatState{}, err
	}

	run.Status = zw.StatusDone
	run.CurrentStep = zw.StepManagerFinal
	run.FinishedAt = nowString()
	s.emitWorkflowRun(run)
	s.resetAgentStatuses(model.ID)
	return s.emitChatState(ctx, input.ProjectID, ""), nil
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

type v03WorkflowResult struct {
	Intake             string
	Clarification      string
	Product            string
	Architect          string
	Final              string
	NeedsClarification bool
}

func (s *Service) runV03Workflow(
	ctx context.Context,
	projectID string,
	taskID string,
	run *zw.Run,
	history []chat.Message,
	provider llm.Provider,
	model llm.ModelConfig,
) (v03WorkflowResult, error) {
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
			result.Clarification = formatClarificationMessage(intakeResult)
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

	architectInput := buildArchitectInput(intake, product)
	architect, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepArchitectPlan, architectInput)
	if err != nil {
		return result, err
	}
	result.Architect = architect

	finalInput := buildManagerFinalInput(intake, product, architect)
	final, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepManagerFinal, finalInput)
	if err != nil {
		return result, err
	}
	result.Final = final
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
		s.setAgentStatus(spec.ID, "failed", "Пустой ответ модели", model.ID)
		return "", err
	}
	if len(output) > managerMaxAnswerBytes || looksLikeRepetitionLoop(output) {
		err := errors.New("ответ остановлен: модель начала повторять или генерировать слишком длинный текст")
		failed, finishErr := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusFailed, output, err.Error())
		if finishErr == nil {
			s.emitWorkflowStep(*run, failed)
		}
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
	s.setAgentStatus(spec.ID, "done", stepDoneActivity(stepKey), model.ID)
	s.emitChatState(ctx, projectID, "")
	_ = taskID
	return output, nil
}

func (s *Service) AgentStatuses() []agents.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := []string{agents.ManagerID, agents.ProductID, agents.ArchitectID}
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
		}
	}
	return ProjectState{
		Project:       item,
		Task:          task,
		Messages:      messages,
		WorkflowRun:   workflowRun,
		WorkflowSteps: workflowSteps,
	}, nil
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
	return builder.String()
}

func buildProductInput(intake string) string {
	return strings.TrimSpace(`
Task brief от Менеджера:
` + intake + `

Сделай требования для реализации. Не добавляй технический план глубже продуктового уровня.
`)
}

func buildArchitectInput(intake string, product string) string {
	return strings.TrimSpace(`
Task brief от Менеджера:
` + intake + `

Требования от Продакта:
` + product + `

Сделай технический план реализации. Не утверждай, что файлы проекта уже прочитаны или изменены.
`)
}

func buildManagerFinalInput(intake string, product string, architect string) string {
	return strings.TrimSpace(`
Task brief:
` + intake + `

Требования:
` + product + `

Архитектурный план:
` + architect + `

Собери итоговый ответ пользователю. Он должен быть коротким и практичным.
`)
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

func formatClarificationMessage(intake managerIntakeResult) string {
	var builder strings.Builder
	summary := strings.TrimSpace(intake.Summary)
	goal := strings.TrimSpace(intake.Goal)
	if summary == "" {
		summary = "нужно уточнить детали задачи перед передачей Продакту и Архитектору."
	}

	builder.WriteString("Понял задачу: ")
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

func stepThinkingActivity(stepKey string) string {
	switch stepKey {
	case zw.StepProductRequirements:
		return "Формирует требования"
	case zw.StepArchitectPlan:
		return "Проектирует технический план"
	case zw.StepManagerFinal:
		return "Собирает итоговый ответ"
	default:
		return "Разбирает задачу пользователя"
	}
}

func stepDoneActivity(stepKey string) string {
	switch stepKey {
	case zw.StepProductRequirements:
		return "Требования готовы"
	case zw.StepArchitectPlan:
		return "Архитектурный план готов"
	case zw.StepManagerFinal:
		return "Итоговый ответ готов"
	default:
		return "Task brief готов"
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
