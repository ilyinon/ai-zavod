package openaiapi

import "testing"

func TestParseStreamLine(t *testing.T) {
	event, ok := parseStreamLine(`data: {"choices":[{"delta":{"content":"Привет"}}]}`)
	if !ok {
		t.Fatal("expected stream event")
	}
	if event.Delta != "Привет" {
		t.Fatalf("expected delta, got %q", event.Delta)
	}
	if event.Done {
		t.Fatal("delta event must not be done")
	}
}

func TestParseStreamLineDone(t *testing.T) {
	event, ok := parseStreamLine("data: [DONE]")
	if !ok {
		t.Fatal("expected done event")
	}
	if !event.Done {
		t.Fatal("expected done")
	}
}

func TestNormalizeStreamDeltaHandlesCumulativeChunks(t *testing.T) {
	accumulated, delta, ok := normalizeStreamDelta("", "При")
	if !ok || delta != "При" || accumulated != "При" {
		t.Fatalf("unexpected first chunk: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}

	accumulated, delta, ok = normalizeStreamDelta(accumulated, "Привет")
	if !ok || delta != "вет" || accumulated != "Привет" {
		t.Fatalf("unexpected cumulative chunk: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}

	accumulated, delta, ok = normalizeStreamDelta(accumulated, "Привет!")
	if !ok || delta != "!" || accumulated != "Привет!" {
		t.Fatalf("unexpected cumulative suffix: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}
}

func TestNormalizeStreamDeltaKeepsRegularDeltaChunks(t *testing.T) {
	accumulated, delta, ok := normalizeStreamDelta("При", "вет")
	if !ok || delta != "вет" || accumulated != "Привет" {
		t.Fatalf("unexpected regular chunk: accumulated=%q delta=%q ok=%v", accumulated, delta, ok)
	}
}
