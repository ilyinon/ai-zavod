package openaiapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"zavod_ai/internal/llm"
)

func TestProviderRetryPolicy(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 502, 503, 504} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				w.Write([]byte("SECRET?api_key=private"))
			}))
			defer srv.Close()
			c := NewClient(srv.URL, "")
			c.retry = retryPolicy{time.Second, time.Second, []time.Duration{time.Millisecond, time.Millisecond}}
			_, err := c.Generate(context.Background(), llm.Request{Model: "mock"})
			var failure *llm.ProviderError
			want := int32(1)
			if status == 429 || status == 502 || status == 503 || status == 504 {
				want = 3
			}
			if !errors.As(err, &failure) || calls.Load() != want || failure.Attempt != int(want) || failure.HTTPStatus != status {
				t.Fatalf("calls=%d error=%#v", calls.Load(), err)
			}
			if strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "api_key") {
				t.Fatal("leaked response body")
			}
		})
	}
}

func TestRetrySuccessOnLastAttempt(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(503)
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "")
	c.retry = retryPolicy{time.Second, time.Second, []time.Duration{time.Millisecond, time.Millisecond}}
	got, err := c.Generate(context.Background(), llm.Request{Model: "mock"})
	if err != nil || got.Content != "ok" || calls.Load() != 3 {
		t.Fatalf("%+v %v calls=%d", got, err, calls.Load())
	}
}

func TestTimeoutCancellationAndClassifierBudget(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(100 * time.Millisecond):
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "")
	c.retry = retryPolicy{15 * time.Millisecond, 80 * time.Millisecond, []time.Duration{time.Millisecond, time.Millisecond}}
	_, err := c.Generate(context.Background(), llm.Request{Model: "mock", NoRetry: true})
	var p *llm.ProviderError
	if !errors.As(err, &p) || p.Kind != "provider_timeout" || calls.Load() != 1 {
		t.Fatalf("%v calls=%d", err, calls.Load())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = c.Generate(ctx, llm.Request{Model: "mock"})
	if !errors.As(err, &p) || p.Kind != "execution_deadline" {
		t.Fatalf("expected parent deadline: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	before := calls.Load()
	_, err = c.Generate(ctx, llm.Request{Model: "mock"})
	if !errors.As(err, &p) || p.Kind != "cancelled" || calls.Load() != before {
		t.Fatalf("expected cancellation: %v", err)
	}
}

func TestRetryAfterAndMalformedResponse(t *testing.T) {
	for _, status := range []int{429, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(status)
				w.Write([]byte("invalid"))
			}))
			defer srv.Close()
			c := NewClient(srv.URL, "")
			c.retry = retryPolicy{time.Second, 50 * time.Millisecond, []time.Duration{time.Millisecond, time.Millisecond}}
			_, err := c.Generate(context.Background(), llm.Request{Model: "mock"})
			if err == nil || calls.Load() != 1 {
				t.Fatalf("unexpected retry: %v %d", err, calls.Load())
			}
		})
	}
}
