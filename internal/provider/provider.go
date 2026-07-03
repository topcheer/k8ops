package provider

import (
	"context"
	"fmt"
)

// MessageRole defines the role of a message.
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message represents a chat message.
type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // for tool result messages
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // for assistant messages with tool calls
}

// ToolCall represents a function/tool call from the AI.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string of arguments
}

// ToolDefinition defines a tool the AI can call.
type ToolDefinition struct {
	Type     string             `json:"type"` // always "function"
	Function ToolFunctionSchema `json:"function"`
}

// ToolFunctionSchema describes a function's interface.
type ToolFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema
}

// CompletionRequest is a chat completion request.
type CompletionRequest struct {
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

// CompletionResponse is the AI's response.
type CompletionResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// Usage statistics
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
}

// ProviderConfig holds provider configuration.
type ProviderConfig struct {
	Type        string // openai, anthropic, gemini
	Model       string
	APIKey      string
	Endpoint    string // custom endpoint
	MaxTokens   int
	Temperature float64
}

// Provider is the interface every LLM backend must implement.
type Provider interface {
	// Name returns the provider type identifier.
	Name() string
	// Complete sends a chat completion request with optional tool definitions.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	// StreamComplete sends a chat completion with streaming deltas.
	// onDelta is called for each text chunk. Returns final response with tool calls.
	StreamComplete(ctx context.Context, req CompletionRequest, onDelta func(string)) (*CompletionResponse, error)
}

// Factory creates a provider from config.
type Factory func(cfg ProviderConfig) (Provider, error)

var factories = map[string]Factory{}

// Register registers a provider factory.
func Register(name string, f Factory) {
	factories[name] = f
}

// New creates a provider from config.
func New(cfg ProviderConfig) (Provider, error) {
	f, ok := factories[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown provider type: %s (registered: %v)", cfg.Type, registeredTypes())
	}
	return f(cfg)
}

func registeredTypes() []string {
	types := make([]string, 0, len(factories))
	for k := range factories {
		types = append(types, k)
	}
	return types
}
