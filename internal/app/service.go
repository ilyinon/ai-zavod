package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/agents"
	"zavod_ai/internal/artifacts"
	"zavod_ai/internal/blueprint"
	"zavod_ai/internal/changes"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/config"
	"zavod_ai/internal/ctf"
	"zavod_ai/internal/devworkspace"
	lifecycler "zavod_ai/internal/lifecycle"
	"zavod_ai/internal/lifecycleruntime"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/orchestration"
	"zavod_ai/internal/project"
	"zavod_ai/internal/projectmemory"
	"zavod_ai/internal/providers/openaiapi"
	"zavod_ai/internal/repairloop"
	"zavod_ai/internal/reviewgate"
	"zavod_ai/internal/reviews"
	"zavod_ai/internal/router"
	"zavod_ai/internal/store"
	"zavod_ai/internal/taskspec"
	"zavod_ai/internal/toolruntime"
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
	chatWork      sync.Mutex
	chatBusy      map[string]bool
	projectQueues map[string]chan struct{}
}

type BootstrapState struct {
	Chats               []TaskDTO               `json:"chats"`
	Paths               config.Paths            `json:"paths"`
	Projects            []ProjectDTO            `json:"projects"`
	SelectedProjectID   string                  `json:"selectedProjectId"`
	Chat                ProjectState            `json:"chat"`
	Agents              []agents.Status         `json:"agents"`
	Models              []ModelConfigDTO        `json:"models"`
	ActiveModelID       string                  `json:"activeModelId"`
	WebSettings         WebSettingsDTO          `json:"webSettings"`
	AgentGroups         []AgentGroupDTO         `json:"agentGroups"`
	AgentGroupTemplates []AgentGroupTemplateDTO `json:"agentGroupTemplates"`
	AgentLibrary        []AgentLibraryItemDTO   `json:"agentLibrary"`
}

type ProjectState struct {
	ToolInvocations []toolruntime.Invocation `json:"toolInvocations"`
	Project         ProjectDTO               `json:"project"`
	Task            *TaskDTO                 `json:"task,omitempty"`
	Messages        []MessageDTO             `json:"messages"`
	WorkflowRun     *WorkflowRunDTO          `json:"workflowRun,omitempty"`
	WorkflowSteps   []WorkflowStepDTO        `json:"workflowSteps"`
	WorkflowPlan    *WorkflowPlanDTO         `json:"workflowPlan,omitempty"`
	PlanSteps       []WorkflowPlanStepDTO    `json:"planSteps"`
	Artifacts       []ArtifactDTO            `json:"artifacts"`
	Blueprint       *BlueprintDTO            `json:"blueprint,omitempty"`
	Clarification   *ClarificationDTO        `json:"clarification,omitempty"`
	TaskSpec        *TaskSpecDTO             `json:"taskSpec,omitempty"`
	ProjectMemory   *ProjectMemoryDTO        `json:"projectMemory,omitempty"`
	Changes         []ProposedChangeDTO      `json:"changes"`
	TestRuns        []TestRunDTO             `json:"testRuns"`
	Reviews         []ReviewRunDTO           `json:"reviews"`
	WebSources      []WebSourceDTO           `json:"webSources"`
	CTFWorkspace    *CTFWorkspaceDTO         `json:"ctfWorkspace,omitempty"`
	AgentGroup      *AgentGroupDTO           `json:"agentGroup,omitempty"`
	GroupBinding    *ProjectGroupBindingDTO  `json:"groupBinding,omitempty"`
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
	Name        string `json:"name"`
	GroupID     string `json:"groupId"`
	LifecycleID string `json:"lifecycleId"`
}

type AddExistingProjectInput struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	GroupID     string `json:"groupId"`
	LifecycleID string `json:"lifecycleId"`
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
	ToolConsentModelID string `json:"toolConsentModelId"`
	TaskID             string `json:"taskId"`
	ProjectID          string `json:"projectId"`
	Content            string `json:"content"`
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

type RollbackWorkflowChangesInput struct {
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

type CreateAgentGroupInput struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	DefaultModelID string `json:"defaultModelId"`
}

type UpdateAgentGroupInput struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	DefaultModelID string `json:"defaultModelId"`
}

type ArchiveAgentGroupInput struct {
	GroupID string `json:"groupId"`
}

type SaveAgentProfileInput struct {
	ID            string   `json:"id"`
	GroupID       string   `json:"groupId"`
	Name          string   `json:"name"`
	RoleKey       string   `json:"roleKey"`
	Description   string   `json:"description"`
	AvatarPath    string   `json:"avatarPath"`
	SoulPath      string   `json:"soulPath"`
	ModelID       string   `json:"modelId"`
	ToolProfileID string   `json:"toolProfileId"`
	DefaultSkills []string `json:"defaultSkills"`
	Capabilities  []string `json:"capabilities"`
	AllowedTools  []string `json:"allowedTools"`
	ReadPaths     []string `json:"readPaths"`
	WritePaths    []string `json:"writePaths"`
	HandoffRules  []string `json:"handoffRules"`
	Temperature   float64  `json:"temperature"`
	ContextBudget int      `json:"contextBudget"`
	Enabled       bool     `json:"enabled"`
	SortOrder     int      `json:"sortOrder"`
}

type AgentSoulDTO struct {
	ProfileID string   `json:"profileId"`
	Path      string   `json:"path"`
	Content   string   `json:"content"`
	Warnings  []string `json:"warnings"`
}

type SaveAgentSoulInput struct {
	ProfileID string `json:"profileId"`
	Content   string `json:"content"`
}

type SetAgentProfileEnabledInput struct {
	ProfileID string `json:"profileId"`
	Enabled   bool   `json:"enabled"`
}

type BindProjectAgentGroupInput struct {
	ProjectID   string `json:"projectId"`
	GroupID     string `json:"groupId"`
	LifecycleID string `json:"lifecycleId"`
}

type SaveLifecycleDefinitionInput struct {
	ID                  string `json:"id"`
	GroupID             string `json:"groupId"`
	Name                string `json:"name"`
	Kind                string `json:"kind"`
	Description         string `json:"description"`
	MaxTotalIterations  int    `json:"maxTotalIterations"`
	MaxRepairIterations int    `json:"maxRepairIterations"`
	SameErrorLimit      int    `json:"sameErrorLimit"`
	Status              string `json:"status"`
}

type SaveLifecycleStepInput struct {
	ID               string `json:"id"`
	LifecycleID      string `json:"lifecycleId"`
	StepKey          string `json:"stepKey"`
	Title            string `json:"title"`
	AgentProfileID   string `json:"agentProfileId"`
	Mode             string `json:"mode"`
	Required         bool   `json:"required"`
	CanRetry         bool   `json:"canRetry"`
	MaxRetries       int    `json:"maxRetries"`
	OnSuccessStepKey string `json:"onSuccessStepKey"`
	OnFailureStepKey string `json:"onFailureStepKey"`
	OutputSchema     string `json:"outputSchema"`
	VisibleToUser    bool   `json:"visibleToUser"`
	SortOrder        int    `json:"sortOrder"`
}

type DeleteLifecycleStepInput struct {
	StepID      string `json:"stepId"`
	LifecycleID string `json:"lifecycleId"`
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
type TaskSpecDTO = taskspec.Spec
type ProjectMemoryDTO = projectmemory.Memory
type PendingClarificationDTO = ClarificationDTO
type ProposedChangeDTO = changes.ProposedChange
type TestRunDTO = checks.TestRun
type ReviewRunDTO = reviews.ReviewRun
type WebSourceDTO = webresearch.Source

type CTFWorkspaceDTO struct {
	Title          string              `json:"title"`
	Category       string              `json:"category"`
	ScopeStatus    string              `json:"scopeStatus"`
	Root           string              `json:"root"`
	ArtifactsDir   string              `json:"artifactsDir"`
	EvidenceDir    string              `json:"evidenceDir"`
	EvidenceIndex  string              `json:"evidenceIndex"`
	EvidenceEvents string              `json:"evidenceEvents"`
	SolveDir       string              `json:"solveDir"`
	WriteupPath    string              `json:"writeupPath"`
	Challenge      CTFWorkspaceSection `json:"challenge"`
	Scope          CTFWorkspaceSection `json:"scope"`
	Artifacts      CTFWorkspaceSection `json:"artifacts"`
	Hypotheses     CTFWorkspaceSection `json:"hypotheses"`
	Attempts       CTFWorkspaceSection `json:"attempts"`
	Evidence       CTFWorkspaceSection `json:"evidence"`
	Solver         CTFWorkspaceSection `json:"solver"`
	Writeup        CTFWorkspaceSection `json:"writeup"`
	Files          []CTFWorkspaceFile  `json:"files"`
}

type CTFWorkspaceSection struct {
	Title   string `json:"title"`
	Status  string `json:"status"`
	Content string `json:"content"`
	Path    string `json:"path"`
	AgentID string `json:"agentId"`
}

type CTFWorkspaceFile struct {
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	RelativePath string `json:"relativePath"`
}
type WebSettingsDTO = webresearch.Settings
type AgentGroupDTO = agentgroups.Group
type AgentProfileDTO = agentgroups.Profile
type LifecycleDefinitionDTO = agentgroups.LifecycleDefinition
type LifecycleStepDTO = agentgroups.LifecycleStep
type LifecycleRuntimeIssueDTO = lifecycler.ValidationIssue
type ProjectGroupBindingDTO = agentgroups.ProjectBinding

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

	activeModel, err := db.ActiveModelConfig(ctx)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	service := &Service{
		store:         db,
		paths:         paths,
		sink:          sink,
		agentStatuses: map[string]agents.Status{},
	}
	if err := db.EnsureDefaultAgentGroups(ctx, activeModel.ID); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := service.ensureDefaultAgentSouls(ctx); err != nil {
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
	agentGroups, err := s.store.ListAgentGroups(ctx, false)
	if err != nil {
		return BootstrapState{}, err
	}
	agentGroupTemplates, err := s.ListAgentGroupTemplates(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	agentLibrary, err := s.ListAgentLibrary(ctx)
	if err != nil {
		return BootstrapState{}, err
	}

	selectedID := ""
	state := ProjectState{}

	chats, err := s.store.ListChats(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	selectedChatID, _, _ := s.store.GetSetting(ctx, "selected_chat_id")
	if selectedChatID != "" {
		if selected, selectErr := s.SelectChat(ctx, selectedChatID); selectErr == nil {
			state = selected
			selectedID = selected.Project.ID
		}
	} else {
		state = ProjectState{}
		selectedID = ""
	}
	return BootstrapState{
		Chats:               chats,
		Paths:               s.paths,
		Projects:            projects,
		SelectedProjectID:   selectedID,
		Chat:                state,
		Agents:              s.AgentStatuses(),
		Models:              models,
		ActiveModelID:       activeModel.ID,
		WebSettings:         webSettings,
		AgentGroups:         agentGroups,
		AgentGroupTemplates: agentGroupTemplates,
		AgentLibrary:        agentLibrary,
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
	groupID, lifecycleID, err := s.resolveProjectGroupChoice(ctx, input.GroupID, input.LifecycleID)
	if err != nil {
		return ProjectDTO{}, err
	}

	path := uniqueProjectPath(s.paths.ProjectsDir, safeProjectDirName(name))
	if err := os.MkdirAll(path, 0o755); err != nil {
		return ProjectDTO{}, err
	}

	item, err := s.store.CreateProject(ctx, name, path)
	if err != nil {
		return ProjectDTO{}, err
	}
	if _, err := s.store.BindProjectToAgentGroup(ctx, item.ID, groupID, lifecycleID); err != nil {
		return ProjectDTO{}, err
	}
	_, _ = s.ensureProjectMemory(ctx, item)
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
	groupID, lifecycleID, err := s.resolveProjectGroupChoice(ctx, input.GroupID, input.LifecycleID)
	if err != nil {
		return ProjectDTO{}, err
	}

	item, err := s.store.CreateProject(ctx, name, path)
	if err != nil {
		return ProjectDTO{}, err
	}
	if _, err := s.store.BindProjectToAgentGroup(ctx, item.ID, groupID, lifecycleID); err != nil {
		return ProjectDTO{}, err
	}
	_, _ = s.ensureProjectMemory(ctx, item)
	_ = s.store.SetSetting(ctx, selectedProjectSetting, item.ID)
	return item, nil
}

func (s *Service) resolveProjectGroupChoice(ctx context.Context, groupID string, lifecycleID string) (string, string, error) {
	groupID = strings.TrimSpace(groupID)
	lifecycleID = strings.TrimSpace(lifecycleID)
	if groupID == "" {
		groupID = "group_dev_squad"
	}
	group, err := s.store.GetAgentGroup(ctx, groupID)
	if err != nil {
		return "", "", fmt.Errorf("группа проекта не найдена: %w", err)
	}
	if group.Status == agentgroups.StatusArchived {
		return "", "", fmt.Errorf("нельзя создать проект с архивной группой %s", group.Name)
	}
	if lifecycleID == "" {
		lifecycleID = group.DefaultLifecycleID
	}
	if lifecycleID != "" {
		lifecycle, err := s.store.GetLifecycleDefinition(ctx, lifecycleID)
		if err != nil {
			return "", "", fmt.Errorf("lifecycle группы не найден: %w", err)
		}
		if lifecycle.GroupID != group.ID {
			return "", "", fmt.Errorf("lifecycle не принадлежит выбранной группе")
		}
	}
	return group.ID, lifecycleID, nil
}

func (s *Service) UpdateProject(ctx context.Context, input UpdateProjectInput) (ProjectDTO, error) {
	unlock, err := s.lockProjectEdit(ctx, input.ProjectID)
	if err != nil {
		return ProjectDTO{}, err
	}
	defer unlock()
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
	_, _ = s.ensureProjectMemory(ctx, item)
	return item, nil
}

func (s *Service) DeleteProject(ctx context.Context, input DeleteProjectInput) (BootstrapState, error) {
	unlock, err := s.lockProjectEdit(ctx, input.ProjectID)
	if err != nil {
		return BootstrapState{}, err
	}
	defer unlock()
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
	tasks, err := s.store.ListChats(ctx)
	if err != nil {
		return ProjectState{}, err
	}
	for _, task := range tasks {
		if task.ProjectID == projectID && task.Status == "active" {
			return s.SelectChat(ctx, task.ID)
		}
	}
	return s.CreateChat(ctx, CreateChatInput{ProjectID: projectID})
}

func (s *Service) SendMessage(ctx context.Context, input SendMessageInput) (ChatState, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return ChatState{}, fmt.Errorf("сообщение пустое")
	}

	task, err := s.messageTask(ctx, input)
	if err != nil {
		return ChatState{}, err
	}
	ctx = context.WithValue(ctx, chatContextKey{}, task.ID)
	input.ProjectID = task.ProjectID
	release, err := s.acquireChatWork(ctx, *task)
	if err != nil {
		return ChatState{}, err
	}
	defer release()
	currentProject := project.Project{}
	if task.ProjectID != "" {
		currentProject, err = s.store.GetProject(ctx, task.ProjectID)
		if err != nil {
			return ChatState{}, err
		}
		_ = s.store.TouchProject(ctx, task.ProjectID)
		_, _ = s.ensureProjectMemory(ctx, currentProject)
	}
	if task.Title == "Новый чат" {
		task.Title = titleFromContent(content)
	}
	resumingWorkspace := task.PendingRequest == content && task.ProjectID != ""
	task.PendingRequest = ""
	if _, err := s.store.UpdateChat(ctx, *task); err != nil {
		return ChatState{}, err
	}
	if !resumingWorkspace {
		if _, err := s.store.AddMessage(ctx, task.ID, "user", "", content); err != nil {
			return ChatState{}, err
		}
	}
	_ = s.store.TouchTask(ctx, task.ID)
	s.emitChatState(ctx, input.ProjectID, "")

	model, err := s.store.ActiveModelConfig(ctx)
	if task.ModelID != "" {
		model, err = s.store.GetModelConfig(ctx, task.ModelID)
	}
	if err != nil {
		return ChatState{}, err
	}
	provider := openaiapi.NewClient(model.BaseURL, model.APIKeyRef)
	ctx = s.chatGroupContext(ctx, *task, task.GroupID)

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
	orch := s.orchestrateMessage(ctx, currentProject, *task, content, decision, model.ID)
	if task.GroupID != "" {
		if group, err := s.store.GetAgentGroup(ctx, task.GroupID); err == nil {
			orch.GroupID, orch.LifecycleID = group.ID, group.DefaultLifecycleID
		}
	}
	decision.NeedsWorkflow = orch.NeedsWorkflow
	decision.NeedsProjectContext = orch.NeedsProjectContext
	s.emitChatState(ctx, input.ProjectID, "")
	if task.ProjectID == "" && decision.Intent == router.IntentResearchTask {
		return s.researchWithoutProject(ctx, *task, provider, model, content)
	}
	if task.ProjectID == "" && orch.NeedsWorkflow {
		task.PendingRequest = content
		if _, err := s.store.UpdateChat(ctx, *task); err != nil {
			return ChatState{}, err
		}
		s.resetAgentStatuses(model.ID)
		return s.emitChatState(ctx, "", ""), nil
	}
	if !orch.NeedsWorkflow {
		return s.answerDirect(ctx, currentProject, *task, latestRun, provider, model, content, decision, orch, input.ToolConsentModelID)
	}
	ctx = s.chatGroupContext(ctx, *task, orch.GroupID)
	useCTFWorkflow := decision.Intent == router.IntentPentestTask && s.shouldUseCTFWorkflow(ctx, input.ProjectID, content)

	resumingRun := decision.Intent == router.IntentClarificationAnswer && latestRun != nil && latestRun.Status == zw.StatusWaitingUser
	var run zw.Run
	if resumingRun {
		run = *latestRun
		run.Status = zw.StatusRunning
		if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusRunning, run.CurrentStep, ""); err != nil {
			return ChatState{}, err
		}
	} else {
		createdRun, err := s.store.CreateWorkflowRun(ctx, task.ID)
		if err != nil {
			return ChatState{}, err
		}
		run = createdRun
	}
	if decision.Intent == router.IntentClarificationAnswer {
		_ = s.patchTaskSpec(ctx, taskspec.Spec{
			ProjectID:     currentProject.ID,
			TaskID:        task.ID,
			WorkflowRunID: run.ID,
			Status:        taskspec.StatusActive,
			Source:        "clarification",
		}, false)
	} else {
		_ = s.resetTaskSpecForWorkflow(ctx, currentProject, *task, run.ID, content)
	}
	s.emitWorkflowRun(run)
	if !resumingRun {
		_ = s.createDynamicWorkflowPlan(ctx, currentProject, *task, run.ID, provider, model, content, decision.Intent)
	}

	history, err := s.store.ListMessages(ctx, task.ID)
	if err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, run.CurrentStep, err.Error())
		return ChatState{}, err
	}
	if decision.Intent == router.IntentPentestTask {
		if useCTFWorkflow {
			return s.runCTFWorkflow(ctx, input.ProjectID, currentProject, *task, &run, history, provider, model, content)
		}
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
		clarificationStep := firstNonEmpty(result.ClarificationStep, zw.StepManagerIntake)
		if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusWaitingUser, clarificationStep, ""); err != nil {
			return ChatState{}, err
		}
		run.Status = zw.StatusWaitingUser
		run.CurrentStep = clarificationStep
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
	_ = s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		Status:        taskSpecStatusFromWorkflow(finalStatus),
		Source:        "workflow_final",
	}, false)
	_ = s.updateProjectMemoryFromWorkflow(ctx, currentProject.ID, task.ID, run.ID, result, autoResult)
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
	orch orchestration.Decision,
	consentModelID string,
) (ChatState, error) {
	s.setAgentStatus(agents.ManagerID, "answering", directAnswerActivity(decision.Intent), model.ID)
	s.emitChatState(ctx, currentProject.ID, "")

	if wantsSavedTaskSpec(content) {
		answer := s.savedTaskSpecAnswer(ctx, currentProject, &task)
		if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, answer); err != nil {
			s.setAgentStatus(agents.ManagerID, "failed", "Не удалось сохранить спеку", model.ID)
			return ChatState{}, err
		}
		_ = s.store.TouchTask(ctx, task.ID)
		s.resetAgentStatuses(model.ID)
		return s.emitChatState(ctx, currentProject.ID, ""), nil
	}
	if wantsProjectMemory(content) {
		answer := s.projectMemoryAnswer(ctx, currentProject)
		if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, answer); err != nil {
			s.setAgentStatus(agents.ManagerID, "failed", "Не удалось сохранить память проекта", model.ID)
			return ChatState{}, err
		}
		_ = s.store.TouchTask(ctx, task.ID)
		s.resetAgentStatuses(model.ID)
		return s.emitChatState(ctx, currentProject.ID, ""), nil
	}

	answerCtx, cancel := context.WithTimeout(ctx, 180*time.Second)
	defer cancel()
	req := llm.Request{
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
				Content: s.buildDirectAnswerInput(ctx, currentProject, task, latestRun, content, decision, orch),
			},
		},
		Temperature: 0.2,
		MaxTokens:   1200,
	}
	var resp *llm.Response
	var err error
	if currentProject.ID != "" && decision.Intent == router.IntentProjectAnalysis {
		resp, err = s.generateProjectAnswer(answerCtx, currentProject, task.ID, provider, model, req, consentModelID)
	} else {
		resp, err = provider.Generate(answerCtx, req)
	}
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
	if lifecycleSteps, ok := s.lifecyclePlanSteps(ctx, currentProject, workflowRunID, intent); ok {
		steps = lifecycleSteps
		if intent == router.IntentPentestTask && s.shouldUseCTFWorkflow(ctx, currentProject.ID, userMessage) {
			_, _, err := s.store.CreateWorkflowPlan(ctx, plan, steps)
			if err == nil {
				s.emitChatState(ctx, currentProject.ID, "")
			}
			return err
		}
	}
	if intent == router.IntentResearchTask {
		_, _, err := s.store.CreateWorkflowPlan(ctx, plan, steps)
		if err == nil {
			s.emitChatState(ctx, currentProject.ID, "")
		}
		return err
	}

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

func (s *Service) lifecycleExecutor(ctx context.Context, projectID string) (lifecycler.Executor, bool) {
	definition, steps, err := s.store.ProjectLifecycle(ctx, projectID)
	if err != nil || len(steps) == 0 {
		return lifecycler.Executor{}, false
	}
	return lifecycler.NewExecutor(definition, steps), true
}

func (s *Service) lifecyclePlanSteps(ctx context.Context, currentProject project.Project, workflowRunID string, intent router.Intent) ([]zw.PlanStep, bool) {
	executor, ok := s.lifecycleExecutor(ctx, currentProject.ID)
	if !ok {
		return nil, false
	}
	binding, err := s.store.ProjectGroupBinding(ctx, currentProject.ID)
	if err != nil {
		return nil, false
	}
	group, err := s.store.GetAgentGroup(ctx, binding.GroupID)
	if err != nil {
		return nil, false
	}
	if intent == router.IntentResearchTask && group.Kind != agentgroups.GroupKindResearch {
		return nil, false
	}
	if intent == router.IntentPentestTask && group.Kind != agentgroups.GroupKindCTF && group.Kind != agentgroups.GroupKindSecurity {
		return nil, false
	}
	profiles, err := s.store.ListAgentProfiles(ctx, binding.GroupID)
	if err != nil {
		return nil, false
	}
	profileByID := make(map[string]agentgroups.Profile, len(profiles))
	for _, profile := range profiles {
		profileByID[profile.ID] = profile
	}
	steps := executor.Steps()
	out := make([]zw.PlanStep, 0, len(steps))
	for _, step := range steps {
		if !step.VisibleToUser {
			continue
		}
		profile := profileByID[step.AgentProfileID]
		out = append(out, zw.PlanStep{
			StepKey:     step.StepKey,
			Title:       step.Title,
			Description: lifecycleStepDescription(step, profile),
			AgentID:     normalizePlanAgent(profile.RoleKey),
			Status:      zw.StepStatusQueued,
			SortOrder:   len(out),
		})
	}
	_ = intent
	_ = workflowRunID
	return out, len(out) > 0
}

func lifecycleStepDescription(step agentgroups.LifecycleStep, profile agentgroups.Profile) string {
	parts := []string{}
	if profile.Name != "" {
		parts = append(parts, profile.Name)
	}
	if step.Mode != "" {
		parts = append(parts, step.Mode)
	}
	if cfg, err := lifecycler.ParseStepRuntimeConfig(step.OutputSchema); err == nil {
		if len(cfg.Conditions) > 0 || cfg.Condition.Field != "" || cfg.Condition.Operator != "" {
			parts = append(parts, "condition")
		}
		if len(cfg.Branches) > 0 {
			parts = append(parts, fmt.Sprintf("branches %d", len(cfg.Branches)))
		}
		if len(cfg.ParallelSteps) > 0 {
			parts = append(parts, fmt.Sprintf("parallel %d", len(cfg.ParallelSteps)))
		}
		if cfg.JoinStepKey != "" {
			parts = append(parts, "join: "+cfg.JoinStepKey)
		}
		if cfg.ReturnToStepKey != "" {
			parts = append(parts, "return: "+cfg.ReturnToStepKey)
		}
		if len(cfg.CompletionRules) > 0 {
			parts = append(parts, "completion")
		}
	}
	if step.CanRetry {
		parts = append(parts, fmt.Sprintf("retries %d", step.MaxRetries))
	}
	if step.OnFailureStepKey != "" {
		parts = append(parts, "fallback: "+step.OnFailureStepKey)
	}
	return strings.Join(parts, " · ")
}

func (s *Service) shouldUseCTFWorkflow(ctx context.Context, projectID string, message string) bool {
	binding, err := s.store.ProjectGroupBinding(ctx, projectID)
	if err != nil {
		return false
	}
	group, err := s.store.GetAgentGroup(ctx, binding.GroupID)
	if err != nil {
		return false
	}
	return group.Kind == agentgroups.GroupKindCTF && ctf.IsCTFRequest(message)
}

func (s *Service) orchestrateMessage(
	ctx context.Context,
	currentProject project.Project,
	task chat.Task,
	message string,
	decision router.Decision,
	modelID string,
) orchestration.Decision {
	groups, _ := s.store.ListAgentGroups(ctx, false)
	var currentGroup *agentgroups.Group
	if binding, err := s.store.ProjectGroupBinding(ctx, currentProject.ID); err == nil {
		if group, groupErr := s.store.GetAgentGroup(ctx, binding.GroupID); groupErr == nil {
			currentGroup = &group
		}
	}
	hasSpec := false
	if spec, err := s.store.LatestTaskSpecByTask(ctx, task.ID); err == nil && strings.TrimSpace(spec.ID) != "" {
		hasSpec = true
	}
	hasMemory := false
	if memory, err := s.store.ProjectMemory(ctx, currentProject.ID); err == nil {
		hasMemory = strings.TrimSpace(memory.ID) != ""
	}
	result := orchestration.Decide(orchestration.Input{
		Message:          message,
		RouterDecision:   decision,
		CurrentGroup:     currentGroup,
		AvailableGroups:  groups,
		HasTaskSpec:      hasSpec,
		HasProjectMemory: hasMemory,
	})
	s.setAgentRuntimeStatus(
		agents.ManagerID,
		"orchestrating",
		shortenForPrompt(orchestrationActivity(result), 160),
		modelID,
		agentRuntimeStatusUpdate{
			StepKey: "orchestration",
		},
	)
	return result
}

func orchestrationActivity(decision orchestration.Decision) string {
	if strings.TrimSpace(decision.GroupName) == "" {
		if decision.Mode == orchestration.ModeWorkflow {
			return "Выбирает workflow"
		}
		return "Выбирает прямой ответ"
	}
	if decision.Mode == orchestration.ModeWorkflow {
		return "Выбрала workflow: " + decision.GroupName
	}
	return "Выбрала прямой ответ: " + decision.GroupName
}

func (s *Service) maxRepairIterationsForProject(ctx context.Context, projectID string) int {
	executor, ok := s.lifecycleExecutor(ctx, projectID)
	if !ok {
		return maxAutoRepairIterations
	}
	limit := executor.Definition().MaxRepairIterations
	if limit <= 0 {
		return maxAutoRepairIterations
	}
	return limit
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
	case agents.ResearcherID:
		return agents.ResearcherID
	case agents.SourceReviewID:
		return agents.SourceReviewID
	case agents.AnalystID:
		return agents.AnalystID
	case agents.CTFScoutID, "scout":
		return agents.CTFScoutID
	case agents.CTFWebID:
		return agents.CTFWebID
	case agents.CTFLFIID:
		return agents.CTFLFIID
	case agents.CTFRCEID:
		return agents.CTFRCEID
	case agents.CTFSQLiID:
		return agents.CTFSQLiID
	case agents.CTFPwnID:
		return agents.CTFPwnID
	case agents.CTFCryptoID:
		return agents.CTFCryptoID
	case agents.CTFReverseID:
		return agents.CTFReverseID
	case agents.CTFForensicsID:
		return agents.CTFForensicsID
	case agents.CTFValidatorID, "validator":
		return agents.CTFValidatorID
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
			{StepKey: zw.StepWebResearch, Title: "Поиск", Description: "Сформировать запросы и собрать публичные источники", AgentID: agents.ResearcherID},
			{StepKey: zw.StepResearchSourceReview, Title: "Источники", Description: "Проверить свежесть, доверие и противоречия", AgentID: agents.SourceReviewID},
			{StepKey: zw.StepResearchSynthesis, Title: "Аналитика", Description: "Сравнить источники и отделить факты от выводов", AgentID: agents.AnalystID},
			{StepKey: zw.StepResearchNotes, Title: "Research notes", Description: "Сохранить заметки исследования в проект", AgentID: agents.ResearcherID},
			{StepKey: zw.StepManagerFinal, Title: "Итог", Description: "Дать короткий ответ со ссылками", AgentID: agents.ManagerID},
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
		_ = s.patchTaskSpec(ctx, taskspec.Spec{ProjectID: currentProject.ID, TaskID: task.ID, WorkflowRunID: run.ID, Status: taskspec.StatusBlocked, Decisions: []string{"Web research выключен в настройках."}, Source: "web_research"}, false)
		_ = s.patchProjectMemory(ctx, projectmemory.Memory{ProjectID: currentProject.ID, UpdatedFromTaskID: task.ID, Environment: []string{"Web research был выключен в настройках."}})
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

	s.setAgentStatus(agents.ResearcherID, "searching_web", "Ищет источники в интернете", model.ID)
	searchCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.TimeoutSeconds*settings.MaxPagesPerWorkflow+6)*time.Second)
	sources, searchErr := webresearch.NewClient(time.Duration(settings.TimeoutSeconds)*time.Second).Research(searchCtx, plan, settings)
	cancel()
	if searchErr != nil {
		message := fmt.Sprintf("## Не нашла источники\n\n%s", searchErr.Error())
		_, _ = s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, message)
		_ = s.patchTaskSpec(ctx, taskspec.Spec{ProjectID: currentProject.ID, TaskID: task.ID, WorkflowRunID: run.ID, Status: taskspec.StatusBlocked, Decisions: []string{"Источники не найдены: " + searchErr.Error()}, Source: "web_research"}, false)
		_ = s.patchProjectMemory(ctx, projectmemory.Memory{ProjectID: currentProject.ID, UpdatedFromTaskID: task.ID, Environment: []string{"Последний web research не нашел источники: " + searchErr.Error()}})
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepWebResearch, searchErr.Error())
		_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusBlocked, searchErr.Error())
		s.setAgentStatus(agents.ResearcherID, "failed", "Не нашла источники", model.ID)
		return s.emitChatState(ctx, projectID, ""), nil
	}

	for _, source := range sources {
		source.ProjectID = currentProject.ID
		source.TaskID = task.ID
		source.WorkflowRunID = run.ID
		source.AgentID = agents.ResearcherID
		_, _ = s.store.CreateWebSource(ctx, source)
	}

	sourceReview, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepResearchSourceReview, buildResearchSourceReviewInput(userMessage, currentProject, plan, sources))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepResearchSourceReview, model.ID, err), nil
	}
	synthesis, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepResearchSynthesis, buildResearchSynthesisInput(userMessage, currentProject, plan, sources, sourceReview))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepResearchSynthesis, model.ID, err), nil
	}
	notes, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepResearchNotes, buildResearchNotesInput(userMessage, currentProject, plan, sources, sourceReview, synthesis))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepResearchNotes, model.ID, err), nil
	}
	notesPath, notesErr := s.saveResearchNotes(ctx, currentProject, task, run.ID, notes)
	if notesErr != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepResearchNotes, model.ID, notesErr), nil
	}

	answer := strings.TrimSpace(synthesis)
	if answer == "" {
		answer = webResearchFallbackAnswer(sources)
	}
	if looksLikeRepetitionLoop(answer) || looksLikeRawJSONAnswer(answer) || len(answer) > managerMaxAnswerBytes {
		answer = webResearchFallbackAnswer(sources)
	}
	if notesPath != "" && !strings.Contains(answer, "research-notes.md") {
		answer += "\n\nЗаметки исследования сохранены: `" + notesPath + "`."
	}
	_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, zw.StepManagerFinal, agents.ManagerID, zw.StepStatusDone, "")
	_ = s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		Decisions:     []string{fmt.Sprintf("Research Squad выполнила поиск: найдено источников %d.", len(sources)), "Research notes сохранены: " + notesPath},
		Status:        taskspec.StatusDone,
		Source:        "research_squad",
	}, false)
	_ = s.patchProjectMemory(ctx, projectmemory.Memory{
		ProjectID:         currentProject.ID,
		UpdatedFromTaskID: task.ID,
		Decisions:         []string{fmt.Sprintf("Research Squad использовалась для проекта; последний успешный поиск нашел источников: %d.", len(sources)), "Research notes: " + notesPath},
	})
	if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.AnalystID, answer); err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepManagerFinal, err.Error())
		return ChatState{}, err
	}
	if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusDone, zw.StepManagerFinal, ""); err != nil {
		return ChatState{}, err
	}
	_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusDone, "")
	run.Status = zw.StatusDone
	run.CurrentStep = zw.StepManagerFinal
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
	_ = s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		Requirements:  []string{"Сформировать защитный ИБ-анализ по доступному проектному контексту."},
		Decisions:     []string{"ИБ-задача обработана отдельным security workflow."},
		Status:        taskspec.StatusDone,
		Source:        "security_workflow",
	}, false)
	_ = s.patchProjectMemory(ctx, projectmemory.Memory{
		ProjectID:         currentProject.ID,
		UpdatedFromTaskID: task.ID,
		Decisions:         []string{"Проект поддерживает отдельный security workflow для ИБ-задач."},
	})
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

func (s *Service) runCTFWorkflow(
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
	category := ctf.Classify(userMessage)
	workspace, err := ctf.PrepareWorkspace(currentProject.Path, task.Title, userMessage, category, time.Now())
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFIntake, model.ID, err), nil
	}
	_ = s.createCTFArtifacts(ctx, currentProject, task, run.ID, workspace)

	intake, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepCTFIntake, buildCTFIntakeInput(userMessage, currentProject, history, workspace))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFIntake, model.ID, err), nil
	}
	intakeEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFIntake, agents.CTFScoutID, "agent_output", "Intake", intake, nil)
	scopeOutput, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepCTFScopeCheck, buildCTFScopeInput(userMessage, currentProject, workspace, intake))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFScopeCheck, model.ID, err), nil
	}
	scopeEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFScopeCheck, agents.CTFScoutID, "agent_output", "Scope check", scopeOutput, nil)
	_ = ctf.AppendNotes(currentProject.Path, workspace, map[string]string{
		"Evidence": ctfEvidenceLinks(intakeEvidence, scopeEvidence),
	})

	if workspace.RequiresScope {
		answer := ctfScopeRequiredAnswer(workspace)
		_ = s.patchTaskSpec(ctx, taskspec.Spec{
			ProjectID:     currentProject.ID,
			TaskID:        task.ID,
			WorkflowRunID: run.ID,
			Requirements:  []string{"Перед активными сетевыми действиями нужен явный CTF/lab scope."},
			OpenQuestions: []string{"Подтверди scope и разрешенные цели для CTF/lab проверки."},
			Decisions:     []string{"CTF workflow остановлен на scope gate."},
			Status:        taskspec.StatusWaitingClarification,
			Source:        "ctf_scope",
		}, true)
		_ = s.patchProjectMemory(ctx, projectmemory.Memory{
			ProjectID:         currentProject.ID,
			UpdatedFromTaskID: task.ID,
			Decisions:         []string{"CTF workflow использует scope gate перед активными сетевыми действиями."},
			Environment:       []string{"CTF workspace: " + workspace.RelativeRoot},
		})
		if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.CTFScoutID, answer); err != nil {
			_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepCTFScopeCheck, err.Error())
			return ChatState{}, err
		}
		if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusWaitingUser, zw.StepCTFScopeCheck, "нужен scope для активных сетевых действий"); err != nil {
			return ChatState{}, err
		}
		_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusWaitingUser, "нужен scope")
		run.Status = zw.StatusWaitingUser
		run.CurrentStep = zw.StepCTFScopeCheck
		run.FinishedAt = nowString()
		run.Error = "нужен scope"
		s.emitWorkflowRun(*run)
		s.setAgentStatus(agents.CTFScoutID, "waiting_user", "Ждет подтверждение scope", model.ID)
		_ = s.store.TouchTask(ctx, task.ID)
		return s.emitChatState(ctx, projectID, ""), nil
	}

	artifactsOutput, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepCTFArtifactCollection, buildCTFArtifactInput(userMessage, currentProject, workspace, intake, scopeOutput))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFArtifactCollection, model.ID, err), nil
	}
	artifactsEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFArtifactCollection, agents.CTFScoutID, "found_file", "Artifact collection", artifactsOutput, nil)
	triageOutput, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepCTFTriage, buildCTFTriageInput(userMessage, currentProject, workspace, artifactsOutput))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFTriage, model.ID, err), nil
	}
	triageEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFTriage, agents.CTFScoutID, "agent_output", "Triage", triageOutput, nil)
	hypothesisOutput, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepCTFHypothesisBoard, buildCTFHypothesisInput(workspace, intake, scopeOutput, triageOutput))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFHypothesisBoard, model.ID, err), nil
	}
	hypothesisEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFHypothesisBoard, agents.CTFScoutID, "payload_note", "Hypotheses and payload notes", hypothesisOutput, nil)
	solverSpec := agents.CTFSolverSpec(category)
	solverOutput, err := s.runWorkflowStepWithSpec(ctx, projectID, task.ID, run, provider, model, zw.StepCTFCategorySolver, solverSpec, buildCTFSolverInput(userMessage, currentProject, workspace, intake, scopeOutput, artifactsOutput, triageOutput, hypothesisOutput))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFCategorySolver, model.ID, err), nil
	}
	solverEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFCategorySolver, solverSpec.ID, "solver_output", "Solver output", solverOutput, map[string]string{
		"tool_profile": ctf.ToolProfileID(workspace.Category),
	})
	validationOutput, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepCTFValidation, buildCTFValidationInput(workspace, solverOutput))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFValidation, model.ID, err), nil
	}
	validationEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFValidation, agents.CTFValidatorID, "validation", "Validation", validationOutput, nil)
	writeupOutput, err := s.runWorkflowStep(ctx, projectID, task.ID, run, provider, model, zw.StepCTFWriteup, buildCTFWriteupInput(workspace, intake, scopeOutput, artifactsOutput, triageOutput, hypothesisOutput, solverOutput, validationOutput))
	if err != nil {
		return s.handleWorkflowError(ctx, projectID, task.ID, run.ID, zw.StepCTFWriteup, model.ID, err), nil
	}
	writeupEvidence := s.recordCTFEvidenceStep(currentProject.Path, workspace, zw.StepCTFWriteup, agents.ManagerID, "agent_output", "Writeup draft", writeupOutput, nil)
	_ = ctf.AppendNotes(currentProject.Path, workspace, map[string]string{
		"Evidence": ctfEvidenceLinks(artifactsEvidence, triageEvidence, hypothesisEvidence, solverEvidence, validationEvidence, writeupEvidence),
	})
	_ = ctf.WriteWriteup(currentProject.Path, workspace, writeupOutput)

	answer := ctfDoneAnswer(workspace, solverSpec.Name)
	_ = s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: run.ID,
		Requirements:  []string{"Решить CTF challenge в выбранной категории и сохранить writeup."},
		Decisions:     []string{"CTF category: " + string(category), "Solver: " + solverSpec.Name, "Writeup сохранен: " + workspace.WriteupMD},
		Status:        taskspec.StatusDone,
		Source:        "ctf_workflow",
	}, false)
	_ = s.patchProjectMemory(ctx, projectmemory.Memory{
		ProjectID:         currentProject.ID,
		UpdatedFromTaskID: task.ID,
		Stack:             "ctf",
		Decisions:         []string{"CTF category: " + string(category), "CTF solver: " + solverSpec.Name, "CTF writeup path: " + workspace.WriteupMD},
		Environment:       []string{"CTF workspace: " + workspace.RelativeRoot},
	})
	if _, err := s.store.AddMessage(ctx, task.ID, "agent", agents.ManagerID, answer); err != nil {
		_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusFailed, zw.StepCTFWriteup, err.Error())
		return ChatState{}, err
	}
	if err := s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusDone, zw.StepCTFWriteup, ""); err != nil {
		return ChatState{}, err
	}
	_ = s.store.FinishWorkflowPlan(ctx, run.ID, zw.StatusDone, "")
	run.Status = zw.StatusDone
	run.CurrentStep = zw.StepCTFWriteup
	run.FinishedAt = nowString()
	run.Error = ""
	s.emitWorkflowRun(*run)
	_ = s.store.TouchTask(ctx, task.ID)
	s.resetAgentStatuses(model.ID)
	return s.emitChatState(ctx, projectID, ""), nil
}

func (s *Service) createCTFArtifacts(ctx context.Context, currentProject project.Project, task chat.Task, workflowRunID string, workspace ctf.Workspace) error {
	items := []struct {
		kind  string
		title string
		path  string
	}{
		{kind: "ctf_challenge", title: "CTF challenge", path: workspace.ChallengeYAML},
		{kind: "ctf_scope", title: "CTF scope", path: workspace.ScopeMD},
		{kind: "ctf_notes", title: "CTF notes", path: workspace.NotesMD},
		{kind: "ctf_evidence_index", title: "CTF evidence index", path: workspace.EvidenceIndex},
		{kind: "ctf_evidence_events", title: "CTF evidence events", path: workspace.EvidenceEvents},
		{kind: "ctf_writeup", title: "CTF writeup", path: workspace.WriteupMD},
	}
	for _, item := range items {
		_, err := s.store.CreateArtifact(ctx, artifacts.Artifact{
			ProjectID:     currentProject.ID,
			TaskID:        task.ID,
			WorkflowRunID: workflowRunID,
			AgentID:       agents.ManagerID,
			Kind:          item.kind,
			Title:         item.title,
			Path:          filepath.Join(currentProject.Path, item.path),
			RelativePath:  item.path,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) recordCTFEvidenceStep(projectPath string, workspace ctf.Workspace, stepKey string, agentID string, kind string, title string, content string, metadata map[string]string) ctf.EvidenceEntry {
	entry, err := ctf.RecordEvidence(projectPath, workspace, ctf.EvidenceEntry{
		Kind:     kind,
		Title:    title,
		AgentID:  agentID,
		StepKey:  stepKey,
		Source:   "workflow_step",
		Content:  content,
		Metadata: metadata,
	}, time.Now())
	if err != nil {
		return ctf.EvidenceEntry{}
	}
	return entry
}

func ctfEvidenceLinks(entries ...ctf.EvidenceEntry) string {
	var builder strings.Builder
	for _, entry := range entries {
		if strings.TrimSpace(entry.RelativePath) == "" {
			continue
		}
		title := strings.TrimSpace(entry.Title)
		if title == "" {
			title = entry.Kind
		}
		builder.WriteString("- ")
		builder.WriteString(title)
		builder.WriteString(": `")
		builder.WriteString(filepath.ToSlash(entry.RelativePath))
		builder.WriteString("`\n")
	}
	return strings.TrimSpace(builder.String())
}

func (s *Service) buildCTFWorkspaceState(currentProject project.Project, task *chat.Task, run *zw.Run, steps []zw.Step, artifactItems []artifacts.Artifact) *CTFWorkspaceDTO {
	if task == nil || run == nil {
		return nil
	}
	if !hasCTFWorkspaceData(steps, artifactItems) {
		return nil
	}
	stepByKey := map[string]zw.Step{}
	for _, step := range steps {
		stepByKey[step.StepKey] = step
	}
	root := ctfRootFromArtifacts(artifactItems)
	category := ctfCategoryFromState(currentProject, task, root, stepByKey)
	scopeStatus := ctfScopeStatusFromState(currentProject, root, stepByKey)
	files := ctfWorkspaceFiles(currentProject, root, artifactItems)

	return &CTFWorkspaceDTO{
		Title:          task.Title,
		Category:       category,
		ScopeStatus:    scopeStatus,
		Root:           root,
		ArtifactsDir:   ctfPathJoin(root, "artifacts"),
		EvidenceDir:    ctfPathJoin(root, "evidence"),
		EvidenceIndex:  ctfPathJoin(root, "evidence", "index.md"),
		EvidenceEvents: ctfPathJoin(root, "evidence", "events.jsonl"),
		SolveDir:       ctfPathJoin(root, "solve"),
		WriteupPath:    ctfPathJoin(root, "writeup.md"),
		Challenge:      ctfFileSection(currentProject, ctfPathJoin(root, "challenge.yml"), "Challenge", "ctf_challenge"),
		Scope:          ctfSectionFromFileOrStep(currentProject, ctfPathJoin(root, "scope.md"), "Scope", stepByKey[zw.StepCTFScopeCheck]),
		Artifacts:      ctfSectionFromStep("Артефакты", stepByKey[zw.StepCTFArtifactCollection]),
		Hypotheses:     ctfSectionFromStep("Гипотезы", stepByKey[zw.StepCTFHypothesisBoard]),
		Attempts:       ctfAttemptsSection(currentProject, root, stepByKey),
		Evidence:       ctfEvidenceSection(currentProject, root, stepByKey, files),
		Solver:         ctfSolverSection(currentProject, root, stepByKey[zw.StepCTFCategorySolver]),
		Writeup:        ctfSectionFromFileOrStep(currentProject, ctfPathJoin(root, "writeup.md"), "Writeup", stepByKey[zw.StepCTFWriteup]),
		Files:          files,
	}
}

func hasCTFWorkspaceData(steps []zw.Step, artifactItems []artifacts.Artifact) bool {
	for _, step := range steps {
		if isCTFStep(step.StepKey) {
			return true
		}
	}
	for _, item := range artifactItems {
		relative := filepath.ToSlash(strings.TrimSpace(item.RelativePath))
		if strings.HasPrefix(item.Kind, "ctf_") || strings.HasPrefix(relative, "ctf/") {
			return true
		}
	}
	return false
}

func isCTFStep(stepKey string) bool {
	switch stepKey {
	case zw.StepCTFIntake, zw.StepCTFScopeCheck, zw.StepCTFArtifactCollection, zw.StepCTFTriage, zw.StepCTFHypothesisBoard, zw.StepCTFCategorySolver, zw.StepCTFValidation, zw.StepCTFWriteup:
		return true
	default:
		return false
	}
}

func ctfRootFromArtifacts(items []artifacts.Artifact) string {
	for _, item := range items {
		relative := filepath.ToSlash(strings.TrimSpace(item.RelativePath))
		if !strings.HasPrefix(relative, "ctf/") {
			continue
		}
		parts := strings.Split(relative, "/")
		if len(parts) >= 2 && parts[1] != "" {
			return filepath.ToSlash(filepath.Join(parts[0], parts[1]))
		}
	}
	return ""
}

func ctfCategoryFromState(currentProject project.Project, task *chat.Task, root string, steps map[string]zw.Step) string {
	if value := ctfYAMLValue(readProjectRelativeFile(currentProject, ctfPathJoin(root, "challenge.yml")), "category"); value != "" {
		return value
	}
	var text strings.Builder
	if task != nil {
		text.WriteString(task.Title)
		text.WriteString("\n")
	}
	for _, key := range []string{zw.StepCTFIntake, zw.StepCTFTriage, zw.StepCTFCategorySolver} {
		text.WriteString(steps[key].Output)
		text.WriteString("\n")
	}
	return ctf.Classify(text.String())
}

func ctfScopeStatusFromState(currentProject project.Project, root string, steps map[string]zw.Step) string {
	if value := ctfYAMLValue(readProjectRelativeFile(currentProject, ctfPathJoin(root, "challenge.yml")), "scope_status"); value != "" {
		return value
	}
	output := strings.ToLower(steps[zw.StepCTFScopeCheck].Output)
	switch {
	case strings.Contains(output, "needs_scope") || strings.Contains(output, "нужен scope") || strings.Contains(output, "нужно подтвердить"):
		return "needs_scope"
	case strings.Contains(output, "ctf") || strings.Contains(output, "lab") || strings.Contains(output, "локаль"):
		return "ctf_or_lab_scope"
	case strings.TrimSpace(output) != "":
		return "reviewed"
	default:
		return ""
	}
}

func ctfFileSection(currentProject project.Project, relativePath string, title string, status string) CTFWorkspaceSection {
	content := readProjectRelativeFile(currentProject, relativePath)
	return CTFWorkspaceSection{
		Title:   title,
		Status:  status,
		Content: shortenForPrompt(content, 1400),
		Path:    relativePath,
	}
}

func ctfSectionFromFileOrStep(currentProject project.Project, relativePath string, title string, step zw.Step) CTFWorkspaceSection {
	content := readProjectRelativeFile(currentProject, relativePath)
	if strings.TrimSpace(content) == "" {
		content = step.Output
	}
	return CTFWorkspaceSection{
		Title:   title,
		Status:  ctfSectionStatus(step),
		Content: shortenForPrompt(content, 1800),
		Path:    relativePath,
		AgentID: step.AgentID,
	}
}

func ctfSectionFromStep(title string, step zw.Step) CTFWorkspaceSection {
	return CTFWorkspaceSection{
		Title:   title,
		Status:  ctfSectionStatus(step),
		Content: shortenForPrompt(step.Output, 1800),
		AgentID: step.AgentID,
	}
}

func ctfAttemptsSection(currentProject project.Project, root string, steps map[string]zw.Step) CTFWorkspaceSection {
	content := extractMarkdownSection(readProjectRelativeFile(currentProject, ctfPathJoin(root, "notes.md")), "Attempts")
	if strings.TrimSpace(content) == "" {
		content = steps[zw.StepCTFValidation].Output
	}
	return CTFWorkspaceSection{
		Title:   "Попытки",
		Status:  ctfSectionStatus(steps[zw.StepCTFValidation]),
		Content: shortenForPrompt(content, 1800),
		Path:    ctfPathJoin(root, "notes.md"),
		AgentID: steps[zw.StepCTFValidation].AgentID,
	}
}

func ctfEvidenceSection(currentProject project.Project, root string, steps map[string]zw.Step, files []CTFWorkspaceFile) CTFWorkspaceSection {
	evidenceIndex := ctfPathJoin(root, "evidence", "index.md")
	content := readProjectRelativeFile(currentProject, evidenceIndex)
	if strings.TrimSpace(content) == "" {
		content = extractMarkdownSection(readProjectRelativeFile(currentProject, ctfPathJoin(root, "notes.md")), "Evidence")
	}
	if strings.TrimSpace(content) == "" {
		content = steps[zw.StepCTFArtifactCollection].Output
	}
	if len(files) > 0 {
		var builder strings.Builder
		if strings.TrimSpace(content) != "" {
			builder.WriteString(strings.TrimSpace(content))
			builder.WriteString("\n\n")
		}
		builder.WriteString("Файлы workspace:\n")
		for _, file := range files {
			builder.WriteString("- ")
			builder.WriteString(file.RelativePath)
			builder.WriteString("\n")
		}
		content = builder.String()
	}
	return CTFWorkspaceSection{
		Title:   "Evidence",
		Status:  ctfSectionStatus(steps[zw.StepCTFArtifactCollection]),
		Content: shortenForPrompt(content, 1800),
		Path:    evidenceIndex,
		AgentID: steps[zw.StepCTFArtifactCollection].AgentID,
	}
}

func ctfSolverSection(currentProject project.Project, root string, step zw.Step) CTFWorkspaceSection {
	content := step.Output
	solveFiles := ctfDirectoryFiles(currentProject, ctfPathJoin(root, "solve"), "solver")
	if len(solveFiles) > 0 {
		var builder strings.Builder
		if strings.TrimSpace(content) != "" {
			builder.WriteString(strings.TrimSpace(content))
			builder.WriteString("\n\n")
		}
		builder.WriteString("Solver scripts:\n")
		for _, file := range solveFiles {
			builder.WriteString("- ")
			builder.WriteString(file.RelativePath)
			builder.WriteString("\n")
		}
		content = builder.String()
	}
	return CTFWorkspaceSection{
		Title:   "Solver scripts",
		Status:  ctfSectionStatus(step),
		Content: shortenForPrompt(content, 1800),
		Path:    ctfPathJoin(root, "solve"),
		AgentID: step.AgentID,
	}
}

func ctfSectionStatus(step zw.Step) string {
	if strings.TrimSpace(step.Status) != "" {
		return step.Status
	}
	if strings.TrimSpace(step.Output) != "" {
		return zw.StepStatusDone
	}
	return zw.StepStatusQueued
}

func ctfWorkspaceFiles(currentProject project.Project, root string, artifactItems []artifacts.Artifact) []CTFWorkspaceFile {
	seen := map[string]bool{}
	var files []CTFWorkspaceFile
	for _, item := range artifactItems {
		if !strings.HasPrefix(item.Kind, "ctf_") {
			continue
		}
		relative := filepath.ToSlash(strings.TrimSpace(item.RelativePath))
		if relative == "" || seen[relative] {
			continue
		}
		seen[relative] = true
		files = append(files, CTFWorkspaceFile{Kind: item.Kind, Title: item.Title, RelativePath: relative})
	}
	for _, dir := range []struct {
		path string
		kind string
	}{
		{ctfPathJoin(root, "artifacts"), "artifact"},
		{ctfPathJoin(root, "evidence"), "evidence"},
		{ctfPathJoin(root, "solve"), "solver"},
	} {
		for _, file := range ctfDirectoryFiles(currentProject, dir.path, dir.kind) {
			if seen[file.RelativePath] {
				continue
			}
			seen[file.RelativePath] = true
			files = append(files, file)
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files
}

func ctfDirectoryFiles(currentProject project.Project, relativeDir string, kind string) []CTFWorkspaceFile {
	if strings.TrimSpace(currentProject.Path) == "" || strings.TrimSpace(relativeDir) == "" {
		return nil
	}
	root := filepath.Clean(currentProject.Path)
	dir := filepath.Clean(filepath.Join(root, relativeDir))
	if dir != root && !strings.HasPrefix(dir, root+string(filepath.Separator)) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := make([]CTFWorkspaceFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		relative := filepath.ToSlash(filepath.Join(relativeDir, entry.Name()))
		files = append(files, CTFWorkspaceFile{
			Kind:         kind,
			Title:        entry.Name(),
			RelativePath: relative,
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].RelativePath < files[j].RelativePath
	})
	return files
}

func readProjectRelativeFile(currentProject project.Project, relativePath string) string {
	if strings.TrimSpace(currentProject.Path) == "" || strings.TrimSpace(relativePath) == "" {
		return ""
	}
	root := filepath.Clean(currentProject.Path)
	target := filepath.Clean(filepath.Join(root, relativePath))
	if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
		return ""
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return ""
	}
	return string(data)
}

func ctfPathJoin(root string, parts ...string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	all := append([]string{root}, parts...)
	return filepath.ToSlash(filepath.Join(all...))
}

func ctfYAMLValue(content string, key string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, key+":") {
			continue
		}
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, key+":")), `" '`)
	}
	return ""
}

func extractMarkdownSection(content string, heading string) string {
	if strings.TrimSpace(content) == "" || strings.TrimSpace(heading) == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	start := -1
	startLevel := 0
	needle := strings.ToLower(strings.TrimSpace(heading))
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		if strings.ToLower(title) == needle {
			start = index + 1
			startLevel = markdownHeadingLevel(trimmed)
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for index := start; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if strings.HasPrefix(trimmed, "#") && markdownHeadingLevel(trimmed) <= startLevel {
			end = index
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

func markdownHeadingLevel(line string) int {
	level := 0
	for _, r := range line {
		if r != '#' {
			break
		}
		level++
	}
	return level
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

	task, err := s.store.WorkflowTask(ctx, workflowRunID, projectID)
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
	_ = s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:       projectID,
		TaskID:          task.ID,
		WorkflowRunID:   workflowRunID,
		AcceptedAnswers: taskSpecAnswersFromClarifications(answers),
		OpenQuestions:   []string{},
		Status:          taskspec.StatusActive,
		Source:          "clarification_answers",
	}, true)
	return s.SendMessage(ctx, SendMessageInput{
		TaskID:    task.ID,
		ProjectID: projectID,
		Content:   clarificationAnswersMessage(answers),
	})
}

func (s *Service) ApplyWorkflowChanges(ctx context.Context, input ApplyWorkflowChangesInput) (ChatState, error) {
	ctx, release, err := s.acquireWorkflowWork(ctx, input.ProjectID, input.WorkflowRunID)
	if err != nil {
		return ChatState{}, err
	}
	defer release()
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
		if err := s.suggestTestCommands(ctx, currentProject, taskID, workflowRunID, model); err != nil {
			s.setAgentStatus(agents.TesterID, "failed", "Не удалось подготовить проверки: "+err.Error(), model.ID)
			s.emitChatState(ctx, projectID, "")
		}
	}
	return state, nil
}

func (s *Service) RollbackWorkflowChanges(ctx context.Context, input RollbackWorkflowChangesInput) (ChatState, error) {
	ctx, release, err := s.acquireWorkflowWork(ctx, input.ProjectID, input.WorkflowRunID)
	if err != nil {
		return ChatState{}, err
	}
	defer release()
	projectID := strings.TrimSpace(input.ProjectID)
	workflowRunID := strings.TrimSpace(input.WorkflowRunID)
	if projectID == "" || workflowRunID == "" {
		return ChatState{}, fmt.Errorf("project_id и workflow_run_id обязательны")
	}
	currentProject, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return ChatState{}, err
	}
	items, err := s.store.ListProposedChanges(ctx, projectID, workflowRunID, 200)
	if err != nil {
		return ChatState{}, err
	}
	applied := make([]changes.ProposedChange, 0, len(items))
	for _, item := range items {
		if item.Status == changes.StatusApplied {
			applied = append(applied, item)
		}
	}
	if len(applied) == 0 {
		return s.emitChatState(ctx, projectID, ""), nil
	}

	model, modelErr := s.store.ActiveModelConfig(ctx)
	if modelErr == nil {
		s.setAgentStatus(agents.DeveloperID, "writing_files", "Откатывает изменения workflow", model.ID)
	}

	rolledBack := 0
	failed := 0
	taskID := applied[0].TaskID
	for _, change := range applied {
		result, err := changes.Rollback(currentProject.Path, change)
		if err != nil {
			failed++
			_ = s.store.MarkProposedChangeFailed(ctx, change.ID, "rollback: "+err.Error())
			continue
		}
		rolledBack++
		_ = s.store.MarkProposedChangeRolledBack(ctx, change.ID, result.DiffText)
	}
	if taskID != "" {
		_, _ = s.store.AddMessage(ctx, taskID, "agent", agents.DeveloperID, rollbackChangesMessage(rolledBack, failed))
		_ = s.store.TouchTask(ctx, taskID)
		_ = s.patchProjectMemory(ctx, projectmemory.Memory{
			ProjectID:         currentProject.ID,
			UpdatedFromTaskID: taskID,
			Environment:       []string{fmt.Sprintf("Rollback workflow %s: откатано %d, ошибок %d.", workflowRunID, rolledBack, failed)},
		})
	}
	if modelErr == nil {
		if failed > 0 {
			s.setAgentStatus(agents.DeveloperID, "failed", "Часть изменений не откатилась", model.ID)
		} else {
			s.setAgentStatus(agents.DeveloperID, "done", "Изменения откатаны", model.ID)
		}
	}
	return s.emitChatState(ctx, projectID, ""), nil
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

	maxRepairIterations := s.maxRepairIterationsForProject(ctx, project.ID)
	previousDeveloperOutput := ""
	for iteration := 0; iteration <= maxRepairIterations; iteration++ {
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
			if iteration >= maxRepairIterations {
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
		if failedTests+blockedTests > 0 {
			testRuns, _ := s.store.ListTestRuns(ctx, project.ID, run.ID, 30)
			parsed, ok := repairloop.ReviewFromTests(testRuns)
			if ok {
				result.ReviewStatus = parsed.Status
				result.ReviewReturnTo = parsed.ReturnTo
				result.ReviewSummary = strings.TrimSpace(parsed.Summary)
				result.ReviewFindings = parsed.Findings
				result.ReviewRequired = parsed.RequiredChanges
				if parsed.Status == reviews.StatusBlocked || parsed.ReturnTo == reviews.ReturnToUser {
					result.Blocked = true
					result.BlockReason = reviewBlockedReason(parsed)
					_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepTesterCommands, result.BlockReason)
					return result
				}
				if iteration >= maxRepairIterations {
					result.Blocked = true
					result.BlockReason = "исчерпан лимит repair-итераций: последние проверки все еще требуют доработку"
					_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepTesterCommands, result.BlockReason)
					return result
				}
				developer, err := s.rerunFromReview(ctx, result, project, task, run, provider, model, workflow, parsed)
				if err != nil {
					result.Blocked = true
					result.BlockReason = err.Error()
					_ = s.store.UpdateWorkflowRun(ctx, run.ID, zw.StatusBlocked, zw.StepTesterCommands, result.BlockReason)
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
				continue
			}
		}

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
		if iteration >= maxRepairIterations {
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
	parsedBlueprint = devworkspace.NormalizeBlueprint(parsedBlueprint, project.Path)
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
		result, err := s.applyDevPipelineChange(ctx, project.Path, change)
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

func (s *Service) applyDevPipelineChange(ctx context.Context, projectPath string, change changes.ProposedChange) (changes.ApplyResult, error) {
	result, err := changes.Apply(projectPath, change)
	if err != nil {
		return changes.ApplyResult{}, err
	}
	relativePath := filepath.ToSlash(strings.Trim(change.FilePath, "/"))
	if filepath.Ext(relativePath) != ".go" {
		return result, nil
	}
	targetPath := filepath.Join(projectPath, relativePath)
	formatCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(formatCtx, "gofmt", "-w", targetPath)
	cmd.Dir = projectPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return result, fmt.Errorf("gofmt не прошел для %s: %v: %s", relativePath, err, strings.TrimSpace(string(output)))
	}
	formatted, err := os.ReadFile(targetPath)
	if err != nil {
		return result, err
	}
	result.AfterContent = string(formatted)
	result.DiffText = changes.GenerateUnifiedDiff(relativePath, result.BeforeContent, result.AfterContent)
	return result, nil
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
	ctx, release, err := s.acquireWorkflowWork(ctx, projectID, testRun.WorkflowRunID)
	if err != nil {
		return ChatState{}, err
	}
	defer release()

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
		s.setAgentStatus(agents.TesterID, "failed", "Команда заблокирована policy", modelID)
		_ = s.store.FinishWorkflowPlanStep(ctx, testRun.WorkflowRunID, zw.StepTesterCommands, agents.TesterID, zw.StepStatusFailed, "команда заблокирована execution policy")
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
	ctx, release, err := s.acquireWorkflowWork(ctx, input.ProjectID, input.WorkflowRunID)
	if err != nil {
		return ChatState{}, err
	}
	defer release()
	projectID := strings.TrimSpace(input.ProjectID)
	workflowRunID := strings.TrimSpace(input.WorkflowRunID)
	if projectID == "" || workflowRunID == "" {
		return ChatState{}, fmt.Errorf("project_id и workflow_run_id обязательны")
	}

	currentProject, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return ChatState{}, err
	}
	task, err := s.store.WorkflowTask(ctx, workflowRunID, projectID)
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
	parsed = repairloop.NormalizeReviewForAutopilot(parsed)
	parsed = reviewgate.Enforce(parsed, s.reviewGateReport(ctx, currentProject, run.ID))
	parsed = repairloop.NormalizeReviewForAutopilot(parsed)

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

func (s *Service) ListAgentGroups(ctx context.Context) ([]AgentGroupDTO, error) {
	return s.store.ListAgentGroups(ctx, false)
}

func (s *Service) CreateAgentGroup(ctx context.Context, input CreateAgentGroupInput) ([]AgentGroupDTO, error) {
	defaultModelID := strings.TrimSpace(input.DefaultModelID)
	if defaultModelID == "" {
		model, err := s.store.ActiveModelConfig(ctx)
		if err != nil {
			return nil, err
		}
		defaultModelID = model.ID
	}
	if _, err := s.store.CreateAgentGroup(ctx, agentgroups.Group{
		Name:           input.Name,
		Kind:           input.Kind,
		Description:    input.Description,
		DefaultModelID: defaultModelID,
	}); err != nil {
		return nil, err
	}
	return s.store.ListAgentGroups(ctx, false)
}

func (s *Service) UpdateAgentGroup(ctx context.Context, input UpdateAgentGroupInput) ([]AgentGroupDTO, error) {
	if _, err := s.store.UpdateAgentGroup(ctx, agentgroups.Group{
		ID:             input.ID,
		Name:           input.Name,
		Kind:           input.Kind,
		Description:    input.Description,
		DefaultModelID: input.DefaultModelID,
	}); err != nil {
		return nil, err
	}
	return s.store.ListAgentGroups(ctx, false)
}

func (s *Service) ArchiveAgentGroup(ctx context.Context, input ArchiveAgentGroupInput) ([]AgentGroupDTO, error) {
	if err := s.store.ArchiveAgentGroup(ctx, input.GroupID); err != nil {
		return nil, err
	}
	return s.store.ListAgentGroups(ctx, false)
}

func (s *Service) ListAgentProfiles(ctx context.Context, groupID string) ([]AgentProfileDTO, error) {
	return s.store.ListAgentProfiles(ctx, groupID)
}

func (s *Service) SaveAgentProfile(ctx context.Context, input SaveAgentProfileInput) ([]AgentProfileDTO, error) {
	profile := agentgroups.Profile{
		ID:            input.ID,
		GroupID:       input.GroupID,
		Name:          input.Name,
		RoleKey:       input.RoleKey,
		Description:   input.Description,
		AvatarPath:    input.AvatarPath,
		SoulPath:      input.SoulPath,
		ModelID:       input.ModelID,
		ToolProfileID: input.ToolProfileID,
		DefaultSkills: input.DefaultSkills,
		Capabilities:  input.Capabilities,
		AllowedTools:  input.AllowedTools,
		ReadPaths:     input.ReadPaths,
		WritePaths:    input.WritePaths,
		HandoffRules:  input.HandoffRules,
		Temperature:   input.Temperature,
		ContextBudget: input.ContextBudget,
		Enabled:       input.Enabled,
		SortOrder:     input.SortOrder,
	}
	saved, err := s.store.SaveAgentProfile(ctx, profile)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureSoulFile(ctx, saved); err != nil {
		return nil, err
	}
	return s.store.ListAgentProfiles(ctx, saved.GroupID)
}

func (s *Service) SetAgentProfileEnabled(ctx context.Context, input SetAgentProfileEnabledInput) ([]AgentProfileDTO, error) {
	profileID := strings.TrimSpace(input.ProfileID)
	if profileID == "" {
		return nil, fmt.Errorf("profile_id пустой")
	}
	if err := s.store.SetAgentProfileEnabled(ctx, profileID, input.Enabled); err != nil {
		return nil, err
	}
	groups, err := s.store.ListAgentGroups(ctx, true)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		profiles, err := s.store.ListAgentProfiles(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		for _, profile := range profiles {
			if profile.ID == profileID {
				return profiles, nil
			}
		}
	}
	return nil, fmt.Errorf("агент %s не найден", profileID)
}

func (s *Service) GetAgentSoul(ctx context.Context, profileID string) (AgentSoulDTO, error) {
	profile, err := s.store.GetAgentProfile(ctx, profileID)
	if err != nil {
		return AgentSoulDTO{}, err
	}
	path, err := s.ensureSoulFile(ctx, profile)
	if err != nil {
		return AgentSoulDTO{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return AgentSoulDTO{}, err
	}
	return AgentSoulDTO{
		ProfileID: profile.ID,
		Path:      path,
		Content:   string(content),
		Warnings:  soulWarnings(string(content)),
	}, nil
}

func (s *Service) SaveAgentSoul(ctx context.Context, input SaveAgentSoulInput) (AgentSoulDTO, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return AgentSoulDTO{}, fmt.Errorf("soul.md пустой")
	}
	profile, err := s.store.GetAgentProfile(ctx, input.ProfileID)
	if err != nil {
		return AgentSoulDTO{}, err
	}
	path, err := s.ensureSoulFile(ctx, profile)
	if err != nil {
		return AgentSoulDTO{}, err
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return AgentSoulDTO{}, err
	}
	return AgentSoulDTO{
		ProfileID: profile.ID,
		Path:      path,
		Content:   content,
		Warnings:  soulWarnings(content),
	}, nil
}

func (s *Service) ListLifecycleDefinitions(ctx context.Context, groupID string) ([]LifecycleDefinitionDTO, error) {
	return s.store.ListLifecycleDefinitions(ctx, groupID)
}

func (s *Service) ListLifecycleSteps(ctx context.Context, lifecycleID string) ([]LifecycleStepDTO, error) {
	return s.store.ListLifecycleSteps(ctx, lifecycleID)
}

func (s *Service) ValidateLifecycleRuntime(ctx context.Context, lifecycleID string) ([]LifecycleRuntimeIssueDTO, error) {
	definition, err := s.store.GetLifecycleDefinition(ctx, lifecycleID)
	if err != nil {
		return nil, err
	}
	steps, err := s.store.ListLifecycleSteps(ctx, lifecycleID)
	if err != nil {
		return nil, err
	}
	executor := lifecycler.NewExecutor(definition, steps)
	return executor.ValidateRuntime(), nil
}

func (s *Service) SaveLifecycleDefinition(ctx context.Context, input SaveLifecycleDefinitionInput) ([]LifecycleDefinitionDTO, error) {
	saved, err := s.store.SaveLifecycleDefinition(ctx, agentgroups.LifecycleDefinition{
		ID:                  input.ID,
		GroupID:             input.GroupID,
		Name:                input.Name,
		Kind:                input.Kind,
		Description:         input.Description,
		MaxTotalIterations:  input.MaxTotalIterations,
		MaxRepairIterations: input.MaxRepairIterations,
		SameErrorLimit:      input.SameErrorLimit,
		Status:              input.Status,
	})
	if err != nil {
		return nil, err
	}
	return s.store.ListLifecycleDefinitions(ctx, saved.GroupID)
}

func (s *Service) SaveLifecycleStep(ctx context.Context, input SaveLifecycleStepInput) ([]LifecycleStepDTO, error) {
	if _, err := lifecycler.ParseStepRuntimeConfig(input.OutputSchema); err != nil {
		return nil, err
	}
	saved, err := s.store.SaveLifecycleStep(ctx, agentgroups.LifecycleStep{
		ID:               input.ID,
		LifecycleID:      input.LifecycleID,
		StepKey:          input.StepKey,
		Title:            input.Title,
		AgentProfileID:   input.AgentProfileID,
		Mode:             input.Mode,
		Required:         input.Required,
		CanRetry:         input.CanRetry,
		MaxRetries:       input.MaxRetries,
		OnSuccessStepKey: input.OnSuccessStepKey,
		OnFailureStepKey: input.OnFailureStepKey,
		OutputSchema:     input.OutputSchema,
		VisibleToUser:    input.VisibleToUser,
		SortOrder:        input.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return s.store.ListLifecycleSteps(ctx, saved.LifecycleID)
}

func (s *Service) DeleteLifecycleStep(ctx context.Context, input DeleteLifecycleStepInput) ([]LifecycleStepDTO, error) {
	if strings.TrimSpace(input.StepID) == "" {
		return nil, errors.New("step id is required")
	}
	if err := s.store.DeleteLifecycleStep(ctx, input.StepID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.LifecycleID) == "" {
		return []LifecycleStepDTO{}, nil
	}
	return s.store.ListLifecycleSteps(ctx, input.LifecycleID)
}

func (s *Service) BindProjectAgentGroup(ctx context.Context, input BindProjectAgentGroupInput) (ProjectState, error) {
	if _, err := s.store.BindProjectToAgentGroup(ctx, input.ProjectID, input.GroupID, input.LifecycleID); err != nil {
		return ProjectState{}, err
	}
	return s.projectState(ctx, input.ProjectID)
}

func (s *Service) ensureDefaultAgentSouls(ctx context.Context) error {
	groups, err := s.store.ListAgentGroups(ctx, false)
	if err != nil {
		return err
	}
	for _, group := range groups {
		profiles, err := s.store.ListAgentProfiles(ctx, group.ID)
		if err != nil {
			return err
		}
		for _, profile := range profiles {
			if _, err := s.ensureSoulFile(ctx, profile); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) ensureSoulFile(ctx context.Context, profile agentgroups.Profile) (string, error) {
	group, err := s.store.GetAgentGroup(ctx, profile.GroupID)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(profile.SoulPath)
	if path == "" {
		path = filepath.Join(s.paths.AgentsDir, safePathSegment(group.Slug), safePathSegment(profile.RoleKey), "soul.md")
		profile.SoulPath = path
		if _, err := s.store.SaveAgentProfile(ctx, profile); err != nil {
			return "", err
		}
	}
	if !isPathInside(path, s.paths.AgentsDir) {
		return "", fmt.Errorf("soul_path должен лежать внутри %s", s.paths.AgentsDir)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		content := defaultSoulContent(group, profile)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) agentSoulForStep(ctx context.Context, projectID string, spec agents.Spec) string {
	content, _, _ := s.agentRuntimeContextForStep(ctx, projectID, spec)
	return content
}

func (s *Service) agentRuntimeContextForStep(ctx context.Context, projectID string, spec agents.Spec) (string, string, string) {
	binding, err := s.store.ProjectGroupBinding(ctx, projectID)
	if err != nil {
		return defaultSoulForSpec(spec), "", ""
	}
	profiles, err := s.store.ListAgentProfiles(ctx, binding.GroupID)
	if err != nil {
		return defaultSoulForSpec(spec), "", ""
	}
	for _, profile := range profiles {
		if !profile.Enabled {
			continue
		}
		if profile.RoleKey != spec.Role && profile.RoleKey != spec.ID {
			continue
		}
		path, err := s.ensureSoulFile(ctx, profile)
		if err != nil {
			return defaultSoulForSpec(spec), "", profile.ToolProfileID
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return defaultSoulForSpec(spec), path, profile.ToolProfileID
		}
		return strings.TrimSpace(string(content) + "\n\n" + agentSkillsContract(profile) + "\n\n" + agentCapabilityContract(profile)), path, profile.ToolProfileID
	}
	return defaultSoulForSpec(spec), "", ""
}

func agentSkillsContract(profile agentgroups.Profile) string {
	skills := agentgroups.NormalizeDefaultSkills(profile.DefaultSkills)
	if len(skills) == 0 {
		skills = agentgroups.DefaultSkillsForRole(profile.RoleKey)
	}
	var builder strings.Builder
	builder.WriteString("# Default Skills\n\n")
	builder.WriteString("Эти skills подключены к агенту по умолчанию. Используй их как постоянный рабочий режим, если пользователь явно не выбрал другой skill.\n\n")
	for _, skill := range skills {
		builder.WriteString("- $")
		builder.WriteString(skill)
		builder.WriteString(": ")
		builder.WriteString(skillPurpose(skill))
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func skillPurpose(skill string) string {
	switch strings.ToLower(strings.TrimPrefix(skill, "$")) {
	case "pony-tail":
		return "ясный spec-driven стиль, аккуратные изменения, короткий полезный вывод"
	case "security":
		return "безопасный анализ угроз, scope, defensive рекомендации и guardrails"
	case "research":
		return "поиск, проверка свежести источников, ссылки и аналитическая выжимка"
	case "ctf":
		return "CTF reasoning, гипотезы, evidence, solver scripts и writeup"
	case "dev":
		return "разработка, тесты, review, workspace hygiene и controlled changes"
	default:
		return "кастомный skill агента"
	}
}

func agentCapabilityContract(profile agentgroups.Profile) string {
	profile = agentgroups.NormalizeCapabilities(profile)
	var builder strings.Builder
	builder.WriteString("# Capabilities Contract\n\n")
	builder.WriteString("Этот контракт имеет приоритет над общим стилем из soul.md и описывает границы текущего агента.\n\n")
	builder.WriteString("## Роль\n\n")
	builder.WriteString("- ")
	builder.WriteString(firstNonEmpty(profile.RoleKey, "agent"))
	builder.WriteString("\n\n")
	writeContractList(&builder, "## Что умею", profile.Capabilities)
	writeContractList(&builder, "## Разрешенные инструменты", profile.AllowedTools)
	builder.WriteString("## Доступ к файлам\n\n")
	writeInlineContractList(&builder, "Читать", profile.ReadPaths)
	writeInlineContractList(&builder, "Писать", profile.WritePaths)
	builder.WriteString("\n")
	writeContractList(&builder, "## Когда передаю дальше", profile.HandoffRules)
	return strings.TrimSpace(builder.String())
}

func writeContractList(builder *strings.Builder, title string, values []string) {
	builder.WriteString(title)
	builder.WriteString("\n\n")
	if len(values) == 0 {
		builder.WriteString("- Не задано явно.\n\n")
		return
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
	builder.WriteString("\n")
}

func writeInlineContractList(builder *strings.Builder, title string, values []string) {
	builder.WriteString(title)
	builder.WriteString(":\n")
	if len(values) == 0 {
		builder.WriteString("- Не задано явно.\n")
		return
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		builder.WriteString("- ")
		builder.WriteString(value)
		builder.WriteString("\n")
	}
}

func defaultSoulContent(group agentgroups.Group, profile agentgroups.Profile) string {
	return strings.TrimSpace(fmt.Sprintf(`# Soul

## Кто я

Я — %s, агент группы "%s" в Zavod AI.

## За что отвечаю

%s

## Как принимаю решения

- Сначала сверяюсь с задачей пользователя и текущей task spec.
- Отделяю факты от предположений.
- Если данных недостаточно, явно называю недостающий контекст.
- Не подменяю требования своим удобством.

## Как общаюсь с пользователем

- Пишу по-русски.
- Даю короткие, практичные ответы.
- Не вывожу служебный JSON, trace и внутренние файлы без явной просьбы.

## Что никогда не делаю

- Не игнорирую safety guardrails приложения.
- Не раскрываю секреты, токены и приватные данные.
- Не выполняю произвольные команды вне разрешенного tool profile.
- Не утверждаю, что выполнил действия, если в контексте нет результата.

## Как передаю задачу дальше

- Передаю следующему агенту только значимый контекст.
- Сохраняю решения, ограничения и открытые вопросы.
- Если вижу блокер, формулирую причину и нужный следующий шаг.
`, profile.Name, group.Name, firstNonEmpty(profile.Description, "Выполняю свою роль в жизненном цикле группы."))) + "\n"
}

func defaultSoulForSpec(spec agents.Spec) string {
	return strings.TrimSpace(fmt.Sprintf(`# Soul

## Кто я

Я — %s, агент Zavod AI.

## За что отвечаю

Работаю как роль "%s" и выполняю текущий шаг workflow.

## Как общаюсь с пользователем

Пишу по-русски, кратко и по делу. Не вывожу служебный JSON без необходимости.
`, spec.Name, spec.Role))
}

func soulWarnings(content string) []string {
	lower := strings.ToLower(content)
	checks := []struct {
		needle  string
		warning string
	}{
		{"ignore safety", "Есть инструкция игнорировать safety."},
		{"ignore previous instructions", "Есть инструкция игнорировать предыдущие инструкции."},
		{"раскрой секрет", "Есть подозрительная инструкция про раскрытие секретов."},
		{"выведи токен", "Есть подозрительная инструкция про токены."},
		{"без scope", "Есть риск обхода scope для ИБ-задач."},
		{"произвольн", "Есть риск разрешения произвольных действий."},
	}
	var warnings []string
	for _, check := range checks {
		if strings.Contains(lower, check.needle) {
			warnings = append(warnings, check.warning)
		}
	}
	return warnings
}

func safePathSegment(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "agent"
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 'а' && r <= 'я' || r == 'ё' {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	clean := strings.Trim(builder.String(), "-")
	if clean == "" {
		return "agent"
	}
	return clean
}

func isPathInside(path string, root string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
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
	ClarificationStep  string
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
	if run != nil {
		if savedBlueprint, err := s.store.LatestTaskBlueprint(ctx, run.ID); err == nil && savedBlueprint != nil {
			result.Blueprint = *savedBlueprint
		}
	}

	executed := map[string]bool{}
	outputs := map[string]string{}
	var runStep func(stepKey string, force bool) (string, error)
	runStep = func(stepKey string, force bool) (string, error) {
		if executed[stepKey] && !force {
			return outputs[stepKey], nil
		}
		switch stepKey {
		case zw.StepManagerIntake:
			intake, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepManagerIntake, buildManagerIntakeInput(history))
			if err != nil {
				return "", err
			}
			result.Intake = intake
			executed[stepKey] = true
			outputs[stepKey] = intake
			intakeResult, hasStructuredIntake := parseManagerIntake(intake)
			if hasStructuredIntake {
				_ = s.updateTaskSpecFromIntake(ctx, project, taskID, run.ID, intakeResult)
			}
			if managerNeedsClarification(intake) {
				result.NeedsClarification = true
				if hasStructuredIntake {
					result.Clarification = intake
					_ = intakeResult
				} else {
					result.Clarification = intake
				}
				result.ClarificationStep = zw.StepManagerIntake
			}
			return intake, nil
		case zw.StepProductRequirements:
			if _, err := runStep(zw.StepManagerIntake, false); err != nil || result.NeedsClarification {
				return "", err
			}
			product, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepProductRequirements, buildProductInput(result.Intake))
			if err != nil {
				return "", err
			}
			result.Product = product
			_ = s.updateTaskSpecFromProduct(ctx, project.ID, taskID, run.ID, product)
			executed[stepKey] = true
			outputs[stepKey] = product
			return product, nil
		case zw.StepTaskBlueprint:
			if _, err := runStep(zw.StepProductRequirements, false); err != nil || result.NeedsClarification {
				return "", err
			}
			blueprintOutput, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepTaskBlueprint, buildBlueprintInput(result.Intake, result.Product, projectCheckSignals(project.Path)))
			if err != nil {
				return "", err
			}
			parsedBlueprint, err := blueprint.Parse(blueprintOutput)
			if err != nil {
				return "", err
			}
			parsedBlueprint = blueprint.NormalizeForProject(parsedBlueprint, project.Path)
			parsedBlueprint = devworkspace.NormalizeBlueprint(parsedBlueprint, project.Path)
			parsedBlueprint.ProjectID = projectID
			parsedBlueprint.TaskID = taskID
			parsedBlueprint.WorkflowRunID = run.ID
			savedBlueprint, err := s.store.CreateTaskBlueprint(ctx, parsedBlueprint)
			if err != nil {
				return "", err
			}
			result.Blueprint = savedBlueprint
			_ = s.updateTaskSpecFromBlueprint(ctx, project.ID, taskID, run.ID, savedBlueprint)
			_ = s.updateProjectMemoryFromBlueprint(ctx, project.ID, taskID, savedBlueprint)
			executed[stepKey] = true
			outputs[stepKey] = blueprintOutput
			return blueprintOutput, nil
		case zw.StepArchitectPlan:
			if _, err := runStep(zw.StepTaskBlueprint, false); err != nil || result.NeedsClarification {
				return "", err
			}
			architect, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepArchitectPlan, buildArchitectInput(result.Intake, result.Product, &result.Blueprint))
			if err != nil {
				return "", err
			}
			result.Architect = architect
			_ = s.patchProjectMemory(ctx, projectmemory.Memory{
				ProjectID:         project.ID,
				Architecture:      shortenForPrompt(architect, 1800),
				UpdatedFromTaskID: taskID,
			})
			executed[stepKey] = true
			outputs[stepKey] = architect
			return architect, nil
		case zw.StepDeveloperPlan:
			if _, err := runStep(zw.StepArchitectPlan, false); err != nil || result.NeedsClarification {
				return "", err
			}
			developer, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, zw.StepDeveloperPlan, buildDeveloperInput(result.Intake, result.Product, &result.Blueprint, result.Architect, projectSourceSnapshot(project.Path)))
			if err != nil {
				return "", err
			}
			result.Developer = developer
			executed[stepKey] = true
			outputs[stepKey] = developer
			return developer, nil
		default:
			if isRuntimeOwnedStep(stepKey) {
				return "", nil
			}
			if _, err := runStep(zw.StepManagerIntake, false); err != nil || result.NeedsClarification {
				return "", err
			}
			output, err := s.runWorkflowStep(ctx, projectID, taskID, run, provider, model, stepKey, buildGenericLifecycleInput(result, project, stepKey))
			if err != nil {
				return "", err
			}
			executed[stepKey] = true
			outputs[stepKey] = output
			return output, nil
		}
		return "", nil
	}

	if executor, ok := s.lifecycleExecutor(ctx, projectID); ok {
		if err := s.runV03RuntimeLifecycle(ctx, run, &result, runStep, executor); err != nil {
			return result, err
		}
		return result, nil
	}

	for _, stepKey := range s.devLifecycleStepKeys(ctx, projectID) {
		if _, err := runStep(stepKey, false); err != nil {
			return result, err
		}
		if result.NeedsClarification {
			return result, nil
		}
	}

	return result, nil
}

func (s *Service) runV03RuntimeLifecycle(
	ctx context.Context,
	run *zw.Run,
	result *v03WorkflowResult,
	runStep func(stepKey string, force bool) (string, error),
	executor lifecycler.Executor,
) error {
	handler := func(ctx context.Context, step lifecycleruntime.StepContext) lifecycleruntime.StepResult {
		force := step.Retry || step.Force
		output, err := runStep(step.Step.StepKey, force)
		if result.NeedsClarification {
			return lifecycleruntime.StepResult{
				Status:    zw.StatusWaitingUser,
				Output:    output,
				Error:     firstNonEmpty(result.Clarification, "step waits for user input"),
				WaitHuman: true,
			}
		}
		return lifecycleRuntimeStepResult(output, err)
	}
	runner := lifecycleruntime.NewRunner(executor, map[string]lifecycleruntime.StepHandler{
		lifecycler.ModeLLM:      handler,
		lifecycler.ModeTool:     handler,
		lifecycler.ModeChecks:   handler,
		lifecycler.ModeReview:   handler,
		lifecycler.ModeArtifact: handler,
		lifecycler.ModeFinal:    handler,
		lifecycler.ModeJoin: func(ctx context.Context, step lifecycleruntime.StepContext) lifecycleruntime.StepResult {
			return lifecycleruntime.StepResult{Status: zw.StepStatusDone, Output: "join complete"}
		},
	}, handler)
	runtimeResult, err := runner.Run(ctx, s.lifecycleRuntimeState(ctx, run, result))
	if runtimeResult.Status == zw.StatusWaitingUser {
		result.NeedsClarification = true
		result.ClarificationStep = runtimeResult.CurrentStepKey
		result.Clarification = runtimeResult.Reason
		return nil
	}
	if runtimeResult.Status == zw.StatusBlocked {
		return errors.New(firstNonEmpty(runtimeResult.Reason, "lifecycle runtime blocked"))
	}
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) lifecycleRuntimeState(ctx context.Context, run *zw.Run, result *v03WorkflowResult) lifecycler.RuntimeState {
	state := lifecycler.RuntimeState{
		Results:    map[string]lifecycler.StepResult{},
		Attempts:   map[string]int{},
		Variables:  map[string]string{},
		HumanGates: map[string]bool{},
	}
	if run == nil || strings.TrimSpace(run.ID) == "" {
		return state
	}
	state.CurrentStepKey = run.CurrentStep
	steps, err := s.store.ListWorkflowSteps(ctx, run.ID)
	if err != nil {
		return state
	}
	for _, step := range steps {
		if strings.TrimSpace(step.StepKey) == "" {
			continue
		}
		state.Results[step.StepKey] = lifecycler.StepResult{
			StepKey: step.StepKey,
			Status:  step.Status,
			Output:  step.Output,
			Error:   step.Error,
		}
		if step.Status == zw.StepStatusFailed {
			state.Attempts[step.StepKey]++
		}
		hydrateV03ResultFromWorkflowStep(result, step)
	}
	if run.Status == zw.StatusWaitingUser && strings.TrimSpace(run.CurrentStep) != "" {
		state.HumanGates[run.CurrentStep] = true
	}
	return state
}

func hydrateV03ResultFromWorkflowStep(result *v03WorkflowResult, step zw.Step) {
	if result == nil {
		return
	}
	switch step.StepKey {
	case zw.StepManagerIntake:
		result.Intake = firstNonEmpty(result.Intake, step.Output)
	case zw.StepProductRequirements:
		result.Product = firstNonEmpty(result.Product, step.Output)
	case zw.StepArchitectPlan:
		result.Architect = firstNonEmpty(result.Architect, step.Output)
	case zw.StepDeveloperPlan:
		result.Developer = firstNonEmpty(result.Developer, step.Output)
	case zw.StepManagerFinal:
		result.Final = firstNonEmpty(result.Final, step.Output)
	}
}

func lifecycleRuntimeStepResult(output string, err error) lifecycleruntime.StepResult {
	status := zw.StepStatusDone
	errText := ""
	if err != nil {
		status = zw.StepStatusFailed
		errText = err.Error()
	}
	return lifecycleruntime.StepResult{
		Status: status,
		Output: output,
		Error:  errText,
	}
}

func lifecycleHumanGateNotice(decision lifecycler.RuntimeDecision) string {
	var builder strings.Builder
	builder.WriteString("## Нужно подтверждение\n\n")
	builder.WriteString(firstNonEmpty(decision.Reason, "Нужно подтвердить следующий шаг workflow."))
	if len(decision.RequiredInputs) > 0 {
		builder.WriteString("\n\nЧто нужно подтвердить:\n")
		for _, input := range decision.RequiredInputs {
			if strings.TrimSpace(input) == "" {
				continue
			}
			builder.WriteString("- ")
			builder.WriteString(strings.TrimSpace(input))
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func (s *Service) devLifecycleStepKeys(ctx context.Context, projectID string) []string {
	keys := []string{}
	if executor, ok := s.lifecycleExecutor(ctx, projectID); ok {
		keys = append(keys, executor.StepKeys()...)
	}
	required := []string{
		zw.StepManagerIntake,
		zw.StepProductRequirements,
		zw.StepTaskBlueprint,
		zw.StepArchitectPlan,
		zw.StepDeveloperPlan,
	}
	for _, key := range required {
		if !containsString(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func isRuntimeOwnedStep(stepKey string) bool {
	switch stepKey {
	case zw.StepUserPlan, zw.StepTesterCommands, zw.StepReview, zw.StepManagerFinal, zw.StepWebResearch, zw.StepSecurityAnalysis:
		return true
	default:
		return false
	}
}

func buildGenericLifecycleInput(result v03WorkflowResult, project project.Project, stepKey string) string {
	return strings.TrimSpace(fmt.Sprintf(`
Проект: %s
Шаг lifecycle: %s

Task brief:
%s

Требования:
%s

Task Blueprint:
%s

Архитектурный план:
%s

Выполни этот lifecycle-шаг кратко и практично. Не меняй файлы напрямую и не утверждай, что изменения уже применены.
`, project.Name, stepKey, result.Intake, result.Product, blueprint.ToPrompt(&result.Blueprint), result.Architect))
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
	return s.runWorkflowStepWithSpec(ctx, projectID, taskID, run, provider, model, stepKey, agents.SpecForStep(stepKey), input)
}

func (s *Service) runWorkflowStepWithSpec(
	ctx context.Context,
	projectID string,
	taskID string,
	run *zw.Run,
	provider llm.Provider,
	model llm.ModelConfig,
	stepKey string,
	spec agents.Spec,
	input string,
) (string, error) {
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

	soul, soulPath, toolID := s.agentRuntimeContextForStep(ctx, projectID, spec)
	runtime := agentRuntimeStatusUpdate{
		ToolID:   toolID,
		SoulPath: soulPath,
		StepKey:  stepKey,
	}
	s.setAgentRuntimeStatus(spec.ID, "thinking", stepThinkingActivity(stepKey), model.ID, runtime)
	s.setAgentRuntimeStatus(spec.ID, "calling_model", "Отправляет шаг в "+model.Name, model.ID, runtime)

	stepCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := provider.Generate(stepCtx, agents.RequestForSpecWithSoul(model.ModelName, spec, soul, input))
	if resp != nil {
		runtime.InputTokens = resp.InputTokens
		runtime.OutputTokens = resp.OutputTokens
		runtime.TotalTokens = resp.TotalTokens
	}
	if err != nil {
		failed, finishErr := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusFailed, "", err.Error())
		if finishErr == nil {
			s.emitWorkflowStep(*run, failed)
		}
		_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID, zw.StepStatusFailed, err.Error())
		s.setAgentRuntimeStatus(spec.ID, "failed", "Ошибка вызова модели", model.ID, runtime)
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
		s.setAgentRuntimeStatus(spec.ID, "failed", "Пустой ответ модели", model.ID, runtime)
		return "", err
	}
	if len(output) > managerMaxAnswerBytes || looksLikeRepetitionLoop(output) {
		err := errors.New("ответ остановлен: модель начала повторять или генерировать слишком длинный текст")
		failed, finishErr := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusFailed, output, err.Error())
		if finishErr == nil {
			s.emitWorkflowStep(*run, failed)
		}
		_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID, zw.StepStatusFailed, err.Error())
		s.setAgentRuntimeStatus(spec.ID, "failed", "Ответ модели остановлен", model.ID, runtime)
		return "", err
	}

	s.setAgentRuntimeStatus(spec.ID, "answering", "Сохраняет результат шага", model.ID, runtime)
	done, err := s.store.FinishWorkflowStep(ctx, step.ID, zw.StepStatusDone, output, "")
	if err != nil {
		s.setAgentRuntimeStatus(spec.ID, "failed", "Не удалось сохранить шаг", model.ID, runtime)
		return "", err
	}
	s.emitWorkflowStep(*run, done)
	_ = s.store.FinishWorkflowPlanStep(ctx, run.ID, stepKey, spec.ID, zw.StepStatusDone, "")
	s.setAgentRuntimeStatus(spec.ID, "done", stepDoneActivity(stepKey), model.ID, runtime)
	s.emitChatState(ctx, projectID, "")
	_ = taskID
	return output, nil
}

func (s *Service) AgentStatuses() []agents.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := []string{
		agents.ManagerID,
		agents.ProductID,
		agents.ArchitectID,
		agents.DeveloperID,
		agents.TesterID,
		agents.ReviewerID,
		agents.SecurityID,
		agents.ResearcherID,
		agents.SourceReviewID,
		agents.AnalystID,
		agents.CTFScoutID,
		agents.CTFWebID,
		agents.CTFLFIID,
		agents.CTFRCEID,
		agents.CTFSQLiID,
		agents.CTFPwnID,
		agents.CTFCryptoID,
		agents.CTFReverseID,
		agents.CTFForensicsID,
		agents.CTFValidatorID,
	}
	out := make([]agents.Status, 0, len(order))
	for _, id := range order {
		if status, ok := s.agentStatuses[id]; ok {
			out = append(out, status)
		}
	}
	return out
}

func (s *Service) projectState(ctx context.Context, projectID string) (ProjectState, error) {
	if projectID == "" {
		return s.unboundChatState(ctx)
	}
	item, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return ProjectState{}, err
	}
	task, err := s.contextTask(ctx, projectID)
	if err != nil {
		return ProjectState{}, err
	}
	messages := []chat.Message{}
	toolInvocations := []toolruntime.Invocation{}
	var workflowRun *zw.Run
	workflowSteps := []zw.Step{}
	var workflowPlan *zw.Plan
	planSteps := []zw.PlanStep{}
	artifactsList := []artifacts.Artifact{}
	var taskBlueprint *blueprint.Blueprint
	var liveTaskSpec *taskspec.Spec
	var liveProjectMemory *projectmemory.Memory
	var clarification *ClarificationDTO
	proposedChanges := []changes.ProposedChange{}
	testRuns := []checks.TestRun{}
	reviewRuns := []reviews.ReviewRun{}
	webSources := []webresearch.Source{}
	var ctfWorkspace *CTFWorkspaceDTO
	var activeGroup *agentgroups.Group
	var groupBinding *agentgroups.ProjectBinding
	if task != nil {
		ctx = s.chatGroupContext(ctx, *task, task.GroupID)
	}
	binding, err := s.store.ProjectGroupBinding(ctx, projectID)
	if err != nil {
		return ProjectState{}, err
	}
	groupBinding = &binding
	group, err := s.store.GetAgentGroup(ctx, binding.GroupID)
	if err != nil {
		return ProjectState{}, err
	}
	activeGroup = &group
	if task != nil {
		messages, err = s.store.ListMessages(ctx, task.ID)
		if err != nil {
			return ProjectState{}, err
		}
		toolInvocations, err = s.store.ListToolInvocations(ctx, task.ID)
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
		if savedSpec, specErr := s.store.LatestTaskSpecByTask(ctx, task.ID); specErr == nil {
			liveTaskSpec = &savedSpec
		}
	}
	artifactsList, err = s.store.ListArtifacts(ctx, projectID, 20)
	if err != nil {
		return ProjectState{}, err
	}
	if task != nil {
		filtered := artifactsList[:0]
		for _, artifact := range artifactsList {
			if artifact.TaskID == task.ID {
				filtered = append(filtered, artifact)
			}
		}
		artifactsList = filtered
	}
	if task != nil && workflowRun != nil {
		ctfWorkspace = s.buildCTFWorkspaceState(item, task, workflowRun, workflowSteps, artifactsList)
	}
	if memory, memoryErr := s.ensureProjectMemory(ctx, item); memoryErr == nil && strings.TrimSpace(memory.ID) != "" {
		liveProjectMemory = &memory
	}
	return ProjectState{
		Project:         item,
		Task:            task,
		Messages:        messages,
		ToolInvocations: toolInvocations,
		WorkflowRun:     workflowRun,
		WorkflowSteps:   workflowSteps,
		WorkflowPlan:    workflowPlan,
		PlanSteps:       planSteps,
		Artifacts:       artifactsList,
		Blueprint:       taskBlueprint,
		Clarification:   clarification,
		TaskSpec:        liveTaskSpec,
		ProjectMemory:   liveProjectMemory,
		Changes:         proposedChanges,
		TestRuns:        testRuns,
		Reviews:         reviewRuns,
		WebSources:      webSources,
		CTFWorkspace:    ctfWorkspace,
		AgentGroup:      activeGroup,
		GroupBinding:    groupBinding,
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
	case "python", "python3", ".venv/bin/python", ".venv/bin/python3":
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
	if err != nil || taskBlueprint == nil {
		return drafts
	}
	return devworkspace.EnsureDrafts(project.Path, *taskBlueprint, drafts)
}

func ensureGoModVersion(content string) string {
	return devworkspace.EnsureGoModVersion(content)
}

func goVersionAtLeast125(value string) bool {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > 1 || major == 1 && minor >= 25
}

func draftIndexByPath(drafts []changes.Draft, path string) (int, bool) {
	path = filepath.ToSlash(strings.Trim(path, "/"))
	for index, draft := range drafts {
		if filepath.ToSlash(strings.Trim(draft.FilePath, "/")) == path {
			return index, true
		}
	}
	return -1, false
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
	return devworkspace.PythonRequirementsContent(items)
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

type agentRuntimeStatusUpdate struct {
	ToolID       string
	SoulPath     string
	StepKey      string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

func (s *Service) setAgentStatus(agentID string, status string, activity string, modelID string) {
	s.setAgentRuntimeStatus(agentID, status, activity, modelID, agentRuntimeStatusUpdate{})
}

func (s *Service) setAgentRuntimeStatus(agentID string, status string, activity string, modelID string, update agentRuntimeStatusUpdate) {
	role, name := agents.Describe(agentID)
	now := nowString()
	value := agents.Status{
		ID:           agentID,
		Role:         role,
		Name:         name,
		Status:       status,
		Activity:     activity,
		ModelID:      modelID,
		ToolID:       update.ToolID,
		SoulPath:     update.SoulPath,
		StepKey:      update.StepKey,
		InputTokens:  update.InputTokens,
		OutputTokens: update.OutputTokens,
		TotalTokens:  update.TotalTokens,
		UpdatedAt:    now,
	}

	s.mu.Lock()
	if previous, ok := s.agentStatuses[agentID]; ok {
		if value.ToolID == "" {
			value.ToolID = previous.ToolID
		}
		if value.SoulPath == "" {
			value.SoulPath = previous.SoulPath
		}
		if value.StepKey == "" {
			value.StepKey = previous.StepKey
		}
		if isActiveAgentStatus(status) && isActiveAgentStatus(previous.Status) {
			value.StartedAt = previous.StartedAt
		}
	}
	if isActiveAgentStatus(status) && value.StartedAt == "" {
		value.StartedAt = now
	}
	value.ElapsedMS = elapsedMS(value.StartedAt, now)
	if !isActiveAgentStatus(status) {
		value.StartedAt = ""
	}
	s.agentStatuses[agentID] = value
	s.mu.Unlock()

	if s.sink != nil {
		s.sink.Emit("agent_status_changed", value)
	}
}

func isActiveAgentStatus(status string) bool {
	switch status {
	case "thinking", "orchestrating", "calling_model", "answering", "writing_files", "running", "searching_web", "waiting_user", "needs_work":
		return true
	default:
		return false
	}
}

func elapsedMS(startedAt string, finishedAt string) int64 {
	startedAt = strings.TrimSpace(startedAt)
	finishedAt = strings.TrimSpace(finishedAt)
	if startedAt == "" || finishedAt == "" {
		return 0
	}
	start, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		return 0
	}
	finish, err := time.Parse(time.RFC3339, finishedAt)
	if err != nil {
		return 0
	}
	if finish.Before(start) {
		return 0
	}
	return finish.Sub(start).Milliseconds()
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

func buildCTFIntakeInput(userMessage string, project project.Project, history []chat.Message, workspace ctf.Workspace) string {
	var builder strings.Builder
	builder.WriteString("# CTF request\n")
	builder.WriteString(userMessage)
	builder.WriteString("\n\n# Workspace\n")
	builder.WriteString(ctfWorkspacePrompt(project, workspace))
	if len(history) > 0 {
		builder.WriteString("\n\n# Recent chat\n")
		for _, item := range recentMessages(history, 6) {
			role := item.Role
			if item.AgentID != "" {
				role = item.AgentID
			}
			builder.WriteString("- ")
			builder.WriteString(role)
			builder.WriteString(": ")
			builder.WriteString(shortenForPrompt(item.Content, 320))
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

func buildCTFScopeInput(userMessage string, project project.Project, workspace ctf.Workspace, intake string) string {
	return strings.TrimSpace(`
# User request
` + userMessage + `

# Workspace
` + ctfWorkspacePrompt(project, workspace) + `

# Tool profile
` + ctf.ToolProfileID(workspace.Category) + `

# Intake
` + intake + `

Проверь scope именно для этой CTF/lab задачи. Не проси разрешение для локальных файлов, docker/lab или явно CTF challenge. Для внешней цели без разрешения остановись и сформулируй, что пользователь должен подтвердить.
`)
}

func buildCTFArtifactInput(userMessage string, project project.Project, workspace ctf.Workspace, intake string, scopeOutput string) string {
	return strings.TrimSpace(`
# User request
` + userMessage + `

# Workspace
` + ctfWorkspacePrompt(project, workspace) + `

# Intake
` + intake + `

# Scope
` + scopeOutput + `

Собери список известных артефактов из запроса и workspace. Не утверждай, что запускал команды или читал файлы, если результатов нет во входных данных.
`)
}

func buildCTFTriageInput(userMessage string, project project.Project, workspace ctf.Workspace, artifactsOutput string) string {
	return strings.TrimSpace(`
# User request
` + userMessage + `

# Workspace
` + ctfWorkspacePrompt(project, workspace) + `

# Artifacts
` + artifactsOutput + `

Подтверди категорию и выбери краткую стратегию решения.
`)
}

func buildCTFHypothesisInput(workspace ctf.Workspace, intake string, scopeOutput string, triageOutput string) string {
	return strings.TrimSpace(`
# Workspace
` + ctfWorkspacePrompt(project.Project{}, workspace) + `

# Intake
` + intake + `

# Scope
` + scopeOutput + `

# Triage
` + triageOutput + `

Собери гипотезы решения и приоритеты проверок.
`)
}

func buildCTFSolverInput(userMessage string, project project.Project, workspace ctf.Workspace, intake string, scopeOutput string, artifactsOutput string, triageOutput string, hypothesisOutput string) string {
	return strings.TrimSpace(`
# User request
` + userMessage + `

# Workspace
` + ctfWorkspacePrompt(project, workspace) + `

# Intake
` + intake + `

# Scope
` + scopeOutput + `

# Artifacts
` + artifactsOutput + `

# Triage
` + triageOutput + `

# Hypotheses
` + hypothesisOutput + `

Решай как CTF/lab задачу категории workspace.category. Если нет данных для flag, дай воспроизводимый план и TODO вместо выдуманного результата.
`)
}

func buildCTFValidationInput(workspace ctf.Workspace, solverOutput string) string {
	return strings.TrimSpace(`
# Workspace
` + ctfWorkspacePrompt(project.Project{}, workspace) + `

# Solver output
` + solverOutput + `

Проверь воспроизводимость, evidence, scope и writeup-readiness. Не выдумывай flag.
`)
}

func buildCTFWriteupInput(workspace ctf.Workspace, intake string, scopeOutput string, artifactsOutput string, triageOutput string, hypothesisOutput string, solverOutput string, validationOutput string) string {
	return strings.TrimSpace(`
# Workspace
` + ctfWorkspacePrompt(project.Project{}, workspace) + `

# Intake
` + intake + `

# Scope
` + scopeOutput + `

# Artifacts
` + artifactsOutput + `

# Triage
` + triageOutput + `

# Hypotheses
` + hypothesisOutput + `

# Solver
` + solverOutput + `

# Validation
` + validationOutput + `

Собери writeup. Если flag неизвестен, оставь TODO.
`)
}

func ctfWorkspacePrompt(project project.Project, workspace ctf.Workspace) string {
	var builder strings.Builder
	if project.Name != "" {
		builder.WriteString(fmt.Sprintf("- project: %s\n", project.Name))
	}
	builder.WriteString(fmt.Sprintf("- title: %s\n", workspace.Title))
	builder.WriteString(fmt.Sprintf("- category: %s\n", workspace.Category))
	builder.WriteString(fmt.Sprintf("- scope_status: %s\n", workspace.ScopeStatus))
	builder.WriteString(fmt.Sprintf("- requires_scope: %t\n", workspace.RequiresScope))
	builder.WriteString(fmt.Sprintf("- root: %s\n", workspace.RelativeRoot))
	builder.WriteString(fmt.Sprintf("- challenge: %s\n", workspace.ChallengeYAML))
	builder.WriteString(fmt.Sprintf("- scope: %s\n", workspace.ScopeMD))
	builder.WriteString(fmt.Sprintf("- notes: %s\n", workspace.NotesMD))
	builder.WriteString(fmt.Sprintf("- evidence_dir: %s\n", workspace.EvidenceDir))
	builder.WriteString(fmt.Sprintf("- evidence_index: %s\n", workspace.EvidenceIndex))
	builder.WriteString(fmt.Sprintf("- evidence_events: %s\n", workspace.EvidenceEvents))
	builder.WriteString(fmt.Sprintf("- solve_dir: %s\n", workspace.SolveDir))
	builder.WriteString(fmt.Sprintf("- writeup: %s\n", workspace.WriteupMD))
	if len(workspace.AllowedActions) > 0 {
		builder.WriteString("- allowed_actions: ")
		builder.WriteString(strings.Join(workspace.AllowedActions, ", "))
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func ctfScopeRequiredAnswer(workspace ctf.Workspace) string {
	return strings.TrimSpace(fmt.Sprintf(
		"## Нужен scope\n\nКатегория: `%s`\nWorkspace: `%s`\n\nПеред активными сетевыми действиями нужно явно подтвердить разрешение и границы цели в `%s`: target, authorization, allowed actions и ограничения по rate limit.",
		workspace.Category,
		workspace.RelativeRoot,
		workspace.ScopeMD,
	))
}

func ctfDoneAnswer(workspace ctf.Workspace, solverName string) string {
	return strings.TrimSpace(fmt.Sprintf(
		"## CTF workspace готов\n\nКатегория: `%s`\nSolver: %s\nWorkspace: `%s`\nEvidence: `%s`\nWriteup: `%s`",
		workspace.Category,
		solverName,
		workspace.RelativeRoot,
		workspace.EvidenceIndex,
		workspace.WriteupMD,
	))
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
	builder.WriteString("- Не дублируй полный список источников: UI покажет источники отдельным блоком.\n")
	builder.WriteString("- Если ссылка нужна в тексте рядом с важным утверждением, пиши ее строго как обычный Markdown: [название](https://example.com). Не вкладывай одну ссылку внутрь другой и не экранируй круглые скобки.\n")
	builder.WriteString("- Не выводи сырой JSON, YAML или служебные dumps в чат.\n")
	builder.WriteString("- Не придумывай факты, которых нет в источниках.\n")
	return strings.TrimSpace(builder.String())
}

func buildResearchSourceReviewInput(userMessage string, project project.Project, plan webresearch.Plan, sources []webresearch.Source) string {
	var builder strings.Builder
	builder.WriteString("Проект: ")
	builder.WriteString(project.Name)
	builder.WriteString("\nЗапрос пользователя:\n")
	builder.WriteString(userMessage)
	builder.WriteString("\n\nПлан поиска:\n")
	if planJSON, err := json.MarshalIndent(plan, "", "  "); err == nil {
		builder.Write(planJSON)
	}
	builder.WriteString("\n\nИсточники для проверки:\n")
	builder.WriteString(webresearch.FormatSourcesForPrompt(sources))
	builder.WriteString("\n\nПроверь свежесть, trust level, прямые ссылки, противоречия и достаточность источников.")
	return strings.TrimSpace(builder.String())
}

func buildResearchSynthesisInput(userMessage string, project project.Project, plan webresearch.Plan, sources []webresearch.Source, sourceReview string) string {
	var builder strings.Builder
	builder.WriteString(buildWebResearchAnswerInput(userMessage, project, plan, sources))
	builder.WriteString("\n\nSource review:\n")
	builder.WriteString(sourceReview)
	builder.WriteString("\n\nСобери ответ для пользователя: коротко, по делу, с явным отделением фактов от выводов. Полный список источников не включай: он отображается отдельным UI-блоком.")
	return strings.TrimSpace(builder.String())
}

func buildResearchNotesInput(userMessage string, project project.Project, plan webresearch.Plan, sources []webresearch.Source, sourceReview string, synthesis string) string {
	var builder strings.Builder
	builder.WriteString("Проект: ")
	builder.WriteString(project.Name)
	builder.WriteString("\nЗапрос пользователя:\n")
	builder.WriteString(userMessage)
	builder.WriteString("\n\nПлан поиска:\n")
	if planJSON, err := json.MarshalIndent(plan, "", "  "); err == nil {
		builder.Write(planJSON)
	}
	builder.WriteString("\n\nИсточники:\n")
	builder.WriteString(webresearch.FormatSourcesForPrompt(sources))
	builder.WriteString("\n\nSource review:\n")
	builder.WriteString(sourceReview)
	builder.WriteString("\n\nSynthesis:\n")
	builder.WriteString(synthesis)
	builder.WriteString("\n\nСохрани краткие research notes для будущих задач проекта.")
	return strings.TrimSpace(builder.String())
}

func (s *Service) saveResearchNotes(ctx context.Context, currentProject project.Project, task chat.Task, workflowRunID string, notes string) (string, error) {
	relativePath := filepath.Join("docs", "research-notes.md")
	path := filepath.Join(currentProject.Path, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	existing, _ := os.ReadFile(path)
	var builder strings.Builder
	builder.Write(existing)
	if strings.TrimSpace(string(existing)) != "" {
		builder.WriteString("\n\n---\n\n")
	}
	builder.WriteString(strings.TrimSpace(notes))
	builder.WriteString("\n")
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		return "", err
	}
	_, err := s.store.CreateArtifact(ctx, artifacts.Artifact{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRunID,
		AgentID:       agents.ResearcherID,
		Kind:          "research_notes",
		Title:         "Research notes",
		Path:          path,
		RelativePath:  relativePath,
	})
	if err != nil {
		return "", err
	}
	return relativePath, nil
}

func webResearchFallbackAnswer(sources []webresearch.Source) string {
	var builder strings.Builder
	builder.WriteString("## Нашла источники\n\n")
	builder.WriteString(fmt.Sprintf("Я собрала материалы, но модель не смогла надежно сформировать итоговый текст. Источники показаны отдельным блоком: %d.", len(sources)))
	return strings.TrimSpace(builder.String())
}

func looksLikeRawJSONAnswer(value string) bool {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return false
	}
	if !(strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
		return false
	}
	var parsed any
	return json.Unmarshal([]byte(trimmed), &parsed) == nil
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
	builder.WriteString(fmt.Sprintf("Итерация: %d из %d.\n", auto.Iterations, s.maxRepairIterationsForProject(ctx, project.ID)+1))
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
	builder.WriteString("\nПредложи минимальные команды проверки только из auto-раздела Code Execution Policy V0.8.4, только если они подходят структуре проекта и связаны с примененными изменениями текущего workflow. Не предлагай confirm/deny команды как автопроверки. Не запускай Python-проверки, если в текущем workflow не менялись Python-файлы. Не запускай npm-проверки, если не менялись frontend/package файлы. Для Go-изменений предпочитай go test ./... и не добавляй проверки другого стека. Для Python используй только .venv/bin/python: entrypoint, -m pytest или -m py_compile <file.py>.")
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
	gateReport := s.reviewGateReport(ctx, project, workflowRunID)

	var builder strings.Builder
	builder.WriteString("Проект: ")
	builder.WriteString(project.Name)
	builder.WriteString("\nПуть проекта не используй как источник фактов, ревью делай только по данным ниже.\n")
	builder.WriteString("\n# Task Blueprint\n")
	builder.WriteString(blueprint.ToPrompt(taskBlueprint))
	builder.WriteString("\n")
	builder.WriteString("\n")
	builder.WriteString(reviewgate.RenderPrompt(gateReport))
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

func (s *Service) reviewGateReport(ctx context.Context, project project.Project, workflowRunID string) reviewgate.Report {
	changesList, _ := s.store.ListProposedChanges(ctx, project.ID, workflowRunID, 100)
	testRuns, _ := s.store.ListTestRuns(ctx, project.ID, workflowRunID, 100)
	testRuns = s.refreshUnsupportedTestRuns(ctx, project, testRuns)
	testRuns = latestTestRunsByCommand(testRuns)
	taskBlueprint, _ := s.store.LatestTaskBlueprint(ctx, workflowRunID)
	var taskSpec *taskspec.Spec
	if task, err := s.store.WorkflowTask(ctx, workflowRunID, project.ID); err == nil {
		if spec, err := s.store.LatestTaskSpecByTask(ctx, task.ID); err == nil && strings.TrimSpace(spec.ID) != "" {
			taskSpec = &spec
		}
	}
	return reviewgate.Build(reviewgate.Input{
		TaskSpec:  taskSpec,
		Blueprint: taskBlueprint,
		Changes:   changesList,
		Tests:     testRuns,
	})
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

func (s *Service) buildDirectAnswerInput(ctx context.Context, project project.Project, task chat.Task, latestRun *zw.Run, userMessage string, decision router.Decision, orch orchestration.Decision) string {
	var builder strings.Builder
	builder.WriteString("# User request\n")
	builder.WriteString(userMessage)
	builder.WriteString("\n\n# Router decision\n")
	builder.WriteString(fmt.Sprintf("- intent: %s\n- confidence: %s\n- reason: %s\n", decision.Intent, decision.Confidence, decision.Reason))
	if strings.TrimSpace(orch.Explanation) != "" {
		builder.WriteString("\n# Orchestration decision\n")
		builder.WriteString(fmt.Sprintf("- mode: %s\n- group: %s (%s)\n- lifecycle: %s\n- used_memory: %t\n- explanation: %s\n", orch.Mode, orch.GroupName, orch.GroupKind, orch.LifecycleID, orch.UsedMemory, orch.Explanation))
		if len(orch.SkippedSteps) > 0 {
			builder.WriteString("- skipped_steps: ")
			builder.WriteString(strings.Join(orch.SkippedSteps, ", "))
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n# Project\n")
	builder.WriteString(fmt.Sprintf("- name: %s\n- path: %s\n", project.Name, project.Path))

	if spec, err := s.store.LatestTaskSpecByTask(ctx, task.ID); err == nil && strings.TrimSpace(spec.ID) != "" {
		builder.WriteString("\n# Live Task Spec\n")
		builder.WriteString(taskspec.RenderMarkdown(spec))
	}

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

	if project.ID == "" {
		return builder.String()
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
		if taskID, ok := ctx.Value(chatContextKey{}).(string); ok && item.TaskID != taskID {
			continue
		}
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

func (s *Service) resetTaskSpecForWorkflow(ctx context.Context, currentProject project.Project, task chat.Task, workflowRunID string, userRequest string) error {
	_, err := s.store.UpsertTaskSpec(ctx, taskspec.Spec{
		ProjectID:     currentProject.ID,
		TaskID:        task.ID,
		WorkflowRunID: workflowRunID,
		UserRequest:   userRequest,
		Summary:       titleFromContent(userRequest),
		Goal:          userRequest,
		Status:        taskspec.StatusActive,
		Source:        "user_request",
	})
	return err
}

func (s *Service) updateTaskSpecFromIntake(ctx context.Context, currentProject project.Project, taskID string, workflowRunID string, intake managerIntakeResult) error {
	status := taskspec.StatusActive
	if intake.NeedsClarification {
		status = taskspec.StatusWaitingClarification
	}
	return s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:     currentProject.ID,
		TaskID:        taskID,
		WorkflowRunID: workflowRunID,
		Summary:       intake.Summary,
		Goal:          intake.Goal,
		Requirements:  intake.Constraints,
		OpenQuestions: intake.OpenQuestions,
		Status:        status,
		Source:        "manager_intake",
	}, true)
}

func (s *Service) updateTaskSpecFromProduct(ctx context.Context, projectID string, taskID string, workflowRunID string, productOutput string) error {
	requirements := markdownItemsFromSections(productOutput, "функциональные требования", "требования", "requirements")
	acceptance := markdownItemsFromSections(productOutput, "критерии готовности", "acceptance criteria", "definition of done")
	return s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:          projectID,
		TaskID:             taskID,
		WorkflowRunID:      workflowRunID,
		Requirements:       requirements,
		AcceptanceCriteria: acceptance,
		Status:             taskspec.StatusActive,
		Source:             "product_requirements",
	}, false)
}

func (s *Service) updateTaskSpecFromBlueprint(ctx context.Context, projectID string, taskID string, workflowRunID string, taskBlueprint blueprint.Blueprint) error {
	return s.patchTaskSpec(ctx, taskspec.Spec{
		ProjectID:          projectID,
		TaskID:             taskID,
		WorkflowRunID:      workflowRunID,
		Requirements:       requirementsFromBlueprint(taskBlueprint),
		AcceptanceCriteria: acceptanceFromBlueprint(taskBlueprint),
		Decisions:          decisionsFromBlueprint(taskBlueprint),
		OpenQuestions:      taskBlueprint.OpenQuestions,
		Status:             taskspec.StatusActive,
		Source:             "task_blueprint",
	}, len(taskBlueprint.OpenQuestions) > 0)
}

func (s *Service) patchTaskSpec(ctx context.Context, patch taskspec.Spec, replaceOpenQuestions bool) error {
	if strings.TrimSpace(patch.ProjectID) == "" || strings.TrimSpace(patch.TaskID) == "" {
		return nil
	}
	current, err := s.store.LatestTaskSpecByTask(ctx, patch.TaskID)
	if err != nil || strings.TrimSpace(current.ID) == "" {
		current = taskspec.Spec{
			ProjectID: patch.ProjectID,
			TaskID:    patch.TaskID,
			Status:    taskspec.StatusDraft,
		}
	}
	if patch.WorkflowRunID != "" {
		current.WorkflowRunID = patch.WorkflowRunID
	}
	if patch.UserRequest != "" {
		current.UserRequest = patch.UserRequest
	}
	if patch.Summary != "" {
		current.Summary = patch.Summary
	}
	if patch.Goal != "" {
		current.Goal = patch.Goal
	}
	current.Requirements = taskspec.MergeStringLists(current.Requirements, patch.Requirements)
	current.AcceptanceCriteria = taskspec.MergeStringLists(current.AcceptanceCriteria, patch.AcceptanceCriteria)
	current.Decisions = taskspec.MergeStringLists(current.Decisions, patch.Decisions)
	if replaceOpenQuestions {
		current.OpenQuestions = taskspec.MergeStringLists(nil, patch.OpenQuestions)
	} else {
		current.OpenQuestions = taskspec.MergeStringLists(current.OpenQuestions, patch.OpenQuestions)
	}
	current.AcceptedAnswers = taskspec.MergeAnswers(current.AcceptedAnswers, patch.AcceptedAnswers)
	if patch.Status != "" {
		current.Status = patch.Status
	}
	if patch.Source != "" {
		current.Source = patch.Source
	}
	_, err = s.store.UpsertTaskSpec(ctx, current)
	return err
}

func (s *Service) ensureProjectMemory(ctx context.Context, currentProject project.Project) (projectmemory.Memory, error) {
	if currentProject.ID == "" || currentProject.Path == "" {
		return projectmemory.Memory{}, fmt.Errorf("рабочий проект не выбран")
	}
	current, err := s.store.ProjectMemory(ctx, currentProject.ID)
	if err != nil {
		return projectmemory.Memory{}, err
	}
	filesystemMemory := projectMemoryFromFilesystem(currentProject)
	if strings.TrimSpace(current.ID) == "" {
		return s.store.UpsertProjectMemory(ctx, filesystemMemory)
	}
	merged := projectmemory.Merge(current, filesystemMemory)
	if projectMemoryEqual(current, merged) {
		return current, nil
	}
	merged.ID = current.ID
	merged.CreatedAt = current.CreatedAt
	return s.store.UpsertProjectMemory(ctx, merged)
}

func (s *Service) patchProjectMemory(ctx context.Context, patch projectmemory.Memory) error {
	if strings.TrimSpace(patch.ProjectID) == "" {
		return nil
	}
	current, err := s.store.ProjectMemory(ctx, patch.ProjectID)
	if err != nil {
		return err
	}
	merged := projectmemory.Merge(current, patch)
	if strings.TrimSpace(merged.ProjectID) == "" {
		merged.ProjectID = patch.ProjectID
	}
	if current.ID != "" {
		merged.ID = current.ID
		merged.CreatedAt = current.CreatedAt
	}
	_, err = s.store.UpsertProjectMemory(ctx, merged)
	return err
}

func (s *Service) updateProjectMemoryFromBlueprint(ctx context.Context, projectID string, taskID string, taskBlueprint blueprint.Blueprint) error {
	memory := projectmemory.Memory{
		ProjectID:         projectID,
		Stack:             taskBlueprint.Stack,
		Runtime:           taskBlueprint.Runtime,
		ProjectType:       taskBlueprint.ProjectType,
		UpdatedFromTaskID: taskID,
	}
	for _, command := range taskBlueprint.TestCommands {
		if strings.TrimSpace(command.Command) != "" {
			memory.TestCommands = append(memory.TestCommands, strings.TrimSpace(command.Command))
		}
	}
	memory.Decisions = decisionsFromBlueprint(taskBlueprint)
	if taskBlueprint.Dependencies.Policy != "" {
		memory.Environment = append(memory.Environment, "Dependency policy: "+taskBlueprint.Dependencies.Policy)
	}
	for _, dep := range taskBlueprint.Dependencies.Items {
		memory.Environment = append(memory.Environment, "Dependency: "+dep)
	}
	return s.patchProjectMemory(ctx, memory)
}

func (s *Service) updateProjectMemoryFromWorkflow(ctx context.Context, projectID string, taskID string, workflowRunID string, result v03WorkflowResult, auto autopilotResult) error {
	memory := projectmemory.Memory{
		ProjectID:         projectID,
		UpdatedFromTaskID: taskID,
	}
	if result.Blueprint.Stack != "" || result.Blueprint.Runtime != "" || result.Blueprint.ProjectType != "" {
		memory.Stack = result.Blueprint.Stack
		memory.Runtime = result.Blueprint.Runtime
		memory.ProjectType = result.Blueprint.ProjectType
	}
	memory.Decisions = append(memory.Decisions, decisionsFromBlueprint(result.Blueprint)...)
	if spec, err := s.store.LatestTaskSpecByTask(ctx, taskID); err == nil && strings.TrimSpace(spec.ID) != "" {
		memory.Decisions = append(memory.Decisions, spec.Decisions...)
		for _, answer := range spec.AcceptedAnswers {
			if answer.Question != "" && answer.Answer != "" {
				memory.Decisions = append(memory.Decisions, answer.Question+": "+answer.Answer)
			}
		}
	}
	testRuns, _ := s.store.ListTestRuns(ctx, projectID, workflowRunID, 30)
	for _, testRun := range latestTestRunsByCommand(testRuns) {
		if strings.TrimSpace(testRun.Command) == "" {
			continue
		}
		memory.TestCommands = append(memory.TestCommands, strings.TrimSpace(testRun.Command))
		if testRun.Status == checks.StatusPassed {
			memory.Environment = append(memory.Environment, "Проверка проходит: "+strings.TrimSpace(testRun.Command))
		}
	}
	if auto.Blocked && auto.BlockReason != "" {
		memory.Environment = append(memory.Environment, "Последний workflow был остановлен: "+auto.BlockReason)
	}
	return s.patchProjectMemory(ctx, memory)
}

func projectMemoryFromFilesystem(currentProject project.Project) projectmemory.Memory {
	memory := projectmemory.Memory{ProjectID: currentProject.ID}
	stacks := []string{}
	addStack := func(value string) {
		stacks = projectmemory.MergeStringLists(stacks, []string{value})
	}
	if content := readProjectFileSnippet(currentProject.Path, "go.mod", 64*1024); content != "" {
		addStack("go")
		if runtime := goRuntimeFromMod(content); runtime != "" {
			memory.Runtime = runtime
		}
		if moduleName := goModuleName(content); moduleName != "" {
			memory.Environment = append(memory.Environment, "Go module: "+moduleName)
		}
		memory.BuildCommands = append(memory.BuildCommands, "go build ./...")
		memory.TestCommands = append(memory.TestCommands, "go test ./...")
		memory.StyleGuide = append(memory.StyleGuide, "Go code must be formatted with gofmt.")
	}
	if fileExists(filepath.Join(currentProject.Path, "requirements.txt")) || len(rootFilesWithSuffix(currentProject.Path, ".py")) > 0 {
		addStack("python")
		if fileExists(filepath.Join(currentProject.Path, "requirements.txt")) {
			memory.Environment = append(memory.Environment, "Python dependencies are declared in requirements.txt.")
		}
		if fileExists(filepath.Join(currentProject.Path, ".venv")) {
			memory.Environment = append(memory.Environment, "Python virtualenv exists at .venv.")
		} else {
			memory.Environment = append(memory.Environment, "Python tasks should use .venv.")
		}
		memory.TestCommands = append(memory.TestCommands, "python -m pytest")
	}
	if content := readProjectFileSnippet(currentProject.Path, "package.json", 64*1024); content != "" {
		addStack("node")
		memory.BuildCommands = append(memory.BuildCommands, packageJSONCommands(content, "")...)
	}
	if content := readProjectFileSnippet(currentProject.Path, filepath.Join("frontend", "package.json"), 64*1024); content != "" {
		addStack("frontend")
		memory.BuildCommands = append(memory.BuildCommands, packageJSONCommands(content, "frontend")...)
	}
	if content := readProjectFileSnippet(currentProject.Path, "Makefile", 64*1024); content != "" {
		memory.BuildCommands = append(memory.BuildCommands, makefileCommands(content, "build", "app", "dmg", "up", "run")...)
		memory.TestCommands = append(memory.TestCommands, makefileCommands(content, "test", "check", "lint")...)
		memory.Environment = append(memory.Environment, "Makefile is available for common project commands.")
	}
	if fileExists(filepath.Join(currentProject.Path, ".editorconfig")) {
		memory.StyleGuide = append(memory.StyleGuide, "Follow .editorconfig.")
	}
	if fileExists(filepath.Join(currentProject.Path, "README.md")) {
		memory.Environment = append(memory.Environment, "README.md contains project setup notes.")
	}
	if len(stacks) > 0 {
		memory.Stack = strings.Join(stacks, ", ")
	}
	memory.BuildCommands = projectmemory.MergeStringLists(nil, memory.BuildCommands)
	memory.TestCommands = projectmemory.MergeStringLists(nil, memory.TestCommands)
	memory.StyleGuide = projectmemory.MergeStringLists(nil, memory.StyleGuide)
	memory.Decisions = projectmemory.MergeStringLists(nil, memory.Decisions)
	memory.Environment = projectmemory.MergeStringLists(nil, memory.Environment)
	return memory
}

func goRuntimeFromMod(content string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "go" {
			return "Go " + fields[1] + "+"
		}
	}
	return ""
}

func goModuleName(content string) string {
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func packageJSONCommands(content string, prefix string) []string {
	var decoded struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(content), &decoded); err != nil {
		return nil
	}
	commands := []string{}
	for _, script := range []string{"build", "test", "lint", "dev"} {
		if _, ok := decoded.Scripts[script]; !ok {
			continue
		}
		command := "npm run " + script
		if prefix != "" {
			command += " --prefix " + prefix
		}
		commands = append(commands, command)
	}
	return commands
}

func makefileCommands(content string, targets ...string) []string {
	available := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		name, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, " $/") {
			continue
		}
		available[name] = true
	}
	commands := []string{}
	for _, target := range targets {
		if available[target] {
			commands = append(commands, "make "+target)
		}
	}
	return commands
}

func projectMemoryEqual(left projectmemory.Memory, right projectmemory.Memory) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func taskSpecAnswersFromClarifications(answers []ClarificationAnswer) []taskspec.AcceptedAnswer {
	out := make([]taskspec.AcceptedAnswer, 0, len(answers))
	for _, item := range answers {
		out = append(out, taskspec.AcceptedAnswer{
			QuestionID: item.QuestionID,
			Question:   item.Question,
			Answer:     item.Answer,
		})
	}
	return out
}

func taskSpecStatusFromWorkflow(status string) string {
	switch status {
	case zw.StatusDone:
		return taskspec.StatusDone
	case zw.StatusBlocked, zw.StatusFailed:
		return taskspec.StatusBlocked
	case zw.StatusWaitingUser:
		return taskspec.StatusWaitingClarification
	default:
		return taskspec.StatusActive
	}
}

func requirementsFromBlueprint(taskBlueprint blueprint.Blueprint) []string {
	out := []string{}
	for _, file := range taskBlueprint.ExpectedFiles {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		text := fmt.Sprintf("Файл `%s` должен быть %s", file.Path, firstNonEmpty(file.Action, "обновлен"))
		if file.Purpose != "" {
			text += ": " + file.Purpose
		}
		out = append(out, text)
	}
	for _, dep := range taskBlueprint.Dependencies.Items {
		out = append(out, "Зависимость: "+dep)
	}
	return out
}

func acceptanceFromBlueprint(taskBlueprint blueprint.Blueprint) []string {
	out := []string{}
	for _, command := range taskBlueprint.TestCommands {
		if strings.TrimSpace(command.Command) == "" {
			continue
		}
		text := "`" + strings.TrimSpace(command.Command) + "` проходит успешно"
		if command.Reason != "" {
			text += ": " + command.Reason
		}
		out = append(out, text)
	}
	if len(out) == 0 && len(taskBlueprint.ExpectedFiles) > 0 {
		out = append(out, "Ожидаемые файлы из Task Blueprint созданы или обновлены.")
	}
	return out
}

func decisionsFromBlueprint(taskBlueprint blueprint.Blueprint) []string {
	out := []string{}
	if taskBlueprint.Stack != "" {
		out = append(out, "Стек: "+taskBlueprint.Stack)
	}
	if taskBlueprint.Runtime != "" {
		out = append(out, "Runtime: "+taskBlueprint.Runtime)
	}
	if taskBlueprint.ProjectType != "" {
		out = append(out, "Тип проекта: "+taskBlueprint.ProjectType)
	}
	if taskBlueprint.ScaffoldRequired {
		out = append(out, "Scaffold нужен.")
	} else {
		out = append(out, "Scaffold не нужен.")
	}
	for _, entrypoint := range taskBlueprint.Entrypoints {
		out = append(out, "Entrypoint: "+entrypoint)
	}
	for _, forbidden := range taskBlueprint.ForbiddenFiles {
		out = append(out, "Не менять: "+forbidden)
	}
	return out
}

func markdownItemsFromSections(content string, names ...string) []string {
	section := markdownSection(content, names...)
	if section == "" {
		section = content
	}
	lines := strings.Split(section, "\n")
	items := []string{}
	for _, line := range lines {
		item := strings.TrimSpace(line)
		item = strings.TrimPrefix(item, "- ")
		item = strings.TrimPrefix(item, "* ")
		item = trimNumberedPrefix(item)
		item = strings.TrimSpace(item)
		if item == "" || strings.HasPrefix(item, "#") {
			continue
		}
		if len([]rune(item)) > 220 {
			item = shortenForPrompt(item, 220)
		}
		items = append(items, item)
	}
	return taskspec.MergeStringLists(nil, items)
}

func markdownSection(content string, names ...string) string {
	lines := strings.Split(content, "\n")
	start := -1
	end := len(lines)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
		if start == -1 {
			for _, name := range names {
				if strings.Contains(lower, strings.ToLower(strings.TrimSpace(name))) {
					start = index + 1
					break
				}
			}
			continue
		}
		end = index
		break
	}
	if start == -1 || start >= end {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

func trimNumberedPrefix(value string) string {
	for index, char := range value {
		if char >= '0' && char <= '9' || char == '.' || char == ')' {
			continue
		}
		if index == 0 {
			return value
		}
		return strings.TrimSpace(value[index:])
	}
	return value
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

func wantsProjectMemory(message string) bool {
	text := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(message))), " ")
	if !strings.Contains(text, "памят") && !strings.Contains(text, "memory") {
		return false
	}
	return strings.Contains(text, "проект") ||
		strings.Contains(text, "выведи") ||
		strings.Contains(text, "покажи") ||
		strings.Contains(text, "что помни")
}

func (s *Service) projectMemoryAnswer(ctx context.Context, currentProject project.Project) string {
	memory, err := s.ensureProjectMemory(ctx, currentProject)
	if err != nil || strings.TrimSpace(memory.ID) == "" {
		return "## Память проекта не найдена\nЯ пока не нашла сохраненную память для этого проекта."
	}
	return "## Память проекта\n\n" + projectmemory.RenderMarkdown(memory)
}

func (s *Service) savedTaskSpecAnswer(ctx context.Context, project project.Project, task *chat.Task) string {
	if task != nil {
		if spec, err := s.store.LatestTaskSpecByTask(ctx, task.ID); err == nil && strings.TrimSpace(spec.ID) != "" {
			return "## Спека задачи\n\n" + taskspec.RenderMarkdown(spec)
		}
		return "В этом чате пока нет сохранённой спецификации задачи."
	}
	if spec, err := s.store.LatestTaskSpecByProject(ctx, project.ID); err == nil && strings.TrimSpace(spec.ID) != "" {
		return "## Спека задачи\n\n" + taskspec.RenderMarkdown(spec)
	}
	content, relativePath := s.latestTaskSpecContent(ctx, project)
	if strings.TrimSpace(content) == "" {
		return "## Спека не найдена\nЯ не нашла сохраненную живую спеку для последнего задания. Похоже, workflow еще не формировал Task Spec Store для этого проекта."
	}
	var builder strings.Builder
	builder.WriteString("## Спека задачи\n\n")
	if relativePath != "" {
		builder.WriteString("Источник fallback: `")
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

func rollbackChangesMessage(rolledBack int, failed int) string {
	var builder strings.Builder
	builder.WriteString("## Rollback изменений\n")
	if rolledBack > 0 {
		builder.WriteString(fmt.Sprintf("Откатано файлов: %d.\n", rolledBack))
	}
	if failed > 0 {
		builder.WriteString(fmt.Sprintf("Не откатилось файлов: %d. Подробности видны в блоке изменений.\n", failed))
	}
	if rolledBack == 0 && failed == 0 {
		builder.WriteString("Нет примененных изменений для отката.")
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
	case zw.StepResearchSourceReview:
		return "Проверяет источники"
	case zw.StepResearchSynthesis:
		return "Сравнивает источники"
	case zw.StepResearchNotes:
		return "Сохраняет research notes"
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
	case zw.StepResearchSourceReview:
		return "Источники проверены"
	case zw.StepResearchSynthesis:
		return "Аналитика готова"
	case zw.StepResearchNotes:
		return "Research notes сохранены"
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
