package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestWeatherDoesNotFallBackToUngroundedDirectAnswer(t *testing.T) {
	for _, question := range []string{"Какая погода в нижнем Новгороде?", "чем отличается модель Astra от Open AI от Sol ?"} {
		t.Run(question, func(t *testing.T) {
			ctx := context.Background()
			s := chatTestService(t)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer server.Close()
			model, err := s.store.ActiveModelConfig(ctx)
			if err != nil {
				t.Fatal(err)
			}
			model.BaseURL = server.URL
			if _, err = s.store.SaveModelConfig(ctx, model); err != nil {
				t.Fatal(err)
			}
			if err = s.store.SetSetting(ctx, webSettingsKey, `{"enabled":false}`); err != nil {
				t.Fatal(err)
			}
			created, err := s.CreateChat(ctx, CreateChatInput{})
			if err != nil {
				t.Fatal(err)
			}
			state, err := s.SendMessage(ctx, SendMessageInput{TaskID: created.Task.ID, Content: question})
			if err != nil {
				t.Fatal(err)
			}
			if state.Error != "Поиск выключен в настройках" || calls.Load() != 0 || state.Task.PendingRequest != "" || state.WorkflowRun != nil {
				t.Fatalf("request must use sources, not guess or ask for workspace: error=%q calls=%d", state.Error, calls.Load())
			}
		})
	}
}
