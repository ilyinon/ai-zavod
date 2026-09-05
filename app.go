package main

import (
	"context"
	"errors"

	appsvc "zavod_ai/internal/app"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx     context.Context
	service *appsvc.Service
	initErr error
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	service, err := appsvc.NewService(ctx, wailsEventSink{ctx: ctx})
	if err != nil {
		a.initErr = err
		return
	}
	a.service = service
}

func (a *App) Bootstrap() (appsvc.BootstrapState, error) {
	if err := a.ready(); err != nil {
		return appsvc.BootstrapState{}, err
	}
	return a.service.Bootstrap(a.ctx)
}

func (a *App) ListProjects(query string) ([]appsvc.ProjectDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListProjects(a.ctx, query)
}

func (a *App) ListChats() ([]appsvc.TaskDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListChats(a.ctx)
}

func (a *App) ChooseProjectFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Выбрать рабочую папку"})
}
func (a *App) CreateChat(input appsvc.CreateChatInput) (appsvc.ProjectState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ProjectState{}, err
	}
	return a.service.CreateChat(a.ctx, input)
}
func (a *App) SelectChat(id string) (appsvc.ProjectState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ProjectState{}, err
	}
	return a.service.SelectChat(a.ctx, id)
}
func (a *App) UpdateChat(input appsvc.UpdateChatInput) (appsvc.TaskDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.TaskDTO{}, err
	}
	return a.service.UpdateChat(a.ctx, input)
}
func (a *App) DeleteChat(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.service.DeleteChat(a.ctx, id)
}

func (a *App) CreateProject(input appsvc.CreateProjectInput) (appsvc.ProjectDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.ProjectDTO{}, err
	}
	return a.service.CreateProject(a.ctx, input)
}

func (a *App) AddExistingProject(input appsvc.AddExistingProjectInput) (appsvc.ProjectDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.ProjectDTO{}, err
	}
	return a.service.AddExistingProject(a.ctx, input)
}

func (a *App) UpdateProject(input appsvc.UpdateProjectInput) (appsvc.ProjectDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.ProjectDTO{}, err
	}
	return a.service.UpdateProject(a.ctx, input)
}

func (a *App) DeleteProject(input appsvc.DeleteProjectInput) (appsvc.BootstrapState, error) {
	if err := a.ready(); err != nil {
		return appsvc.BootstrapState{}, err
	}
	return a.service.DeleteProject(a.ctx, input)
}

func (a *App) SelectProject(projectID string) (appsvc.ProjectState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ProjectState{}, err
	}
	return a.service.SelectProject(a.ctx, projectID)
}

func (a *App) SendMessage(input appsvc.SendMessageInput) (appsvc.ChatState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ChatState{}, err
	}
	return a.service.SendMessage(a.ctx, input)
}

func (a *App) SubmitClarification(input appsvc.SubmitClarificationInput) (appsvc.ChatState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ChatState{}, err
	}
	return a.service.SubmitClarification(a.ctx, input)
}

func (a *App) ApplyWorkflowChanges(input appsvc.ApplyWorkflowChangesInput) (appsvc.ChatState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ChatState{}, err
	}
	return a.service.ApplyWorkflowChanges(a.ctx, input)
}

func (a *App) RollbackWorkflowChanges(input appsvc.RollbackWorkflowChangesInput) (appsvc.ChatState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ChatState{}, err
	}
	return a.service.RollbackWorkflowChanges(a.ctx, input)
}

func (a *App) RunTestCommand(input appsvc.RunTestCommandInput) (appsvc.ChatState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ChatState{}, err
	}
	return a.service.RunTestCommand(a.ctx, input)
}

func (a *App) RunReview(input appsvc.RunReviewInput) (appsvc.ChatState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ChatState{}, err
	}
	return a.service.RunReview(a.ctx, input)
}

func (a *App) SaveModelConfig(input appsvc.SaveModelConfigInput) ([]appsvc.ModelConfigDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.SaveModelConfig(a.ctx, input)
}

func (a *App) SetActiveModel(modelID string) ([]appsvc.ModelConfigDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.SetActiveModel(a.ctx, modelID)
}

func (a *App) CheckModel(modelID string) ([]appsvc.ModelConfigDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.CheckModel(a.ctx, modelID)
}

func (a *App) SaveWebSettings(input appsvc.SaveWebSettingsInput) (appsvc.WebSettingsDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.WebSettingsDTO{}, err
	}
	return a.service.SaveWebSettings(a.ctx, input)
}

func (a *App) ListAgentGroups() ([]appsvc.AgentGroupDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListAgentGroups(a.ctx)
}

func (a *App) CreateAgentGroup(input appsvc.CreateAgentGroupInput) ([]appsvc.AgentGroupDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.CreateAgentGroup(a.ctx, input)
}

func (a *App) ListAgentGroupTemplates() ([]appsvc.AgentGroupTemplateDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListAgentGroupTemplates(a.ctx)
}

func (a *App) CreateAgentGroupFromTemplate(input appsvc.CreateAgentGroupFromTemplateInput) ([]appsvc.AgentGroupDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.CreateAgentGroupFromTemplate(a.ctx, input)
}

func (a *App) ListAgentLibrary() ([]appsvc.AgentLibraryItemDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListAgentLibrary(a.ctx)
}

func (a *App) UpdateAgentGroup(input appsvc.UpdateAgentGroupInput) ([]appsvc.AgentGroupDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.UpdateAgentGroup(a.ctx, input)
}

func (a *App) ArchiveAgentGroup(input appsvc.ArchiveAgentGroupInput) ([]appsvc.AgentGroupDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ArchiveAgentGroup(a.ctx, input)
}

func (a *App) ListAgentProfiles(groupID string) ([]appsvc.AgentProfileDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListAgentProfiles(a.ctx, groupID)
}

func (a *App) SaveAgentProfile(input appsvc.SaveAgentProfileInput) ([]appsvc.AgentProfileDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.SaveAgentProfile(a.ctx, input)
}

func (a *App) AddAgentFromLibrary(input appsvc.AddAgentFromLibraryInput) ([]appsvc.AgentProfileDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.AddAgentFromLibrary(a.ctx, input)
}

func (a *App) DuplicateAgentProfile(input appsvc.DuplicateAgentProfileInput) ([]appsvc.AgentProfileDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.DuplicateAgentProfile(a.ctx, input)
}

func (a *App) ReplaceAgentSoulFromLibrary(input appsvc.ReplaceAgentSoulFromLibraryInput) (appsvc.AgentSoulDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.AgentSoulDTO{}, err
	}
	return a.service.ReplaceAgentSoulFromLibrary(a.ctx, input)
}

func (a *App) SetAgentProfileEnabled(input appsvc.SetAgentProfileEnabledInput) ([]appsvc.AgentProfileDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.SetAgentProfileEnabled(a.ctx, input)
}

func (a *App) GetAgentSoul(profileID string) (appsvc.AgentSoulDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.AgentSoulDTO{}, err
	}
	return a.service.GetAgentSoul(a.ctx, profileID)
}

func (a *App) SaveAgentSoul(input appsvc.SaveAgentSoulInput) (appsvc.AgentSoulDTO, error) {
	if err := a.ready(); err != nil {
		return appsvc.AgentSoulDTO{}, err
	}
	return a.service.SaveAgentSoul(a.ctx, input)
}

func (a *App) ListLifecycleDefinitions(groupID string) ([]appsvc.LifecycleDefinitionDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListLifecycleDefinitions(a.ctx, groupID)
}

func (a *App) ListLifecycleSteps(lifecycleID string) ([]appsvc.LifecycleStepDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.ListLifecycleSteps(a.ctx, lifecycleID)
}

func (a *App) SaveLifecycleDefinition(input appsvc.SaveLifecycleDefinitionInput) ([]appsvc.LifecycleDefinitionDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.SaveLifecycleDefinition(a.ctx, input)
}

func (a *App) SaveLifecycleStep(input appsvc.SaveLifecycleStepInput) ([]appsvc.LifecycleStepDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.SaveLifecycleStep(a.ctx, input)
}

func (a *App) DeleteLifecycleStep(input appsvc.DeleteLifecycleStepInput) ([]appsvc.LifecycleStepDTO, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.service.DeleteLifecycleStep(a.ctx, input)
}

func (a *App) BindProjectAgentGroup(input appsvc.BindProjectAgentGroupInput) (appsvc.ProjectState, error) {
	if err := a.ready(); err != nil {
		return appsvc.ProjectState{}, err
	}
	return a.service.BindProjectAgentGroup(a.ctx, input)
}

func (a *App) ready() error {
	if a.initErr != nil {
		return a.initErr
	}
	if a.service == nil {
		return errors.New("приложение еще не инициализировано")
	}
	return nil
}

type wailsEventSink struct {
	ctx context.Context
}

func (s wailsEventSink) Emit(name string, data any) {
	runtime.EventsEmit(s.ctx, name, data)
}
