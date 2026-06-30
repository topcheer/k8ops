package chat

import (
	"errors"
	"testing"
	"time"

	"github.com/ggai/k8ops/internal/provider"
	"github.com/ggai/k8ops/internal/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Pure logic tests for engine.go — no mock provider, no streaming.
// Tested functions: NewEngine, truncate, errorEvent, toProviderTools, isRetryableError.
// ---------------------------------------------------------------------------

// --- TestNewEngine ---

func TestNewEngine(t *testing.T) {
	registry := tools.NewRegistry()

	engine := NewEngine(
		func() provider.Provider { return nil },
		registry,
		nil,
		"you are a helpful assistant",
		nil,
	)

	assert.Equal(t, 15, engine.maxSteps, "maxSteps should default to 15")
	assert.Equal(t, 180*time.Second, engine.timeout, "timeout should default to 180s")
	require.NotNil(t, engine.conversations, "conversations map must be initialized")
	assert.Empty(t, engine.conversations, "conversations map should be empty on creation")
	assert.Same(t, registry, engine.registry, "registry reference should match")
	assert.Equal(t, "you are a helpful assistant", engine.systemPrompt)
	assert.NotNil(t, engine.circuitBreaker, "circuit breaker should be initialized")
	assert.Equal(t, 5, engine.retryCfg.MaxAttempts, "retry MaxAttempts should default to 5")
}

// --- TestTruncate ---

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string unchanged", "hello world", 100, "hello world"},
		{"exact length unchanged", "12345", 5, "12345"},
		{"empty string unchanged", "", 10, ""},
		{"long string truncated with ellipsis", "This is a very long string that definitely exceeds the max", 10, "This is a ..."},
		{"one char over gets truncated", "abcdef", 5, "abcde..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, got)
		})
	}
}

// --- TestErrorEvent ---

func TestErrorEvent(t *testing.T) {
	evt := errorEvent("something went wrong")

	assert.Equal(t, EventError, evt.Type, "event type should be EventError")

	msg, ok := evt.Data.(map[string]string)
	require.True(t, ok, "Data should be map[string]string")
	assert.Equal(t, "something went wrong", msg["message"])

	assert.NotEmpty(t, evt.Timestamp, "timestamp should not be empty")
	_, err := time.Parse(time.RFC3339, evt.Timestamp)
	assert.NoError(t, err, "timestamp should be valid RFC3339")
}

// --- TestToProviderTools ---

func TestToProviderTools(t *testing.T) {
	defs := []tools.ToolDef{
		{
			Type: "function",
			Function: tools.ToolFunc{
				Name:        "get_pods",
				Description: "List pods in a namespace",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"namespace": map[string]any{"type": "string"},
					},
					"required": []string{"namespace"},
				},
			},
		},
		{
			Type: "function",
			Function: tools.ToolFunc{
				Name:        "get_nodes",
				Description: "List cluster nodes",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
	}

	result := toProviderTools(defs)

	require.Len(t, result, 2, "should convert all tool defs")
	assert.Equal(t, "function", result[0].Type)
	assert.Equal(t, "get_pods", result[0].Function.Name)
	assert.Equal(t, "List pods in a namespace", result[0].Function.Description)
	assert.Contains(t, result[0].Function.Parameters, "properties")

	assert.Equal(t, "function", result[1].Type)
	assert.Equal(t, "get_nodes", result[1].Function.Name)
	assert.Equal(t, "List cluster nodes", result[1].Function.Description)

	// Empty input → empty output
	emptyResult := toProviderTools([]tools.ToolDef{})
	assert.Len(t, emptyResult, 0)
}

// --- TestIsRetryableError (table-driven) ---

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		// Retryable: rate limiting
		{"429 status", errors.New("HTTP 429 Too Many Requests"), true},
		{"rate limit message", errors.New("rate limit exceeded"), true},
		{"访问量过大", errors.New("访问量过大，请稍后重试"), true},

		// Retryable: server errors
		{"500 status", errors.New("HTTP 500 Internal Server Error"), true},
		{"502 status", errors.New("HTTP 502 Bad Gateway"), true},
		{"503 status", errors.New("HTTP 503 Service Unavailable"), true},
		{"Internal Server Error text", errors.New("Internal Server Error"), true},
		{"Bad Gateway text", errors.New("Bad Gateway"), true},
		{"Service Unavailable text", errors.New("Service Unavailable"), true},

		// Retryable: timeouts
		{"timeout", errors.New("context deadline exceeded: request timeout after 30s"), true},
		{"deadline exceeded", errors.New("rpc error: code = DeadlineExceeded desc = context deadline exceeded"), true},
		{"context deadline", errors.New("context deadline exceeded"), true},

		// Retryable: connection errors
		{"connection refused", errors.New("dial tcp 10.0.0.1:443: connect: connection refused"), true},
		{"connection reset", errors.New("read tcp: connection reset by peer"), true},
		{"EOF", errors.New("EOF"), true},
		{"no such host", errors.New("dial tcp: lookup api.invalid: no such host"), true},

		// NOT retryable: client errors
		{"400 bad request", errors.New("HTTP 400 Bad Request"), false},
		{"401 unauthorized", errors.New("HTTP 401 Unauthorized"), false},
		{"403 forbidden", errors.New("HTTP 403 Forbidden"), false},
		{"404 not found", errors.New("HTTP 404 Not Found"), false},

		// NOT retryable: misc
		{"plain error", errors.New("invalid model name"), false},
		{"unknown error", errors.New("unknown error"), false},

		// Edge: nil
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			assert.Equal(t, tt.retryable, got)
		})
	}
}
