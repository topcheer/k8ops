package anthropic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggai/k8ops/internal/provider"
)

func TestAnthropicProvider_Name(t *testing.T) {
	p := &AnthropicProvider{}
	if p.Name() != "anthropic" {
		t.Errorf("expected 'anthropic', got '%s'", p.Name())
	}
}

func TestAnthropicProvider_SimpleCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("unexpected api key header: %s", r.Header.Get("x-api-key"))
		}

		resp := map[string]any{
			"id":      "msg_test",
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "Diagnosis: pod is OOMKilled"},
			},
			"model":   "claude-3-5-sonnet-20241022",
			"usage": map[string]any{
				"input_tokens":  80,
				"output_tokens": 15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &AnthropicProvider{
		apiKey:   "test-key",
		model:    "claude-3-5-sonnet-20241022",
		endpoint: server.URL,
		client:   server.Client(),
	}

	resp, err := p.Complete(t.Context(), provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Why did my pod crash?"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Diagnosis: pod is OOMKilled" {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if resp.PromptTokens != 80 {
		t.Errorf("expected 80 prompt tokens, got %d", resp.PromptTokens)
	}
}

func TestAnthropicProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "authentication_error", "message": "invalid x-api-key"},
		})
	}))
	defer server.Close()

	p := &AnthropicProvider{
		apiKey:   "bad-key",
		model:    "claude-3-5-sonnet-20241022",
		endpoint: server.URL,
		client:   server.Client(),
	}

	_, err := p.Complete(t.Context(), provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})
	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}
