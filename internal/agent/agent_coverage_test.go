package agent

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/tools"
)

// --- Mock Provider ---

type mockProvider struct {
	responses []provider.CompletionResponse
	calls     int
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	if len(m.responses) == 0 {
		return &provider.CompletionResponse{Content: "fallback answer"}, nil
	}
	resp := m.responses[m.calls%len(m.responses)]
	m.calls++
	return &resp, nil
}
func (m *mockProvider) StreamComplete(ctx context.Context, req provider.CompletionRequest, onDelta func(string)) (*provider.CompletionResponse, error) {
	resp, err := m.Complete(ctx, req)
	if err == nil && resp != nil {
		onDelta(resp.Content)
	}
	return resp, err
}

func newMockProvider(responses ...provider.CompletionResponse) *mockProvider {
	return &mockProvider{responses: responses}
}

// --- Mock Tool ---

type mockTool struct {
	name    string
	result  *tools.ToolResult
	err     error
	execute func(ctx context.Context, args map[string]any) (*tools.ToolResult, error)
}

func (t *mockTool) Name() string { return t.name }
func (t *mockTool) Description() string { return "mock tool for testing" }
func (t *mockTool) Parameters() map[string]any { return map[string]any{} }
func (t *mockTool) Execute(ctx context.Context, args map[string]any) (*tools.ToolResult, error) {
	if t.execute != nil {
		return t.execute(ctx, args)
	}
	if t.err != nil {
		return nil, t.err
	}
	if t.result != nil {
		return t.result, nil
	}
	return &tools.ToolResult{Success: true, Output: "mock output"}, nil
}

func newMockTool(name string) *mockTool {
	return &mockTool{name: name, result: &tools.ToolResult{Success: true, Output: "tool result"}}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- Tests ---

func TestNew_Defaults(t *testing.T) {
	agent := New(AgentConfig{}, nil)
	if agent.cfg.MaxSteps != 15 {
		t.Errorf("MaxSteps default = %d, want 15", agent.cfg.MaxSteps)
	}
	if agent.cfg.Timeout != 120*time.Second {
		t.Errorf("Timeout default = %v, want 120s", agent.cfg.Timeout)
	}
}

func TestNew_CustomConfig(t *testing.T) {
	agent := New(AgentConfig{
		MaxSteps: 5,
		Timeout:  30 * time.Second,
	}, nil)
	if agent.cfg.MaxSteps != 5 {
		t.Errorf("MaxSteps = %d, want 5", agent.cfg.MaxSteps)
	}
	if agent.cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", agent.cfg.Timeout)
	}
}

func TestRun_ImmediateAnswer(t *testing.T) {
	mp := newMockProvider(provider.CompletionResponse{
		Content:    "Everything is fine",
		ToolCalls:  nil,
	})
	reg := tools.NewRegistry()
	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "check cluster status")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Answer != "Everything is fine" {
		t.Errorf("Answer = %q, want %q", result.Answer, "Everything is fine")
	}
	if len(result.Steps) != 0 {
		t.Errorf("Steps = %d, want 0", len(result.Steps))
	}
}

func TestRun_NilProvider(t *testing.T) {
	reg := tools.NewRegistry()
	agent := New(AgentConfig{
		Registry: reg,
	}, quietLogger())

	_, err := agent.Run(context.Background(), "test")
	if err == nil {
		t.Error("expected error with nil provider")
	}
}

func TestRun_ToolCallSuccess(t *testing.T) {
	// Step 1: LLM requests a tool call
	// Step 2: LLM gives final answer
	mp := newMockProvider(
		provider.CompletionResponse{
			Content: "Let me check the pods",
			ToolCalls: []provider.ToolCall{
				{Name: "mock_tool", ID: "tc1", Arguments: `{"key":"value"}`},
			},
		},
		provider.CompletionResponse{
			Content: "The cluster is healthy",
		},
	)

	reg := tools.NewRegistry()
	reg.Register(newMockTool("mock_tool"))

	agent := New(AgentConfig{
		Provider:    mp,
		Registry:    reg,
		SystemPrompt: "test prompt",
	}, quietLogger())

	result, err := agent.Run(context.Background(), "check pods")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Answer != "The cluster is healthy" {
		t.Errorf("Answer = %q, want 'The cluster is healthy'", result.Answer)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("Steps = %d, want 1", len(result.Steps))
	}
	s := result.Steps[0]
	if s.Action != "mock_tool" {
		t.Errorf("Action = %q, want 'mock_tool'", s.Action)
	}
	if s.Observation != "tool result" {
		t.Errorf("Observation = %q, want 'tool result'", s.Observation)
	}
}

func TestRun_UnknownTool(t *testing.T) {
	mp := newMockProvider(
		provider.CompletionResponse{
			Content: "Calling unknown tool",
			ToolCalls: []provider.ToolCall{
				{Name: "nonexistent", ID: "tc1", Arguments: `{}`},
			},
		},
		provider.CompletionResponse{
			Content: "OK, moving on",
		},
	)

	reg := tools.NewRegistry()
	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("Steps = %d, want 1", len(result.Steps))
	}
	if !strings.Contains(result.Steps[0].Observation, "unknown tool") {
		t.Errorf("Observation should mention 'unknown tool', got: %q", result.Steps[0].Observation)
	}
}

func TestRun_MalformedArguments(t *testing.T) {
	mp := newMockProvider(
		provider.CompletionResponse{
			Content: "Calling tool with bad args",
			ToolCalls: []provider.ToolCall{
				{Name: "mock_tool", ID: "tc1", Arguments: `{invalid json}`},
			},
		},
		provider.CompletionResponse{
			Content: "Done",
		},
	)

	reg := tools.NewRegistry()
	reg.Register(newMockTool("mock_tool"))
	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("Steps = %d, want 1", len(result.Steps))
	}
	if !strings.Contains(result.Steps[0].Observation, "failed to parse arguments") {
		t.Errorf("Observation should mention parse error, got: %q", result.Steps[0].Observation)
	}
}

func TestRun_ToolExecutionError(t *testing.T) {
	mp := newMockProvider(
		provider.CompletionResponse{
			Content: "Calling failing tool",
			ToolCalls: []provider.ToolCall{
				{Name: "fail_tool", ID: "tc1", Arguments: `{}`},
			},
		},
		provider.CompletionResponse{
			Content: "Done",
		},
	)

	reg := tools.NewRegistry()
	reg.Register(&mockTool{
		name: "fail_tool",
		err:  context.DeadlineExceeded,
	})

	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("Steps = %d, want 1", len(result.Steps))
	}
	if !strings.Contains(result.Steps[0].Observation, "Error executing tool") {
		t.Errorf("Observation should mention execution error, got: %q", result.Steps[0].Observation)
	}
}

func TestRun_ToolReturnsError(t *testing.T) {
	mp := newMockProvider(
		provider.CompletionResponse{
			Content: "Calling tool that returns error",
			ToolCalls: []provider.ToolCall{
				{Name: "err_tool", ID: "tc1", Arguments: `{}`},
			},
		},
		provider.CompletionResponse{
			Content: "Done",
		},
	)

	reg := tools.NewRegistry()
	reg.Register(&mockTool{
		name:   "err_tool",
		result: &tools.ToolResult{Success: false, Error: "permission denied", Output: "some debug"},
	})

	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(result.Steps[0].Observation, "Tool returned error") {
		t.Errorf("Observation should mention tool error, got: %q", result.Steps[0].Observation)
	}
}

func TestRun_MaxStepsExceeded(t *testing.T) {
	// Provider always returns tool calls, never a final answer
	mp := newMockProvider(provider.CompletionResponse{
		Content: "Still checking...",
		ToolCalls: []provider.ToolCall{
			{Name: "mock_tool", ID: "tc1", Arguments: `{}`},
		},
	})

	reg := tools.NewRegistry()
	reg.Register(newMockTool("mock_tool"))
	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
		MaxSteps: 3,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(result.Answer, "maximum steps") {
		t.Errorf("Answer should mention max steps, got: %q", result.Answer)
	}
	if len(result.Steps) != 3 {
		t.Errorf("Steps = %d, want 3", len(result.Steps))
	}
}

func TestRun_LongObservationTruncated(t *testing.T) {
	longOutput := strings.Repeat("A", 10000)
	mp := newMockProvider(
		provider.CompletionResponse{
			Content: "Checking",
			ToolCalls: []provider.ToolCall{
				{Name: "verbose_tool", ID: "tc1", Arguments: `{}`},
			},
		},
		provider.CompletionResponse{
			Content: "Done",
		},
	)

	reg := tools.NewRegistry()
	reg.Register(&mockTool{
		name:   "verbose_tool",
		result: &tools.ToolResult{Success: true, Output: longOutput},
	})

	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(result.Steps[0].Observation, "truncated") {
		t.Errorf("Observation should be truncated, got length %d", len(result.Steps[0].Observation))
	}
	if len(result.Steps[0].Observation) > 8100 {
		t.Errorf("Observation too long: %d chars", len(result.Steps[0].Observation))
	}
}

func TestRun_TokenUsageTracking(t *testing.T) {
	mp := newMockProvider(provider.CompletionResponse{
		Content:          "Done",
		PromptTokens:     100,
		CompletionTokens: 50,
	})

	reg := tools.NewRegistry()
	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.TokenUsage.Prompt != 100 {
		t.Errorf("PromptTokens = %d, want 100", result.TokenUsage.Prompt)
	}
	if result.TokenUsage.Completion != 50 {
		t.Errorf("CompletionTokens = %d, want 50", result.TokenUsage.Completion)
	}
	if result.TokenUsage.Total != 150 {
		t.Errorf("TotalTokens = %d, want 150", result.TokenUsage.Total)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	// Provider that blocks until context is cancelled
	mp := &blockingProvider{}
	reg := tools.NewRegistry()
	agent := New(AgentConfig{
		Provider:    mp,
		Registry:    reg,
		Timeout:     100 * time.Millisecond,
	}, quietLogger())

	_, err := agent.Run(context.Background(), "test")
	if err == nil {
		t.Error("expected timeout error")
	}
}

type blockingProvider struct{}

func (b *blockingProvider) Name() string { return "blocking" }
func (b *blockingProvider) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (b *blockingProvider) StreamComplete(ctx context.Context, req provider.CompletionRequest, onDelta func(string)) (*provider.CompletionResponse, error) {
	return b.Complete(ctx, req)
}

func TestRun_MultipleToolCallsInOneStep(t *testing.T) {
	mp := newMockProvider(
		provider.CompletionResponse{
			Content: "Running multiple checks",
			ToolCalls: []provider.ToolCall{
				{Name: "tool_a", ID: "tc1", Arguments: `{}`},
				{Name: "tool_b", ID: "tc2", Arguments: `{}`},
			},
		},
		provider.CompletionResponse{
			Content: "All checks passed",
		},
	)

	reg := tools.NewRegistry()
	reg.Register(newMockTool("tool_a"))
	reg.Register(newMockTool("tool_b"))

	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "check everything")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("Steps = %d, want 2", len(result.Steps))
	}
	if result.Steps[0].Action != "tool_a" {
		t.Errorf("Step 0 action = %q, want 'tool_a'", result.Steps[0].Action)
	}
	if result.Steps[1].Action != "tool_b" {
		t.Errorf("Step 1 action = %q, want 'tool_b'", result.Steps[1].Action)
	}
}

func TestToProviderTools(t *testing.T) {
	agent := New(AgentConfig{}, nil)
	defs := []tools.ToolDef{
		{
			Type: "function",
			Function: tools.ToolFunc{
				Name:        "test_tool",
				Description: "a test tool",
				Parameters:  map[string]any{"type": "object"},
			},
		},
	}

	result := agent.toProviderTools(defs)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].Type != "function" {
		t.Errorf("Type = %q, want 'function'", result[0].Type)
	}
	if result[0].Function.Name != "test_tool" {
		t.Errorf("Name = %q, want 'test_tool'", result[0].Function.Name)
	}
	if result[0].Function.Description != "a test tool" {
		t.Errorf("Description = %q, want 'a test tool'", result[0].Function.Description)
	}
}

func TestToProviderTools_Empty(t *testing.T) {
	agent := New(AgentConfig{}, nil)
	result := agent.toProviderTools(nil)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d items", len(result))
	}
}

func TestDiagnosticSystemPrompt_ContainsKeyElements(t *testing.T) {
	prompt := DiagnosticSystemPrompt()
	checks := []string{
		"k8ops", "SRE", "diagnose", "JSON",
		"findings", "confidence", "suggestedActions",
		"Methodology", "Rules",
	}
	for _, s := range checks {
		if !strings.Contains(prompt, s) {
			t.Errorf("DiagnosticSystemPrompt missing %q", s)
		}
	}
}

func TestOptimizationSystemPrompt_ContainsKeyElements(t *testing.T) {
	prompt := OptimizationSystemPrompt()
	checks := []string{
		"k8ops", "optimization", "suggestions",
		"resource-rightsize", "cost-reduction",
		"estimatedSavings", "confidence", "priority",
	}
	for _, s := range checks {
		if !strings.Contains(prompt, s) {
			t.Errorf("OptimizationSystemPrompt missing %q", s)
		}
	}
}

func TestRun_ResultJSONSerializable(t *testing.T) {
	mp := newMockProvider(provider.CompletionResponse{
		Content: "test answer",
	})
	reg := tools.NewRegistry()
	agent := New(AgentConfig{
		Provider: mp,
		Registry: reg,
	}, quietLogger())

	result, err := agent.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var unmarshaled Result
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if unmarshaled.Answer != "test answer" {
		t.Errorf("Answer after roundtrip = %q", unmarshaled.Answer)
	}
}

func TestStep_Fields(t *testing.T) {
	s := Step{
		Thought:     "I need to check pods",
		Action:      "k8s_list_pods",
		ActionInput: `{"namespace":"default"}`,
		Observation: "3 pods running",
	}
	if s.Thought == "" || s.Action == "" || s.ActionInput == "" || s.Observation == "" {
		t.Error("Step fields should be populated")
	}
}
