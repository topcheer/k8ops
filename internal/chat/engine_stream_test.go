package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/ggai/k8ops/internal/memory"
	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/resilience"
	"github.com/ggai/k8ops/internal/tools"
)

// ---------------------------------------------------------------------------
// RunStream integration tests with a scripted mock provider.
// Covers: multi-step tool calls, SSE event format, context cancellation,
//         concurrency safety, error recovery.
// ---------------------------------------------------------------------------

// scriptedProvider returns pre-configured responses in sequence.
type scriptedProvider struct {
	mu        sync.Mutex
	responses []*provider.CompletionResponse
	errs      []error
	calls     int
	deltas    []string // deltas to emit during StreamComplete
}

func (p *scriptedProvider) Name() string { return "scripted" }

func (p *scriptedProvider) Complete(_ context.Context, _ provider.CompletionRequest) (*provider.CompletionResponse, error) {
	return &provider.CompletionResponse{Content: "ok"}, nil
}

func (p *scriptedProvider) StreamComplete(_ context.Context, _ provider.CompletionRequest, onDelta func(string)) (*provider.CompletionResponse, error) {
	p.mu.Lock()
	idx := p.calls
	p.calls++
	p.mu.Unlock()

	for _, d := range p.deltas {
		onDelta(d)
	}

	if idx < len(p.errs) && p.errs[idx] != nil {
		return nil, p.errs[idx]
	}
	if idx < len(p.responses) {
		return p.responses[idx], nil
	}
	return &provider.CompletionResponse{Content: "fallback"}, nil
}

// mockTool satisfies tools.Tool for testing.
type mockTool struct {
	name    string
	output  string
	execErr error
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "mock tool for testing" }
func (t *mockTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *mockTool) Execute(_ context.Context, _ map[string]any) (*tools.ToolResult, error) {
	if t.execErr != nil {
		return nil, t.execErr
	}
	return &tools.ToolResult{Success: true, Output: t.output}, nil
}

// newStreamTestEngine creates an engine with a scripted provider and registry,
// using a minimal retry config (1 attempt, no delay) for fast tests.
func newStreamTestEngine(p provider.Provider, reg *tools.Registry) *Engine {
	return &Engine{
		conversations:   make(map[string]*memory.Conversation),
		provider:        func() provider.Provider { return p },
		systemPrompt:    "test",
		maxSteps:        10,
		timeout:         10 * time.Second,
		log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		retryCfg:        resilience.RetryConfig{MaxAttempts: 1, InitialDelay: 1 * time.Millisecond, MaxDelay: 1 * time.Millisecond, Multiplier: 1},
		circuitBreaker:  resilience.NewCircuitBreaker(100, 60*time.Second),
		registry:        reg,
	}
}

// === 1. Multi-step tool call chain ===

func TestRunStream_MultiStepToolChain(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "get_pods", output: "pod1\npod2"})
	reg.Register(&mockTool{name: "get_nodes", output: "node1\nnode2"})

	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{
			// Step 1: LLM calls get_pods
			{
				Content:   "Let me check the pods.",
				ToolCalls: []provider.ToolCall{{ID: "c1", Name: "get_pods", Arguments: "{}"}},
			},
			// Step 2: LLM calls get_nodes
			{
				Content:   "Now checking nodes.",
				ToolCalls: []provider.ToolCall{{ID: "c2", Name: "get_nodes", Arguments: "{}"}},
			},
			// Step 3: final answer (no tool calls)
			{Content: "Found 2 pods on 2 nodes.", PromptTokens: 100, CompletionTokens: 50},
		},
	}

	e := newStreamTestEngine(p, reg)
	var events []StreamEvent
	var mu sync.Mutex

	err := e.RunStream(context.Background(), "conv-multi", "check cluster",
		func(evt StreamEvent) {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Count event types
	toolCalls, toolResults, answers, dones := 0, 0, 0, 0
	for _, e := range events {
		switch e.Type {
		case EventToolCall:
			toolCalls++
		case EventToolResult:
			toolResults++
		case EventAnswer:
			answers++
		case EventDone:
			dones++
		}
	}

	if toolCalls != 2 {
		t.Errorf("tool_call count = %d, want 2", toolCalls)
	}
	if toolResults != 2 {
		t.Errorf("tool_result count = %d, want 2", toolResults)
	}
	if answers != 1 {
		t.Errorf("answer count = %d, want 1", answers)
	}
	if dones != 1 {
		t.Errorf("done count = %d, want 1", dones)
	}

	// Verify final answer
	var ans *StreamEvent
	for i := range events {
		if events[i].Type == EventAnswer {
			ans = &events[i]
			break
		}
	}
	if ans == nil {
		t.Fatal("no answer event")
	}
	data, ok := ans.Data.(AnswerData)
	if !ok {
		t.Fatalf("answer data type = %T, want AnswerData", ans.Data)
	}
	if data.Content != "Found 2 pods on 2 nodes." {
		t.Errorf("answer content = %q", data.Content)
	}
	if data.Steps != 3 {
		t.Errorf("answer steps = %d, want 3", data.Steps)
	}
	if data.TotalTokens != 150 {
		t.Errorf("total tokens = %d, want 150", data.TotalTokens)
	}
}

// === 2. SSE event format validation ===

func TestRunStream_SSEEventFormat(t *testing.T) {
	reg := tools.NewRegistry()
	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{
			{Content: "Hello", PromptTokens: 10, CompletionTokens: 5},
		},
		deltas: []string{"Hel", "lo"},
	}

	e := newStreamTestEngine(p, reg)
	var events []StreamEvent

	err := e.RunStream(context.Background(), "conv-sse", "hi",
		func(evt StreamEvent) { events = append(events, evt) },
	)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("no events received")
	}

	for i, evt := range events {
		// Type must be non-empty
		if evt.Type == "" {
			t.Errorf("event[%d]: empty type", i)
		}
		// Timestamp must be valid RFC3339
		if evt.Timestamp == "" {
			t.Errorf("event[%d]: empty timestamp", i)
		} else if _, err := time.Parse(time.RFC3339, evt.Timestamp); err != nil {
			t.Errorf("event[%d]: invalid timestamp %q: %v", i, evt.Timestamp, err)
		}
		// Data must be JSON-serializable
		if _, err := json.Marshal(evt.Data); err != nil {
			t.Errorf("event[%d]: data not JSON-serializable: %v", i, err)
		}
	}

	// Last event should be done
	last := events[len(events)-1]
	if last.Type != EventDone {
		t.Errorf("last event type = %q, want 'done'", last.Type)
	}

	// Must have an answer event
	var hasAnswer bool
	for _, evt := range events {
		if evt.Type == EventAnswer {
			hasAnswer = true
			ad, ok := evt.Data.(AnswerData)
			if !ok {
				t.Error("answer data should be AnswerData")
			} else if ad.Content != "Hello" {
				t.Errorf("answer content = %q, want 'Hello'", ad.Content)
			}
		}
	}
	if !hasAnswer {
		t.Error("missing answer event")
	}
}

// === 3. Context cancellation ===

func TestRunStream_ContextCancel(t *testing.T) {
	reg := tools.NewRegistry()

	// Provider blocks until context done
	blocking := &blockingP{}
	e := newStreamTestEngine(blocking, reg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := e.RunStream(ctx, "conv-cancel", "test", func(evt StreamEvent) {})
	if err == nil {
		t.Error("expected error on cancelled context")
	}
}

// blockingP blocks StreamComplete until ctx is done.
type blockingP struct{}

func (p *blockingP) Name() string { return "blocking" }
func (p *blockingP) Complete(_ context.Context, _ provider.CompletionRequest) (*provider.CompletionResponse, error) {
	return &provider.CompletionResponse{Content: ""}, nil
}
func (p *blockingP) StreamComplete(ctx context.Context, _ provider.CompletionRequest, _ func(string)) (*provider.CompletionResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// === 4. Concurrency safety ===

func TestRunStream_ConcurrentSafety(t *testing.T) {
	reg := tools.NewRegistry()
	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{{Content: "ok", PromptTokens: 5, CompletionTokens: 3}},
	}
	e := newStreamTestEngine(p, reg)

	const N = 10
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make([]error, N)

	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = e.RunStream(
				context.Background(),
				fmt.Sprintf("conv-concurrent-%d", idx),
				"test",
				func(evt StreamEvent) {},
			)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}
	// Verify all conversations created
	stats := e.ConversationStats()
	if len(stats) != N {
		t.Errorf("conversation count = %d, want %d", len(stats), N)
	}
}

func TestRunStream_ConcurrentGetDelete(t *testing.T) {
	e := newTestEngine()

	// Pre-create
	for i := 0; i < 20; i++ {
		e.GetOrCreateConversation(fmt.Sprintf("conv-%d", i))
	}

	var wg sync.WaitGroup
	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			e.GetOrCreateConversation(fmt.Sprintf("conv-%d", idx%20))
		}(i)
	}
	// Deleters
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			e.DeleteConversation(fmt.Sprintf("conv-%d", idx))
		}(i)
	}
	// Stats
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = e.ConversationStats()
		}()
	}
	wg.Wait()
}

// === 5. Error recovery ===

func TestRunStream_NonRetryableError(t *testing.T) {
	reg := tools.NewRegistry()
	p := &scriptedProvider{
		errs: []error{errors.New("HTTP 400 Bad Request")},
	}
	e := newStreamTestEngine(p, reg)

	var events []StreamEvent
	var mu sync.Mutex

	err := e.RunStream(context.Background(), "conv-err", "test",
		func(evt StreamEvent) {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
		},
	)
	if err == nil {
		t.Error("expected error from failing provider")
	}

	mu.Lock()
	defer mu.Unlock()

	hasError := false
	for _, evt := range events {
		if evt.Type == EventError {
			hasError = true
			msg, ok := evt.Data.(map[string]string)
			if !ok {
				t.Error("error event data should be map[string]string")
			} else if msg["message"] == "" {
				t.Error("error message should not be empty")
			}
		}
	}
	if !hasError {
		t.Error("expected error event in stream")
	}
}

func TestRunStream_NoProvider(t *testing.T) {
	e := &Engine{
		conversations:   make(map[string]*memory.Conversation),
		provider:        func() provider.Provider { return nil },
		systemPrompt:    "test",
		maxSteps:        5,
		timeout:         10 * time.Second,
		log:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		retryCfg:        resilience.RetryConfig{MaxAttempts: 1, InitialDelay: 1 * time.Millisecond, MaxDelay: 1 * time.Millisecond, Multiplier: 1},
		circuitBreaker:  resilience.NewCircuitBreaker(100, 60*time.Second),
		registry:        tools.NewRegistry(),
	}

	var events []StreamEvent
	err := e.RunStream(context.Background(), "conv-noprovider", "test",
		func(evt StreamEvent) { events = append(events, evt) },
	)
	if err == nil {
		t.Fatal("expected error when no provider")
	}
	if len(events) == 0 {
		t.Fatal("expected at least an error event")
	}
	if events[0].Type != EventError {
		t.Errorf("first event type = %q, want 'error'", events[0].Type)
	}
}

func TestRunStream_UnknownTool(t *testing.T) {
	reg := tools.NewRegistry() // empty registry

	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{
			{
				Content:   "Using unknown tool",
				ToolCalls: []provider.ToolCall{{ID: "c1", Name: "nonexistent", Arguments: "{}"}},
			},
			{Content: "Tool not found.", PromptTokens: 5, CompletionTokens: 3},
		},
	}
	e := newStreamTestEngine(p, reg)

	var events []StreamEvent
	var mu sync.Mutex
	err := e.RunStream(context.Background(), "conv-unknown", "test",
		func(evt StreamEvent) {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, evt := range events {
		if evt.Type == EventToolResult {
			trd, ok := evt.Data.(ToolResultData)
			if !ok {
				t.Fatal("tool_result data type mismatch")
			}
			if trd.Success {
				t.Error("unknown tool should have failed result")
			}
			if trd.Error == "" {
				t.Error("failed tool should have error message")
			}
			return
		}
	}
	t.Fatal("expected tool_result event")
}

func TestRunStream_MalformedToolArgs(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "valid", output: "ok"})

	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{
			{
				Content:   "Bad args",
				ToolCalls: []provider.ToolCall{{ID: "c1", Name: "valid", Arguments: "{bad json}"}},
			},
			{Content: "Recovered.", PromptTokens: 5, CompletionTokens: 3},
		},
	}
	e := newStreamTestEngine(p, reg)

	var events []StreamEvent
	var mu sync.Mutex
	err := e.RunStream(context.Background(), "conv-badargs", "test",
		func(evt StreamEvent) {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, evt := range events {
		if evt.Type == EventToolResult {
			trd, ok := evt.Data.(ToolResultData)
			if !ok {
				t.Fatal("tool_result data type mismatch")
			}
			if trd.Success {
				t.Error("malformed args should produce failed result")
			}
			if trd.Error == "" {
				t.Error("should have error message for parse failure")
			}
			return
		}
	}
	t.Fatal("expected tool_result event with parse error")
}

func TestRunStream_ToolExecutionError(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "fail", execErr: errors.New("connection refused")})

	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{
			{
				Content:   "Calling failing tool",
				ToolCalls: []provider.ToolCall{{ID: "c1", Name: "fail", Arguments: "{}"}},
			},
			{Content: "Tool failed.", PromptTokens: 5, CompletionTokens: 3},
		},
	}
	e := newStreamTestEngine(p, reg)

	var events []StreamEvent
	var mu sync.Mutex
	err := e.RunStream(context.Background(), "conv-toolerr", "test",
		func(evt StreamEvent) {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, evt := range events {
		if evt.Type == EventToolResult {
			trd, ok := evt.Data.(ToolResultData)
			if !ok {
				t.Fatal("tool_result data type mismatch")
			}
			if trd.Success {
				t.Error("execution error should produce failed result")
			}
			if trd.Error == "" {
				t.Error("should have error message")
			}
			return
		}
	}
	t.Fatal("expected tool_result event with execution error")
}

func TestRunStream_MaxSteps(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&mockTool{name: "loop", output: "looping"})

	// Always returns tool call, never final answer
	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{
			{
				Content:   "Looping",
				ToolCalls: []provider.ToolCall{{ID: "c1", Name: "loop", Arguments: "{}"}},
			},
		},
	}
	e := newStreamTestEngine(p, reg)
	e.maxSteps = 2 // low limit

	var events []StreamEvent
	var mu sync.Mutex
	err := e.RunStream(context.Background(), "conv-maxsteps", "test",
		func(evt StreamEvent) {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	var ans *StreamEvent
	for i := range events {
		if events[i].Type == EventAnswer {
			ans = &events[i]
			break
		}
	}
	if ans == nil {
		t.Fatal("expected answer event at max steps")
	}
	data := ans.Data.(AnswerData)
	if data.Steps != 2 {
		t.Errorf("steps = %d, want 2", data.Steps)
	}
}

func TestRunStream_StreamingDeltas(t *testing.T) {
	reg := tools.NewRegistry()
	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{{Content: "final answer", PromptTokens: 5, CompletionTokens: 3}},
		deltas:    []string{"Hello", " ", "world"},
	}
	e := newStreamTestEngine(p, reg)

	deltaCount := 0
	var mu sync.Mutex
	err := e.RunStream(context.Background(), "conv-deltas", "hi",
		func(evt StreamEvent) {
			mu.Lock()
			if evt.Type == EventThinkingDelta {
				deltaCount++
			}
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if deltaCount != 3 {
		t.Errorf("delta count = %d, want 3", deltaCount)
	}
}

func TestRunStreamWithRegistry_CustomRegistry(t *testing.T) {
	defaultReg := tools.NewRegistry()
	defaultReg.Register(&mockTool{name: "default_tool", output: "default"})

	customReg := tools.NewRegistry()
	customReg.Register(&mockTool{name: "custom_tool", output: "custom"})

	p := &scriptedProvider{
		responses: []*provider.CompletionResponse{
			{
				Content:   "Using custom tool",
				ToolCalls: []provider.ToolCall{{ID: "c1", Name: "custom_tool", Arguments: "{}"}},
			},
			{Content: "Done.", PromptTokens: 5, CompletionTokens: 3},
		},
	}
	e := newStreamTestEngine(p, defaultReg)

	var events []StreamEvent
	var mu sync.Mutex
	err := e.RunStreamWithRegistry(context.Background(), "conv-customreg", "test",
		func(evt StreamEvent) {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
		},
		customReg,
	)
	if err != nil {
		t.Fatalf("RunStreamWithRegistry failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, evt := range events {
		if evt.Type == EventToolResult {
			trd := evt.Data.(ToolResultData)
			if !trd.Success {
				t.Error("custom tool should succeed")
			}
			if trd.Output != "custom" {
				t.Errorf("output = %q, want 'custom'", trd.Output)
			}
			return
		}
	}
	t.Fatal("expected tool_result from custom registry")
}
