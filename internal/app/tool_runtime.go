package app

import (
	"context"
	"fmt"
	"slices"

	"zavod_ai/internal/agentgroups"
	"zavod_ai/internal/agents"
	"zavod_ai/internal/checks"
	"zavod_ai/internal/llm"
	"zavod_ai/internal/project"
	"zavod_ai/internal/toolruntime"
)

func (s *Service) toolProfile(ctx context.Context, projectID, role string) (agentgroups.Profile, error) {
	binding, err := s.store.ProjectGroupBinding(ctx, projectID)
	if err != nil {
		return agentgroups.Profile{}, err
	}
	profiles, err := s.store.ListAgentProfiles(ctx, binding.GroupID)
	if err != nil {
		return agentgroups.Profile{}, err
	}
	for _, profile := range profiles {
		if profile.Enabled && (profile.RoleKey == role || profile.ID == role) {
			return profile, nil
		}
	}
	return agentgroups.Profile{}, fmt.Errorf("в группе нет включенного агента для роли %s", role)
}

// Diagnosis delegates to an enabled agent; the manager does not inherit privileges.
func (s *Service) diagnosticProfile(ctx context.Context, p project.Project) (agentgroups.Profile, error) {
	suggestions := checks.DefaultSuggestions(p.Path)
	for _, role := range []string{"tester", "developer"} {
		profile, err := s.toolProfile(ctx, p.ID, role)
		if err != nil || !slices.Contains(agentgroups.RuntimeTools(profile), "run_check") {
			continue
		}
		for _, suggestion := range suggestions {
			if checks.ValidateCommandWithToolProfile(p.Path, profile.ToolProfileID, suggestion.Command, suggestion.WorkingDir) == nil {
				return profile, nil
			}
		}
	}
	return s.toolProfile(ctx, p.ID, "manager")
}

func (s *Service) generateProjectAnswer(ctx context.Context, p project.Project, taskID string, provider llm.Provider, model llm.ModelConfig, req llm.Request, consentModelID string) (*llm.Response, error) {
	// Explicit one-request consent is bound to the actual selected model.
	// Do not inspect files, execute checks or send tool data without it.
	if consentModelID == "" || consentModelID != model.ID {
		return &llm.Response{Content: "Для диагностики мне нужен доступ к файлам и проверкам проекта. Разреши его для следующего запроса: содержимое прочитанных файлов и вывод проверок будут переданы выбранной модели. Пока я не читала файлы инструментами и не запускала проверки."}, nil
	}
	profile, err := s.diagnosticProfile(ctx, p)
	if err != nil {
		return nil, err
	}
	spec := agents.Spec{ID: profile.RoleKey, Role: profile.RoleKey}
	soul := s.agentSoulForStep(ctx, p.ID, spec)
	req.Messages = append(req.Messages, llm.Message{Role: "system", Content: soul + "\nРежим диагностики проекта без изменений. Помоги Люмен ответить на вопрос пользователя, используя реальные результаты инструментов. Не создавай proposed changes и не запускай workflow. Если проверки недоступны по правам, прямо сообщи об этом."})
	scope := toolruntime.Scope{ProjectID: p.ID, TaskID: taskID, AgentID: profile.ID, AgentName: profile.Name, ModelID: model.ID, WorkingDir: p.Path, ToolProfileID: profile.ToolProfileID, AllowedTools: agentgroups.RuntimeTools(profile), ReadPaths: profile.ReadPaths}
	runtime := toolruntime.Runtime{Scope: scope, Record: func(saveCtx context.Context, inv toolruntime.Invocation) error {
		if err := s.store.SaveToolInvocation(saveCtx, inv); err != nil {
			return err
		}
		s.emitChatState(saveCtx, p.ID, "")
		return nil
	}}
	return runtime.Generate(ctx, provider, req)
}
