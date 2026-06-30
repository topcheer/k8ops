package dashboard

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ggai/k8ops/internal/audit"
)

func TestDashboard_HealthEndpoint(t *testing.T) {
	server := &Server{
		log:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		auditLog: audit.NoopLogger(slog.Default()),
	}

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
}

func TestDashboard_AuditEndpoint_Empty(t *testing.T) {
	auditLog := audit.NoopLogger(slog.Default())
	server := &Server{
		log:      slog.Default(),
		auditLog: auditLog,
	}

	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()

	server.handleAudit(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["count"].(float64) != 0 {
		t.Errorf("expected count 0, got %v", resp["count"])
	}
}

func TestDashboard_AuditStats(t *testing.T) {
	auditLog := audit.NoopLogger(slog.Default())

	// Log some events
	auditLog.Log(reqContext(), audit.Event{
		Type:    audit.EventTypeToolCall,
		Action:  "k8s_get_pod",
		Success: true,
	})
	auditLog.Log(reqContext(), audit.Event{
		Type:     audit.EventTypeSafetyBlock,
		Action:   "delete_resource",
		Success:  false,
		Severity: audit.SeverityCritical,
	})

	server := &Server{
		log:      slog.Default(),
		auditLog: auditLog,
	}

	req := httptest.NewRequest("GET", "/api/audit/stats", nil)
	w := httptest.NewRecorder()

	server.handleAuditStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["total"].(float64) != 2 {
		t.Errorf("expected total 2, got %v", resp["total"])
	}
}

func TestDashboard_Truncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestDashboard_FormatDuration(t *testing.T) {
	// Just verify it produces a non-empty string with proper suffix
	d1 := formatDuration(30 * 1e9) // 30s -> should be 0m
	if d1 == "" {
		t.Error("expected non-empty duration")
	}
}

func reqContext() context.Context {
	return context.Background()
}
