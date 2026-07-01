package tools

import (
	"context"
	"fmt"
	"sync"
)

// ToolResult is the output of a tool execution.
type ToolResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// Tool is the interface every tool must implement.
type Tool interface {
	// Name returns the tool name (used as the function name for AI tool calling).
	Name() string
	// Description returns a human-readable description.
	Description() string
	// Parameters returns the JSON Schema for the tool's parameters.
	Parameters() map[string]any
	// Execute runs the tool with the given arguments.
	Execute(ctx context.Context, args map[string]any) (*ToolResult, error)
}

// Registry holds all registered tools.
// All methods are safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// Definitions returns tool definitions suitable for LLM function calling.
func (r *Registry) Definitions() []ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, ToolDef{
			Type: "function",
			Function: ToolFunc{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		})
	}
	return defs
}

// ToolDef is a simplified tool definition for provider compatibility.
type ToolDef struct {
	Type     string   `json:"type"`
	Function ToolFunc `json:"function"`
}

// ToolFunc describes a function's schema.
type ToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Helper: build a simple JSON Schema for tool parameters.
func Schema(props map[string]Property, required []string) map[string]any {
	properties := make(map[string]any)
	for name, p := range props {
		properties[name] = p.toJSON()
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}

// Property describes a single parameter.
type Property struct {
	Type        string
	Description string
	Enum        []string
	Default     any
	Items       *Property
	Properties  map[string]Property
}

func (p Property) toJSON() map[string]any {
	m := map[string]any{
		"type":        p.Type,
		"description": p.Description,
	}
	if len(p.Enum) > 0 {
		m["enum"] = p.Enum
	}
	if p.Default != nil {
		m["default"] = p.Default
	}
	if p.Items != nil {
		m["items"] = p.Items.toJSON()
	}
	if len(p.Properties) > 0 {
		props := make(map[string]any)
		for k, v := range p.Properties {
			props[k] = v.toJSON()
		}
		m["properties"] = props
	}
	return m
}

// Helper functions to safely extract values from args.

func GetString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s must be a string, got %T", key, v)
	}
	return s, nil
}

func GetStringDefault(args map[string]any, key, def string) string {
	v, ok := args[key]
	if !ok {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	return s
}

func GetInt(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: %s", key)
	}
	f, ok := v.(float64)
	if ok {
		return int(f), nil
	}
	i, ok := v.(int)
	if ok {
		return i, nil
	}
	return 0, fmt.Errorf("parameter %s must be a number", key)
}

func GetIntDefault(args map[string]any, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	f, ok := v.(float64)
	if ok {
		return int(f)
	}
	i, ok := v.(int)
	if ok {
		return i
	}
	return def
}

func GetBool(args map[string]any, key string) bool {
	v, ok := args[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}
