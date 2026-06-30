// Package chat provides the interactive chat engine that connects the
// dashboard UI to the agent with tool execution and streaming responses.
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ggai/k8ops/internal/audit"
	"github.com/ggai/k8ops/internal/memory"
	"github.com/ggai/k8ops/internal/metrics"
	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/resilience"
	"github.com/ggai/k8ops/internal/tools"
)

// EventType for SSE stream events.
type EventType string

const (
	EventThinking     EventType = "thinking"
	EventThinkingDelta EventType = "thinking_delta"
	EventAnswerDelta  EventType = "answer_delta"
	EventToolCall     EventType = "tool_call"
	EventToolResult   EventType = "tool_result"
	EventAnswer       EventType = "answer"
	EventError        EventType = "error"
	EventDone         EventType = "done"
	EventMemory       EventType = "memory"
	EventPing         EventType = "ping"
)

// StreamEvent is a single SSE event sent to the client.
type StreamEvent struct {
	Type      EventType   `json:"type"`
	Data      any         `json:"data"`
	Timestamp string      `json:"timestamp"`
}

// ToolCallData for tool_call events.
type ToolCallData struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Step      int    `json:"step"`
}

// ToolResultData for tool_result events.
type ToolResultData struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
	Step    int    `json:"step"`
}

// AnswerData for answer events.
type AnswerData struct {
	Content      string `json:"content"`
	PromptTokens int    `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens  int    `json:"totalTokens"`
	Steps        int    `json:"steps"`
}

const (
	// cleanupInterval is how often the background goroutine scans for
	// idle conversations. Default: 5 minutes.
	cleanupInterval = 5 * time.Minute
	// conversationTTL is the maximum idle time before a conversation is
	// evicted. Default: 30 minutes.
	conversationTTL = 30 * time.Minute
	// maxConversations caps the number of concurrent conversations. When
	// exceeded, oldest-first eviction kicks in immediately (not waiting for TTL).
	maxConversations = 1000
)

// Engine runs interactive chat sessions with agent capabilities.
type Engine struct {
	provider     func() provider.Provider // hot-swappable
	registry     *tools.Registry
	auditLog     *audit.Logger
	log          *slog.Logger
	maxSteps     int
	timeout      time.Duration
	systemPrompt string

	// Resilience
	retryCfg       resilience.RetryConfig
	circuitBreaker *resilience.CircuitBreaker

	// Conversation store
	mu            sync.RWMutex
	conversations map[string]*memory.Conversation

	// TTL cleanup
	cleanupStop chan struct{}  // closed to signal goroutine to exit (nil = not running)
	cleanupWg   sync.WaitGroup // tracks goroutine liveness
}

// NewEngine creates a chat engine.
func NewEngine(
	providerFn func() provider.Provider,
	registry *tools.Registry,
	auditLog *audit.Logger,
	systemPrompt string,
	log *slog.Logger,
) *Engine {
	return &Engine{
		provider:       providerFn,
		registry:       registry,
		auditLog:       auditLog,
		systemPrompt:   systemPrompt,
		log:            log,
		maxSteps:       15,
		timeout:        180 * time.Second,
		retryCfg:       resilience.RetryConfig{MaxAttempts: 5, InitialDelay: 1 * time.Second, MaxDelay: 30 * time.Second, Multiplier: 2.0},
		circuitBreaker: resilience.NewCircuitBreaker(5, 60*time.Second),
		conversations:  make(map[string]*memory.Conversation),
	}
}

// StartCleanup launches a background goroutine that periodically evicts idle
// conversations (default: every 5 minutes, conversations inactive for > 30
// minutes are removed). The goroutine also enforces a hard cap of 1000
// concurrent conversations, evicting the oldest ones when the limit is exceeded.
// Call StopCleanup to stop the goroutine before the engine is discarded.
func (e *Engine) StartCleanup() {
	e.mu.Lock()
	if e.cleanupStop != nil {
		e.mu.Unlock()
		return // already running
	}
	e.cleanupStop = make(chan struct{})
	stop := e.cleanupStop
	e.cleanupWg.Add(1)
	e.mu.Unlock()

	go e.cleanupLoop(stop)
}

// StopCleanup signals the background cleanup goroutine to exit and waits for
// it to drain. Safe to call even if StartCleanup was never invoked.
func (e *Engine) StopCleanup() {
	e.mu.Lock()
	stop := e.cleanupStop
	e.cleanupStop = nil
	e.mu.Unlock()
	if stop != nil {
		close(stop) // signal goroutine to exit
		e.cleanupWg.Wait()
	}
}

// cleanupLoop periodically evicts idle conversations until stop is closed.
func (e *Engine) cleanupLoop(stop <-chan struct{}) {
	defer e.cleanupWg.Done()
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			evicted := e.evictIdle()
			if evicted > 0 && e.log != nil {
				e.log.Info("conversation cleanup: evicted idle conversations",
					"count", evicted)
			}
		case <-stop:
			return
		}
	}
}

// evictIdle removes conversations whose LastActivity is older than conversationTTL
// and, if the map still exceeds maxConversations, the oldest remaining ones.
// Returns the number of conversations removed.
func (e *Engine) evictIdle() int {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	var evicted int

	// Phase 1: remove conversations idle for > conversationTTL
	for id, conv := range e.conversations {
		if now.Sub(conv.LastActivity()) > conversationTTL {
			delete(e.conversations, id)
			evicted++
		}
	}

	// Phase 2: if still over the hard cap, evict oldest-first
	if len(e.conversations) > maxConversations {
		type entry struct {
			id       string
			activity time.Time
		}
		entries := make([]entry, 0, len(e.conversations))
		for id, conv := range e.conversations {
			entries = append(entries, entry{id, conv.LastActivity()})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].activity.Before(entries[j].activity)
		})
		toRemove := len(e.conversations) - maxConversations
		for i := 0; i < toRemove && i < len(entries); i++ {
			delete(e.conversations, entries[i].id)
			evicted++
		}
	}

	if evicted > 0 {
		metrics.ConversationCount.Set(float64(len(e.conversations)))
	}

	return evicted
}

// GetOrCreateConversation returns an existing conversation or creates a new one.
func (e *Engine) GetOrCreateConversation(id string) *memory.Conversation {
	e.mu.Lock()
	defer e.mu.Unlock()

	if conv, ok := e.conversations[id]; ok {
		return conv
	}

	p := e.provider()
	if p == nil {
		return nil
	}

	// Enforce the hard cap before adding a new conversation
	if len(e.conversations) >= maxConversations {
		e.evictOldestLocked()
	}

	conv := memory.NewConversation(id, p, e.systemPrompt, e.log)
	e.conversations[id] = conv
	metrics.ConversationCount.Set(float64(len(e.conversations)))
	return conv
}

// evictOldestLocked removes the single oldest conversation. Caller must hold e.mu.
func (e *Engine) evictOldestLocked() {
	var oldestID string
	var oldestTime time.Time
	for id, conv := range e.conversations {
		act := conv.LastActivity()
		if oldestID == "" || act.Before(oldestTime) {
			oldestID = id
			oldestTime = act
		}
	}
	if oldestID != "" {
		delete(e.conversations, oldestID)
		metrics.ConversationCount.Set(float64(len(e.conversations)))
		if e.log != nil {
			e.log.Debug("conversation cleanup: evicted oldest to enforce cap", "id", oldestID)
		}
	}
}

// ConversationStats returns stats for all conversations.
func (e *Engine) ConversationStats() []memory.Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]memory.Stats, 0, len(e.conversations))
	for _, c := range e.conversations {
		result = append(result, c.Stats())
	}
	return result
}

// DeleteConversation removes a conversation.
func (e *Engine) DeleteConversation(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.conversations, id)
}

// RunStream executes an agent run and streams events through the callback.
// Uses the engine's default tool registry (no per-user impersonation).
func (e *Engine) RunStream(
	ctx context.Context,
	conversationID string,
	userMessage string,
	onEvent func(StreamEvent),
) error {
	return e.RunStreamWithRegistry(ctx, conversationID, userMessage, onEvent, nil)
}

// RunStreamWithRegistry executes an agent run with a per-request tool registry.
// If reg is nil, falls back to the engine's default registry.
// This allows per-user impersonation: each request gets tools backed by
// impersonated K8s clients that respect the user's RBAC permissions.
func (e *Engine) RunStreamWithRegistry(
	ctx context.Context,
	conversationID string,
	userMessage string,
	onEvent func(StreamEvent),
	reg *tools.Registry,
) error {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	conv := e.GetOrCreateConversation(conversationID)
	if conv == nil {
		onEvent(errorEvent("No provider configured. Please set up LLM config first."))
		return fmt.Errorf("no provider available")
	}

	// Add user message to conversation
	conv.AddUserMessage(userMessage)

	// Maybe compress context
	if compressed, err := conv.MaybeCompress(ctx); err != nil {
		e.log.Warn("compression failed", "error", err)
	} else if compressed {
		onEvent(StreamEvent{
			Type:      EventMemory,
			Data:      map[string]any{"action": "context_compressed", "memory": conv.Memory()},
			Timestamp: time.Now().Format(time.RFC3339),
		})
	}

	p := e.provider()
	if p == nil {
		onEvent(errorEvent("Provider became unavailable during chat"))
		return fmt.Errorf("provider unavailable")
	}

	// Use per-request registry if provided, otherwise fall back to default
	effectiveRegistry := e.registry
	if reg != nil {
		effectiveRegistry = reg
	}

	toolDefs := effectiveRegistry.Definitions()

	var totalPrompt, totalCompletion int
	stepCount := 0

	for step := 0; step < e.maxSteps; step++ {
		stepCount = step + 1
		e.log.Debug("chat agent step", "step", stepCount, "conversation", conversationID)

		// Get current conversation messages (may include compressed summary)
		msgs := conv.Messages()

		req := provider.CompletionRequest{
			Messages:    msgs,
			Tools:       toProviderTools(toolDefs),
			Temperature: 0.1,
			MaxTokens:   4096,
		}

		// Call LLM with streaming + retry + circuit breaker
		if !e.circuitBreaker.Allow() {
			onEvent(errorEvent("Circuit breaker is open: too many failures, please wait a moment."))
			return fmt.Errorf("circuit breaker open")
		}

		var resp *provider.CompletionResponse
		var llmErr error
		llmStart := time.Now()

		retryErr := resilience.Retry(ctx, e.retryCfg, e.log, func() error {
			r, err := p.StreamComplete(ctx, req, func(delta string) {
				if delta == "" {
					return
				}
				// Stream as thinking_delta; will be corrected to answer_delta
				// if no tool calls appear in the final response.
				onEvent(StreamEvent{
					Type:      EventThinkingDelta,
					Data:      map[string]any{"delta": delta, "step": stepCount},
					Timestamp: time.Now().Format(time.RFC3339),
				})
			})
			if err != nil {
				if isRetryableError(err) {
					e.log.Warn("LLM call failed, retrying",
						"error", err, "maxAttempts", e.retryCfg.MaxAttempts)
					return err
				}
				llmErr = err
				return nil
			}
			resp = r
			return nil
		})

		if retryErr != nil {
			llmErr = retryErr
		}
		if llmErr != nil {
			e.circuitBreaker.RecordFailure()
			metrics.LLMCallDuration.WithLabelValues("unknown", "unknown", "failure").Observe(time.Since(llmStart).Seconds())
			onEvent(errorEvent(fmt.Sprintf("LLM call failed (after %d retries): %v", e.retryCfg.MaxAttempts, llmErr)))
			if e.auditLog != nil {
				e.auditLog.Log(ctx, audit.Event{
					Type:     audit.EventTypeLLMCall,
					Severity: audit.SeverityCritical,
					Action:   "chat_completion",
					Actor:    "dashboard-user",
					Success:  false,
					Error:    llmErr.Error(),
				})
			}
			return llmErr
		}
		e.circuitBreaker.RecordSuccess()
		llmDuration := time.Since(llmStart).Seconds()
		metrics.LLMCallDuration.WithLabelValues("unknown", "unknown", "success").Observe(llmDuration)

		totalPrompt += resp.PromptTokens
		totalCompletion += resp.CompletionTokens

		if e.auditLog != nil {
			e.auditLog.Log(ctx, audit.Event{
				Type:     audit.EventTypeLLMCall,
				Action:   "chat_completion",
				Actor:    "dashboard-user",
				Success:  true,
				Severity: audit.SeverityInfo,
				Detail: map[string]any{
					"step":             stepCount,
					"promptTokens":     resp.PromptTokens,
					"completionTokens": resp.CompletionTokens,
					"hasToolCalls":     len(resp.ToolCalls) > 0,
					"conversationId":   conversationID,
				},
			})
		}

		// If no tool calls, the streamed text IS the final answer
		if len(resp.ToolCalls) == 0 {
			conv.AddAssistantMessage(provider.Message{
				Role: provider.RoleAssistant, Content: resp.Content,
			})

			onEvent(StreamEvent{
				Type: EventAnswer,
				Data: AnswerData{
					Content:          resp.Content,
					PromptTokens:     totalPrompt,
					CompletionTokens: totalCompletion,
					TotalTokens:      totalPrompt + totalCompletion,
					Steps:            stepCount,
				},
				Timestamp: time.Now().Format(time.RFC3339),
			})
			onEvent(StreamEvent{Type: EventDone, Timestamp: time.Now().Format(time.RFC3339)})
			return nil
		}

		// Has tool calls: the streamed text is thinking/reasoning
		// Emit a thinking "done" signal so frontend can collapse it
		if resp.Content != "" {
			onEvent(StreamEvent{
				Type:      EventThinking,
				Data:      map[string]any{"content": resp.Content, "step": stepCount, "done": true},
				Timestamp: time.Now().Format(time.RFC3339),
			})
		}

		// Add assistant message with tool calls to conversation
		conv.AddAssistantMessage(provider.Message{
			Role: provider.RoleAssistant,
			Content: resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call
		for _, tc := range resp.ToolCalls {
			onEvent(StreamEvent{
				Type: EventToolCall,
				Data: ToolCallData{
					Name: tc.Name, Arguments: tc.Arguments, Step: stepCount,
				},
				Timestamp: time.Now().Format(time.RFC3339),
			})

			var args map[string]any
			resultOutput := ""
			resultErr := ""
			success := true

			if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
				resultErr = fmt.Sprintf("Failed to parse arguments: %v", err)
				success = false
			} else {
				tool, ok := effectiveRegistry.Get(tc.Name)
				if !ok {
					resultErr = fmt.Sprintf("Unknown tool: %s", tc.Name)
					success = false
				} else {
					start := time.Now()
					toolResult, err := tool.Execute(ctx, args)
					duration := time.Since(start)

					metrics.ToolCallDuration.WithLabelValues(tc.Name, fmt.Sprintf("%t", err == nil)).Observe(duration.Seconds())
					metrics.ToolExecTotal.WithLabelValues(tc.Name, fmt.Sprintf("%t", err == nil && toolResult != nil && toolResult.Success)).Inc()

					if e.auditLog != nil {
						e.auditLog.Log(ctx, audit.Event{
							Type:     audit.EventTypeToolCall,
							Action:   tc.Name,
							Actor:    "dashboard-user",
							Success:  toolResult != nil && toolResult.Success && err == nil,
							Severity: audit.SeverityInfo,
							Detail:   map[string]any{"args": args, "duration": duration.String()},
							Duration: duration.String(),
						})
					}

					if err != nil {
						resultErr = err.Error()
						success = false
					} else if !toolResult.Success {
						resultErr = toolResult.Error
						resultOutput = toolResult.Output
						success = false
					} else {
						resultOutput = toolResult.Output
						if len(resultOutput) > 8000 {
							resultOutput = resultOutput[:8000] + "\n... (truncated)"
						}
					}
				}
			}

			// Emit tool result event
			onEvent(StreamEvent{
				Type: EventToolResult,
				Data: ToolResultData{
					Name:    tc.Name,
					Success: success,
					Output:  truncate(resultOutput, 2000),
					Error:   resultErr,
					Step:    stepCount,
				},
				Timestamp: time.Now().Format(time.RFC3339),
			})

			// Add to conversation
			toolContent := resultOutput
			if resultErr != "" {
				toolContent = fmt.Sprintf("Error: %s\n%s", resultErr, resultOutput)
			}
			conv.AddToolResult(tc.ID, toolContent)
		}
	}

	// Max steps reached
	finalAnswer := "I've reached the maximum number of analysis steps. Based on my investigation so far, I need more specific information to continue. Please provide additional context or narrow the scope."
	conv.AddAssistantMessage(provider.Message{
		Role: provider.RoleAssistant, Content: finalAnswer,
	})

	onEvent(StreamEvent{
		Type: EventAnswer,
		Data: AnswerData{
			Content:      finalAnswer,
			PromptTokens:     totalPrompt,
			CompletionTokens: totalCompletion,
			TotalTokens:  totalPrompt + totalCompletion,
			Steps:        stepCount,
		},
		Timestamp: time.Now().Format(time.RFC3339),
	})
	onEvent(StreamEvent{Type: EventDone, Timestamp: time.Now().Format(time.RFC3339)})
	return nil
}

// UpdateProvider swaps the provider for all conversations.
func (e *Engine) UpdateProvider(p provider.Provider) {
	e.mu.Lock()
	defer e.mu.Unlock()
	// New conversations will pick up the provider via the provider func
}

// SetSystemPrompt updates the system prompt for new conversations.
func (e *Engine) SetSystemPrompt(prompt string) {
	e.systemPrompt = prompt
}

func toProviderTools(defs []tools.ToolDef) []provider.ToolDefinition {
	result := make([]provider.ToolDefinition, 0, len(defs))
	for _, d := range defs {
		result = append(result, provider.ToolDefinition{
			Type: d.Type,
			Function: provider.ToolFunctionSchema{
				Name:        d.Function.Name,
				Description: d.Function.Description,
				Parameters:  d.Function.Parameters,
			},
		})
	}
	return result
}

func errorEvent(msg string) StreamEvent {
	return StreamEvent{
		Type:      EventError,
		Data:      map[string]string{"message": msg},
		Timestamp: time.Now().Format(time.RFC3339),
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// isRetryableError checks if an LLM error is worth retrying.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// Rate limiting
	if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "Too Many Requests") || strings.Contains(msg, "访问量过大") {
		return true
	}
	// Server errors
	if strings.Contains(msg, "500") || strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") || strings.Contains(msg, "Internal Server Error") ||
		strings.Contains(msg, "Bad Gateway") || strings.Contains(msg, "Service Unavailable") {
		return true
	}
	// Timeouts
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline") {
		return true
	}
	// Connection errors
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "EOF") || strings.Contains(msg, "no such host") {
		return true
	}
	return false
}
