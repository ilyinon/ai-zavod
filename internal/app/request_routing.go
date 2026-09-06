package app

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"zavod_ai/internal/chat"
	"zavod_ai/internal/router"
	"zavod_ai/internal/store"
	zw "zavod_ai/internal/workflow"
)

func (s *Service) beginRoutedRequest(ctx context.Context, task chat.Task, input SendMessageInput, content string) (string, chat.RequestState, *router.Decision, error) {
	previous, err := s.store.RequestState(ctx, task.ID)
	if err != nil {
		return content, chat.RequestState{}, nil, err
	}
	state := chat.RequestState{ID: uuid.NewString(), Sequence: 1, Mode: "routing", Original: content}
	if previous != nil {
		state.Sequence = previous.Sequence + 1
	}
	var decision *router.Decision
	if input.ResumeWorkflowRunID != "" {
		if _, _, err := s.validateContinuation(ctx, task, input.ResumeWorkflowRunID); err != nil {
			return content, state, nil, err
		}
		if previous == nil || previous.WorkflowRunID != input.ResumeWorkflowRunID {
			return content, state, nil, fmt.Errorf("повтор относится к предыдущему запросу")
		}
		content = previous.Original
		state.Original = content
		decision = &router.Decision{Intent: router.IntentCodingTask, Confidence: "high", NeedsWorkflow: true, NeedsProjectContext: true, Source: "user", Reason: "явный повтор остановленного вызова модели"}
	}
	if input.RoutingAnswerFor != "" {
		if previous == nil || previous.ID != input.RoutingAnswerFor || previous.Mode != "clarify" {
			return content, state, nil, fmt.Errorf("это уточнение уже неактуально")
		}
		d := router.Decision{Intent: router.IntentDirectAnswer, Confidence: "high", Source: "user", Reason: "пользователь выбрал пример в чате"}
		switch content {
		case "Показать в чате":
		case "Добавить в проект":
			d.Intent = router.IntentCodingTask
			d.NeedsWorkflow = true
			d.NeedsProjectContext = true
			d.Reason = "пользователь подтвердил изменение проекта"
		default:
			return content, state, nil, fmt.Errorf("неизвестный ответ на уточнение режима")
		}
		content = previous.Original + "\n\nВыбранный режим: " + content
		state.Original = content
		decision = &d
	}
	err = s.store.SaveRequestState(ctx, task.ID, state)
	return content, state, decision, err
}

func (s *Service) requestStateForTask(ctx context.Context, task *chat.Task) *chat.RequestState {
	if task == nil {
		return nil
	}
	state, _ := s.store.RequestState(ctx, task.ID)
	return state
}
func (s *Service) failureForRun(ctx context.Context, run *zw.Run) *store.WorkflowFailure {
	if run == nil || run.Status == zw.StatusDone || run.Status == zw.StatusRunning {
		return nil
	}
	failure, _ := s.store.WorkflowFailure(ctx, run.ID)
	return failure
}
