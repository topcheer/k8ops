package gemini

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggai/k8ops/internal/provider"
)

func TestGeminiProvider_Name(t *testing.T) {
	p := &GeminiProvider{}
	if p.Name() != "gemini" {
		t.Errorf("expected 'gemini', got '%s'", p.Name())
	}
}

func TestGeminiProvider_SimpleCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("unexpected key param: %s", r.URL.Query().Get("key"))
		}

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "The deployment has insufficient replicas."},
						},
						"role":  "model",
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     60,
				"candidatesTokenCount": 12,
				"totalTokenCount":      72,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := &GeminiProvider{
		apiKey:   "test-key",
		model:    "gemini-1.5-pro",
		endpoint: server.URL,
		client:   server.Client(),
	}

	resp, err := p.Complete(t.Context(), provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Why is my deployment failing?"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "The deployment has insufficient replicas." {
		t.Errorf("unexpected content: %s", resp.Content)
	}
	if resp.PromptTokens != 60 {
		t.Errorf("expected 60 prompt tokens, got %d", resp.PromptTokens)
	}
}

func TestGeminiProvider_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    403,
				"message": "API key not valid",
			},
		})
	}))
	defer server.Close()

	p := &GeminiProvider{
		apiKey:   "bad-key",
		model:    "gemini-1.5-pro",
		endpoint: server.URL,
		client:   server.Client(),
	}

	_, err := p.Complete(t.Context(), provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})
	if err == nil {
		t.Error("expected error for forbidden request")
	}
}
