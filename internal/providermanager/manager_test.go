package providermanager

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/ggai/k8ops/internal/provider"
	_ "github.com/ggai/k8ops/internal/provider/anthropic"
	_ "github.com/ggai/k8ops/internal/provider/gemini"
	_ "github.com/ggai/k8ops/internal/provider/openai"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestManager_Status_InitialState(t *testing.T) {
	m := New(nil, testLogger())
	status := m.Status()
	if status.Active {
		t.Error("expected inactive state initially")
	}
	if status.Type != "" {
		t.Errorf("expected empty type, got '%s'", status.Type)
	}
}

func TestManager_ReloadFromDirect(t *testing.T) {
	m := New(nil, testLogger())
	err := m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "openai",
		APIKey: "test-key",
		Model:  "gpt-4o",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.Get() == nil {
		t.Fatal("expected non-nil provider")
	}
	if m.Get().Name() != "openai" {
		t.Errorf("expected 'openai', got '%s'", m.Get().Name())
	}

	status := m.Status()
	if !status.Active {
		t.Error("expected active status")
	}
	if status.Type != "openai" {
		t.Errorf("expected type 'openai', got '%s'", status.Type)
	}
	if !status.HasAPIKey {
		t.Error("expected HasAPIKey to be true")
	}
}

func TestManager_ReloadFromDirect_SwapProvider(t *testing.T) {
	m := New(nil, testLogger())

	// Load openai first
	m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "openai",
		APIKey: "key1",
		Model:  "gpt-4o",
	})
	if m.Get().Name() != "openai" {
		t.Errorf("expected openai")
	}

	// Swap to anthropic
	err := m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "anthropic",
		APIKey: "key2",
		Model:  "claude-3-5-sonnet-20241022",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Get().Name() != "anthropic" {
		t.Errorf("expected anthropic after swap, got '%s'", m.Get().Name())
	}
	if m.GetConfig().Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected model updated")
	}
}

func TestManager_ReloadFromDirect_InvalidType(t *testing.T) {
	m := New(nil, testLogger())
	err := m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "unknown",
		APIKey: "key",
		Model:  "model",
	})
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestManager_LastReload(t *testing.T) {
	m := New(nil, testLogger())
	if !m.LastReload().IsZero() {
		t.Error("expected zero time initially")
	}

	m.ReloadFromDirect(context.Background(), provider.ProviderConfig{
		Type:   "openai",
		APIKey: "k",
		Model:  "m",
	})
	if m.LastReload().IsZero() {
		t.Error("expected non-zero time after reload")
	}
}
