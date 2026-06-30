package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- TestRegistry ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	mock := &mockTool{name: "test_tool", desc: "A test tool"}
	r.Register(mock)

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("expected to find registered tool")
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected name 'test_tool', got '%s'", got.Name())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent tool")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "a"})
	r.Register(&mockTool{name: "b"})
	r.Register(&mockTool{name: "c"})

	list := r.List()
	if len(list) != 3 {
		t.Errorf("expected 3 tools, got %d", len(list))
	}
}

func TestRegistry_Definitions(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "test", desc: "Test description"})

	defs := r.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Function.Name != "test" {
		t.Errorf("expected name 'test', got '%s'", defs[0].Function.Name)
	}
	if defs[0].Type != "function" {
		t.Errorf("expected type 'function', got '%s'", defs[0].Type)
	}
}

// --- TestSchema ---

func TestSchema_Basic(t *testing.T) {
	s := Schema(map[string]Property{
		"name":  {Type: "string", Description: "The name"},
		"count": {Type: "integer", Description: "The count"},
	}, []string{"name"})

	if s["type"] != "object" {
		t.Errorf("expected type 'object', got %v", s["type"])
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties to be a map")
	}
	if _, ok := props["name"]; !ok {
		t.Error("expected 'name' property")
	}
	required, ok := s["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "name" {
		t.Errorf("expected required ['name'], got %v", s["required"])
	}
}

func TestSchema_WithEnum(t *testing.T) {
	s := Schema(map[string]Property{
		"mode": {Type: "string", Enum: []string{"auto", "manual"}},
	}, nil)

	props := s["properties"].(map[string]any)
	mode := props["mode"].(map[string]any)
	enum, ok := mode["enum"].([]string)
	if !ok || len(enum) != 2 {
		t.Errorf("expected enum with 2 values, got %v", mode["enum"])
	}
}

// --- TestHelpers ---

func TestGetString(t *testing.T) {
	args := map[string]any{"name": "hello"}
	v, err := GetString(args, "name")
	if err != nil || v != "hello" {
		t.Errorf("expected 'hello', got '%s', err=%v", v, err)
	}

	_, err = GetString(args, "missing")
	if err == nil {
		t.Error("expected error for missing key")
	}

	bad := map[string]any{"name": 123}
	_, err = GetString(bad, "name")
	if err == nil {
		t.Error("expected error for non-string value")
	}
}

func TestGetStringDefault(t *testing.T) {
	args := map[string]any{"name": "value"}
	if v := GetStringDefault(args, "name", "def"); v != "value" {
		t.Errorf("expected 'value', got '%s'", v)
	}
	if v := GetStringDefault(args, "missing", "def"); v != "def" {
		t.Errorf("expected 'def', got '%s'", v)
	}
}

func TestGetInt(t *testing.T) {
	args := map[string]any{"count": float64(42)}
	v, err := GetInt(args, "count")
	if err != nil || v != 42 {
		t.Errorf("expected 42, got %d, err=%v", v, err)
	}

	args2 := map[string]any{"count": 10}
	v, err = GetInt(args2, "count")
	if err != nil || v != 10 {
		t.Errorf("expected 10, got %d, err=%v", v, err)
	}
}

func TestGetIntDefault(t *testing.T) {
	args := map[string]any{}
	if v := GetIntDefault(args, "missing", 99); v != 99 {
		t.Errorf("expected 99, got %d", v)
	}
}

func TestGetBool(t *testing.T) {
	args := map[string]any{"flag": true}
	if !GetBool(args, "flag") {
		t.Error("expected true")
	}
	if GetBool(args, "missing") {
		t.Error("expected false for missing key")
	}
}

// --- TestConcurrency ---

func TestRegistry_Concurrent(t *testing.T) {
	r := NewRegistry()

	var wg sync.WaitGroup

	// Writer goroutine: registers 100 tools.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			r.Register(&mockTool{name: fmt.Sprintf("tool_%d", i), desc: "concurrent tool"})
		}
	}()

	// Multiple reader goroutines: Get/List/Definitions while writes happen.
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				// Read access — must not panic under concurrent writes.
				_, _ = r.Get(fmt.Sprintf("tool_%d", i))
				_ = r.List()
				_ = r.Definitions()
			}
		}(g)
	}

	wg.Wait()

	// Final consistency check.
	list := r.List()
	if len(list) != 100 {
		t.Errorf("expected 100 tools after concurrent writes, got %d", len(list))
	}
}

// --- Mock Tool ---

type mockTool struct {
	name string
	desc string
}

func (m *mockTool) Name() string                 { return m.name }
func (m *mockTool) Description() string           { return m.desc }
func (m *mockTool) Parameters() map[string]any {
	return Schema(map[string]Property{}, nil)
}
func (m *mockTool) Execute(ctx context.Context, args map[string]any) (*ToolResult, error) {
	return &ToolResult{Success: true, Output: "mock output"}, nil
}

// Ensure mockTool satisfies Tool interface.
var _ Tool = (*mockTool)(nil)

// Prevent unused import warning.
var _ = errors.New

// --- Concurrency Test ---

func TestRegistry_ConcurrentAccess(t *testing.T) {
	// This test is designed to be run with -race to detect data races.
	// Simulates the real-world scenario: goroutines registering tools
	// (hot-update provider) while others read via Get/List/Definitions
	// (serving chat requests).
	r := NewRegistry()

	// Pre-register some tools so readers have data to work with.
	for i := 0; i < 10; i++ {
		r.Register(&mockTool{name: fmt.Sprintf("tool_%d", i)})
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writer goroutines: continuously register tools.
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
					r.Register(&mockTool{name: fmt.Sprintf("writer_%d_%d", id, i)})
				}
			}
		}(w)
	}

	// Reader goroutines: continuously read via Get/List/Definitions.
	for rd := 0; rd < 8; rd++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					switch id % 3 {
					case 0:
						_, _ = r.Get("tool_0")
					case 1:
						_ = r.List()
					case 2:
						_ = r.Definitions()
					}
				}
			}
		}(rd)
	}

	// Let the goroutines run for a moment.
	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}
