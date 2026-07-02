package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAuditExport_NilLogger(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/audit/export", nil)
	rr := httptest.NewRecorder()

	s.handleAuditExport(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestGetStringField(t *testing.T) {
	m := map[string]any{
		"name":   "test",
		"number": 42,
	}

	if got := getStringField(m, "name"); got != "test" {
		t.Errorf("getStringField(name) = %q, want 'test'", got)
	}
	if got := getStringField(m, "number"); got != "" {
		t.Errorf("getStringField(number) = %q, want ''", got)
	}
	if got := getStringField(m, "missing"); got != "" {
		t.Errorf("getStringField(missing) = %q, want ''", got)
	}
}

func TestHandleAuditEvents_FilterNilLogger(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/audit/events", nil)
	rr := httptest.NewRecorder()

	s.handleAuditEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleAuditEvents_WithFilters(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/audit/events?actor=admin&action=delete&q=test&page=1&limit=10", nil)
	rr := httptest.NewRecorder()

	s.handleAuditEvents(rr, req)

	// Should return 200 with empty items (nil logger)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
