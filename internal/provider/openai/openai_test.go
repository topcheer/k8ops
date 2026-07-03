package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggai/k8ops/internal/provider"
)

func TestOpenAIProvider_Name(t *testing.T) {
	p := &OpenAIProvider{}
	if p.Name() != "openai" {
		t.Errorf("expected name 'openai', got '%s'", p.Name())
	}
}

func TestOpenAIProvider_SimpleCompletion(t *testing.T) {
	// Mock OpenAI API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		resp := map[string]any{
			"id":     "test-id",
			"object": "chat.completion",
			"model":  "gpt-4o",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "The pod is crashing due to OOM.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     100,
				"completion_tokens": 20,
				"total_tokens":      120,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:   "test-key",
		model:    "gpt-4o",
		endpoint: server.URL,
		client:   server.Client(),
	}

	resp, err := p.Complete(t.Context(), provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Why is my pod crashing?"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "The pod is crashing due to OOM." {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if resp.PromptTokens != 100 {
		t.Errorf("expected prompt tokens 100, got %d", resp.PromptTokens)
	}
	if resp.CompletionTokens != 20 {
		t.Errorf("expected completion tokens 20, got %d", resp.CompletionTokens)
	}
}

func TestOpenAIProvider_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{
							{
								"id":   "call_abc123",
								"type": "function",
								"function": map[string]any{
									"name":      "k8s_get_resource",
									"arguments": `{"apiVersion":"v1","kind":"Pod","name":"test-pod"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     50,
				"completion_tokens": 30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:   "test-key",
		model:    "gpt-4o",
		endpoint: server.URL,
		client:   server.Client(),
	}

	resp, err := p.Complete(t.Context(), provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Check pod test-pod"},
		},
		Tools: []provider.ToolDefinition{
			{Type: "function", Function: provider.ToolFunctionSchema{
				Name:       "k8s_get_resource",
				Parameters: map[string]any{"type": "object"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "k8s_get_resource" {
		t.Errorf("expected tool name 'k8s_get_resource', got '%s'", tc.Name)
	}
	if tc.ID != "call_abc123" {
		t.Errorf("expected tool ID 'call_abc123', got '%s'", tc.ID)
	}
}

func TestOpenAIProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Invalid API key",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	p := &OpenAIProvider{
		apiKey:   "bad-key",
		model:    "gpt-4o",
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
