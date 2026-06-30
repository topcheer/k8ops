package provider_test

import (
	"testing"

	"github.com/ggai/k8ops/internal/provider"

	// Import provider implementations for side-effect registration
	_ "github.com/ggai/k8ops/internal/provider/anthropic"
	_ "github.com/ggai/k8ops/internal/provider/gemini"
	_ "github.com/ggai/k8ops/internal/provider/openai"
)

func TestNew_OpenAI(t *testing.T) {
	p, err := provider.New(provider.ProviderConfig{
		Type:   "openai",
		APIKey: "test-key",
		Model:  "gpt-4o",
	})
	if err != nil {
		t.Fatalf("failed to create openai provider: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "openai" {
		t.Errorf("expected 'openai', got '%s'", p.Name())
	}
}

func TestNew_Anthropic(t *testing.T) {
	p, err := provider.New(provider.ProviderConfig{
		Type:   "anthropic",
		APIKey: "test-key",
		Model:  "claude-3-5-sonnet-20241022",
	})
	if err != nil {
		t.Fatalf("failed to create anthropic provider: %v", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected 'anthropic', got '%s'", p.Name())
	}
}

func TestNew_Gemini(t *testing.T) {
	p, err := provider.New(provider.ProviderConfig{
		Type:   "gemini",
		APIKey: "test-key",
		Model:  "gemini-1.5-pro",
	})
	if err != nil {
		t.Fatalf("failed to create gemini provider: %v", err)
	}
	if p.Name() != "gemini" {
		t.Errorf("expected 'gemini', got '%s'", p.Name())
	}
}

func TestNew_UnknownProvider(t *testing.T) {
	_, err := provider.New(provider.ProviderConfig{
		Type:   "unknown",
		APIKey: "test",
	})
	if err == nil {
		t.Error("expected error for unknown provider type")
	}
}

func TestNew_EmptyType(t *testing.T) {
	_, err := provider.New(provider.ProviderConfig{
		Type: "",
	})
	if err == nil {
		t.Error("expected error for empty provider type")
	}
}
