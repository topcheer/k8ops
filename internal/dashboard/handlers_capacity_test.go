package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleStorageCapacity_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/storage/capacity", nil)
	rr := httptest.NewRecorder()

	s.handleStorageCapacity(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestHandleCapacityPlanning_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/capacity/planning", nil)
	rr := httptest.NewRecorder()

	s.handleCapacityPlanning(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestFormatStorageGB(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{500 * 1024 * 1024, "500 MB"},               // 500 MB
		{int64(1.5 * 1024 * 1024 * 1024), "1.5 GB"}, // 1.5 GB
		{10 * 1024 * 1024 * 1024, "10.0 GB"},        // 10 GB
	}

	for _, tt := range tests {
		got := formatStorageGB(tt.input)
		if got != tt.expected {
			t.Errorf("formatStorageGB(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSafeDeref(t *testing.T) {
	s := "hello"
	if got := safeDeref(&s); got != "hello" {
		t.Errorf("safeDeref(&\"hello\") = %q, want \"hello\"", got)
	}
	if got := safeDeref(nil); got != "" {
		t.Errorf("safeDeref(nil) = %q, want \"\"", got)
	}
	trimmed := "  spaces  "
	if got := safeDeref(&trimmed); got != "spaces" {
		t.Errorf("safeDeref with spaces = %q, want \"spaces\"", got)
	}
}
