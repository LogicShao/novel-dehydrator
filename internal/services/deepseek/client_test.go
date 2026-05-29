package deepseek

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LogicShao/novel-dehydrator/internal/config"
)

func TestNonStreaming(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"non-stream result"}}]}`))
	}))
	defer server.Close()

	client := NewClient(nil, &config.Config{
		DeepseekAPIKey:  "config-key",
		DeepseekModel:   "deepseek-chat",
		DeepseekBaseURL: server.URL,
		MaxRetries:      3,
	})

	result, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hello"}}, false)
	if err != nil {
		t.Fatalf("ChatCompletion returned error: %v", err)
	}
	if result != "non-stream result" {
		t.Fatalf("expected non-stream result, got %q", result)
	}
}

func TestStreaming(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	client := NewClient(nil, &config.Config{
		DeepseekAPIKey:  "config-key",
		DeepseekModel:   "deepseek-chat",
		DeepseekBaseURL: server.URL,
		MaxRetries:      3,
	})

	result, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hello"}}, true)
	if err != nil {
		t.Fatalf("ChatCompletion returned error: %v", err)
	}
	if result != "Hello world" {
		t.Fatalf("expected concatenated stream, got %q", result)
	}
}

func TestRetry429(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`rate limited`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"retried ok"}}]}`))
	}))
	defer server.Close()

	client := NewClient(nil, &config.Config{
		DeepseekAPIKey:  "config-key",
		DeepseekModel:   "deepseek-chat",
		DeepseekBaseURL: server.URL,
		MaxRetries:      2,
	})
	var slept []string
	client.sleep = func(dur time.Duration) { slept = append(slept, dur.String()) }

	result, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hello"}}, false)
	if err != nil {
		t.Fatalf("ChatCompletion returned error: %v", err)
	}
	if result != "retried ok" {
		t.Fatalf("expected retried ok, got %q", result)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 calls, got %d", calls.Load())
	}
	if len(slept) != 1 || slept[0] != "30s" {
		t.Fatalf("expected single 30s sleep, got %#v", slept)
	}
}

func TestConfigFallback(t *testing.T) {
	t.Parallel()

	var authHeader atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"fallback ok"}}]}`))
	}))
	defer server.Close()

	client := NewClient(nil, &config.Config{
		DeepseekAPIKey:  "fallback-key",
		DeepseekModel:   "deepseek-chat",
		DeepseekBaseURL: server.URL,
		MaxRetries:      1,
	})
	client.runtimeSettingsLoader = func(context.Context) (runtimeSettings, error) {
		return runtimeSettings{
			APIKey:  client.APIKey,
			Model:   client.Model,
			BaseURL: client.BaseURL,
		}, nil
	}

	result, err := client.ChatCompletion(context.Background(), []Message{{Role: "user", Content: "hello"}}, false)
	if err != nil {
		t.Fatalf("ChatCompletion returned error: %v", err)
	}
	if result != "fallback ok" {
		t.Fatalf("expected fallback ok, got %q", result)
	}
	if got := authHeader.Load(); got != "Bearer fallback-key" {
		t.Fatalf("expected fallback Authorization header, got %v", got)
	}
}

func TestMergeRuntimeSettingsPrefersDatabaseAPIKey(t *testing.T) {
	t.Parallel()

	settings := mergeRuntimeSettings(runtimeSettings{
		APIKey:  "config-key",
		Model:   "config-model",
		BaseURL: "https://config.example",
	}, "db-key", "db-model", "https://db.example")

	if settings.APIKey != "db-key" {
		t.Fatalf("APIKey = %q, want db-key", settings.APIKey)
	}
	if settings.Model != "db-model" {
		t.Fatalf("Model = %q, want db-model", settings.Model)
	}
	if settings.BaseURL != "https://db.example" {
		t.Fatalf("BaseURL = %q, want https://db.example", settings.BaseURL)
	}
}
