package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"/api/namespaces/default/detail", []string{"api", "namespaces", "default", "detail"}},
		{"/api/health", []string{"api", "health"}},
		{"/", []string{}},
		{"", []string{}},
		{"/api//double/", []string{"api", "double"}},
	}

	for _, tt := range tests {
		got := splitPath(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("splitPath(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.expected, len(tt.expected))
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("splitPath(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestFormatMilliCores(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{500, "500m"},
		{1000, "1.00 cores"},
		{2500, "2.50 cores"},
		{0, "0m"},
	}

	for _, tt := range tests {
		got := formatMilliCores(tt.input)
		if got != tt.expected {
			t.Errorf("formatMilliCores(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestHandleNamespaceRanking_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/namespaces/ranking", nil)
	rr := httptest.NewRecorder()

	s.handleNamespaceRanking(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestHandleNamespaceDetail_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/namespaces/default/detail", nil)
	rr := httptest.NewRecorder()

	s.handleNamespaceDetail(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}
