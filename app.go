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
