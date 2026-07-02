package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleSystemInfo(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/info", nil)
	rr := httptest.NewRecorder()

	s.handleSystemInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleLogRotate_NoLogger(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/system/log/rotate", nil)
	rr := httptest.NewRecorder()

	s.handleLogRotate(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestHandleLogCleanup_NoLogger(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/system/log/cleanup", nil)
	rr := httptest.NewRecorder()

	s.handleLogCleanup(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}
