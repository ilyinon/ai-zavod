package agents

import (
	"strings"
	"testing"

	"zavod_ai/internal/chat"
)

func TestManagerMessagesDoNotPrefixAgentHistory(t *testing.T) {
	messages := managerMessages([]chat.Message{
		{Role: "agent", AgentID: ManagerID, Content: "Понял задачу."},
	})

	last := messages[len(messages)-1]
	if last.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", last.Role)
	}
	if strings.Contains(last.Content, "Агент manager:") {
		t.Fatalf("agent prefix leaked into model history: %q", last.Content)
	}
	if last.Content != "Понял задачу." {
		t.Fatalf("expected original content, got %q", last.Content)
	}
}

func TestManagerMessagesSanitizeRepeatedAgentPrefix(t *testing.T) {
	messages := managerMessages([]chat.Message{
		{Role: "agent", AgentID: ManagerID, Content: strings.Repeat("Агент manager: ", 8)},
	})

	last := messages[len(messages)-1]
	if strings.Contains(last.Content, "Агент manager:") {
		t.Fatalf("repeated agent prefix leaked into model history: %q", last.Content)
	}
	if last.Content == "" {
		t.Fatal("expected sanitized content")
	}
}
