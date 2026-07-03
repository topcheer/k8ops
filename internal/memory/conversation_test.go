package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ggai/k8ops/internal/provider"
)

type mockProvider struct {
	response string
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	return &provider.CompletionResponse{
		Content:          m.response,
		PromptTokens:     10,
		CompletionTokens: 5,
	}, nil
}
func (m *mockProvider) StreamComplete(ctx context.Context, req provider.CompletionRequest, onDelta func(string)) (*provider.CompletionResponse, error) {
	return m.Complete(ctx, req)
}

func TestConversation_New(t *testing.T) {
	conv := NewConversation("test", &mockProvider{}, "system prompt", nil)
	if conv.ID() != "test" {
		t.Errorf("expected id 'test', got '%s'", conv.ID())
	}
	msgs := conv.Messages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (system), got %d", len(msgs))
	}
	if msgs[0].Role != provider.RoleSystem {
		t.Error("expected system role")
	}
}

func TestConversation_AddMessages(t *testing.T) {
	conv := NewConversation("test", &mockProvider{}, "sys", nil)
	conv.AddUserMessage("hello")
	conv.AddAssistantMessage(provider.Message{Role: provider.RoleAssistant, Content: "hi there"})
	conv.AddToolResult("call_1", "tool output")

	msgs := conv.Messages()
	if len(msgs) != 4 {
		t.Errorf("expected 4 messages, got %d", len(msgs))
	}
}

func TestConversation_Memory(t *testing.T) {
	conv := NewConversation("test", &mockProvider{}, "sys", nil)
	conv.AddMemory("finding", "pod has OOMKilled")
	conv.AddMemory("decision", "increase memory limit")

	mem := conv.Memory()
	if len(mem) != 2 {
		t.Errorf("expected 2 memory items, got %d", len(mem))
	}
	if mem[0].Type != "finding" {
		t.Errorf("expected type 'finding', got '%s'", mem[0].Type)
	}
}

func TestConversation_Stats(t *testing.T) {
	conv := NewConversation("stats-test", &mockProvider{}, "sys", nil)
	conv.AddUserMessage("test message")

	stats := conv.Stats()
	if stats.ID != "stats-test" {
		t.Errorf("expected id 'stats-test', got '%s'", stats.ID)
	}
	if stats.MessageCount != 2 {
		t.Errorf("expected 2 messages, got %d", stats.MessageCount)
	}
}

func TestConversation_Compress(t *testing.T) {
	// Use a real mock HTTP server to simulate provider
	compressCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compressCalled = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "Summary of conversation"}}},
		})
	}))
	defer server.Close()

	// Create a conversation with low threshold to trigger compression
	conv := NewConversation("test", &mockProvider{response: "Summary of conversation"}, "sys", slog.Default())
	conv.maxTokens = 1  // very low threshold
	conv.keepRecent = 2 // keep only 2 messages

	// Add enough messages to trigger compression
	for i := 0; i < 6; i++ {
		conv.AddUserMessage(fmt.Sprintf("message %d with some content", i))
		conv.AddAssistantMessage(provider.Message{Role: provider.RoleAssistant, Content: fmt.Sprintf("response %d", i)})
	}

	compressed, err := conv.MaybeCompress(context.Background())
	if err != nil {
		t.Fatalf("compression failed: %v", err)
	}
	if !compressed {
		t.Error("expected compression to occur")
	}
	_ = compressCalled

	// Check memory was added
	mem := conv.Memory()
	if len(mem) == 0 {
		t.Error("expected at least 1 memory item after compression")
	}

	// Check message count decreased
	msgs := conv.Messages()
	if len(msgs) > 5 {
		t.Errorf("expected message count to decrease, got %d", len(msgs))
	}
}

func TestConversation_CompressNotNeeded(t *testing.T) {
	conv := NewConversation("test", &mockProvider{}, "sys", nil)
	conv.maxTokens = 100000 // high threshold
	conv.AddUserMessage("test")

	compressed, err := conv.MaybeCompress(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compressed {
		t.Error("expected compression to NOT occur")
	}
}

func TestEstimateTokens(t *testing.T) {
	if estimateTokens("hello world") != 2 { // 11 chars / 4 = 2
		t.Error("expected 2 tokens")
	}
}
