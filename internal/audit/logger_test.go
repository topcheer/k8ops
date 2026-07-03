package audit

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestLogger_NoFile(t *testing.T) {
	l, err := NewLogger("", testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Close()

	l.Log(context.Background(), Event{
		Type:    EventTypeToolCall,
		Action:  "k8s_get_pod",
		Actor:   "ai-agent",
		Success: true,
	})

	events := l.Recent(10)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Action != "k8s_get_pod" {
		t.Errorf("expected action 'k8s_get_pod', got '%s'", events[0].Action)
	}
}

func TestLogger_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.jsonl")

	l, err := NewLogger(path, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	l.Log(context.Background(), Event{
		Type:     EventTypeRemediation,
		Action:   "scale_deployment",
		Target:   "Deployment/app",
		Success:  true,
		Severity: SeverityWarning,
	})
	l.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}
	if !strings.Contains(string(data), "scale_deployment") {
		t.Error("audit file should contain action name")
	}
}

func TestLogger_RingBuffer(t *testing.T) {
	l, _ := NewLogger("", testLogger())
	defer l.Close()

	for i := 0; i < 10; i++ {
		l.Log(context.Background(), Event{
			Action:  "action_" + string(rune('A'+i)),
			Success: true,
		})
	}

	recent := l.Recent(5)
	if len(recent) != 5 {
		t.Errorf("expected 5 events, got %d", len(recent))
	}
	// Most recent should be last logged
	if recent[0].Action != "action_J" {
		t.Errorf("expected newest 'action_J', got '%s'", recent[0].Action)
	}
}

func TestLogger_Filter(t *testing.T) {
	l, _ := NewLogger("", testLogger())
	defer l.Close()

	l.Log(context.Background(), Event{Type: EventTypeToolCall, Action: "a", Namespace: "default"})
	l.Log(context.Background(), Event{Type: EventTypeRemediation, Action: "b", Namespace: "kube-system"})
	l.Log(context.Background(), Event{Type: EventTypeToolCall, Action: "c", Namespace: "default"})

	filtered := l.Filter(EventTypeToolCall, "default", 10)
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered events, got %d", len(filtered))
	}

	allKubeSystem := l.Filter("", "kube-system", 10)
	if len(allKubeSystem) != 1 {
		t.Errorf("expected 1 kube-system event, got %d", len(allKubeSystem))
	}
}

func TestLogger_Stats(t *testing.T) {
	l, _ := NewLogger("", testLogger())
	defer l.Close()

	l.Log(context.Background(), Event{Type: EventTypeToolCall, Success: true, Severity: SeverityInfo})
	l.Log(context.Background(), Event{Type: EventTypeRemediation, Success: false, Severity: SeverityCritical})
	l.Log(context.Background(), Event{Type: EventTypeSafetyBlock, Success: false, Severity: SeverityWarning})

	stats := l.Stats()
	if stats["total"] != 3 {
		t.Errorf("expected total 3, got %v", stats["total"])
	}
	if stats["successCount"] != 1 {
		t.Errorf("expected 1 success, got %v", stats["successCount"])
	}
	if stats["failureCount"] != 2 {
		t.Errorf("expected 2 failures, got %v", stats["failureCount"])
	}
	byType := stats["byType"].(map[string]int)
	if byType[string(EventTypeSafetyBlock)] != 1 {
		t.Errorf("expected 1 safety_block, got %v", byType)
	}
}

func TestNoopLogger(t *testing.T) {
	l := NoopLogger(testLogger())
	if l == nil {
		t.Fatal("expected non-nil noop logger")
	}
	l.Log(context.Background(), Event{Action: "test"})
	if len(l.Recent(10)) != 1 {
		t.Error("noop logger should still keep in-memory events")
	}
}

func TestQueryFile_Pagination(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.jsonl")

	l, err := NewLogger(path, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i := 0; i < 10; i++ {
		l.Log(context.Background(), Event{
			Type:     EventTypeToolCall,
			Action:   fmt.Sprintf("action_%d", i),
			Severity: SeverityInfo,
			Success:  true,
		})
	}
	l.Close()

	// Page 1, limit 3
	events, total, err := l.QueryFile(1, 3, "", "", "")
	if err != nil {
		t.Fatalf("QueryFile error: %v", err)
	}
	if total != 10 {
		t.Errorf("expected total 10, got %d", total)
	}
	if len(events) != 3 {
		t.Errorf("expected 3 events on page 1, got %d", len(events))
	}

	// Page 4, should give 1 result
	events, total, err = l.QueryFile(4, 3, "", "", "")
	if err != nil {
		t.Fatalf("QueryFile error: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event on page 4, got %d", len(events))
	}

	// Page 5, should be empty
	events, total, err = l.QueryFile(5, 3, "", "", "")
	if err != nil {
		t.Fatalf("QueryFile error: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events on page 5, got %d", len(events))
	}
}

func TestQueryFile_SeverityFilter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.jsonl")

	l, err := NewLogger(path, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	l.Log(context.Background(), Event{Type: EventTypeToolCall, Action: "a", Severity: SeverityInfo, Success: true})
	l.Log(context.Background(), Event{Type: EventTypeRemediation, Action: "b", Severity: SeverityWarning, Success: true})
	l.Log(context.Background(), Event{Type: EventTypeSafetyBlock, Action: "c", Severity: SeverityCritical, Success: false})
	l.Close()

	events, total, err := l.QueryFile(1, 50, "critical", "", "")
	if err != nil {
		t.Fatalf("QueryFile error: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 critical event, got %d", total)
	}
	if len(events) != 1 || events[0].Action != "c" {
		t.Errorf("expected event 'c', got %+v", events)
	}
}

func TestQueryFile_FallbackToRingBuffer(t *testing.T) {
	l, err := NewLogger("", testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer l.Close()

	l.Log(context.Background(), Event{Type: EventTypeToolCall, Action: "ring_a", Severity: SeverityInfo, Success: true})
	l.Log(context.Background(), Event{Type: EventTypeRemediation, Action: "ring_b", Severity: SeverityCritical, Success: false})

	events, total, err := l.QueryFile(1, 50, "", "", "")
	if err != nil {
		t.Fatalf("QueryFile error: %v", err)
	}
	if total != 2 {
		t.Errorf("expected total 2 from ring buffer, got %d", total)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events from ring buffer, got %d", len(events))
	}
}

func TestGetByID(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.jsonl")

	l, err := NewLogger(path, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	l.Log(context.Background(), Event{ID: "evt-001", Action: "first", Severity: SeverityInfo, Success: true})
	l.Log(context.Background(), Event{ID: "evt-002", Action: "second", Severity: SeverityWarning, Success: true})
	l.Close()

	ev, err := l.GetByID("evt-002")
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if ev == nil {
		t.Fatal("expected event, got nil")
	}
	if ev.Action != "second" {
		t.Errorf("expected action 'second', got '%s'", ev.Action)
	}

	// Non-existent ID
	ev, err = l.GetByID("evt-999")
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if ev != nil {
		t.Errorf("expected nil for non-existent ID, got %+v", ev)
	}
}
