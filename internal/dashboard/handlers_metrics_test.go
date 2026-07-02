package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddleware(t *testing.T) {
	s := &Server{}
	handler := s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := requestIDFromCtx(r.Context())
		if reqID == "unknown" || reqID == "" {
			t.Error("expected non-empty request ID in context")
		}
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header in response")
	}
}

func TestRequestIDMiddlewarePreservesIncomingID(t *testing.T) {
	s := &Server{}
	handler := s.requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := requestIDFromCtx(r.Context())
		if reqID != "test-id-123" {
			t.Errorf("expected 'test-id-123', got %q", reqID)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("X-Request-ID", "test-id-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") != "test-id-123" {
		t.Error("expected incoming request ID to be preserved")
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	if len(id1) != 16 {
		t.Errorf("expected 16-char hex ID, got %d chars: %q", len(id1), id1)
	}
	if id1 == id2 {
		t.Error("expected unique request IDs")
	}
}

func TestHTTPMetricsMiddleware(t *testing.T) {
	s := &Server{}
	handler := s.httpMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHTTPMetricsMiddlewareSkipsStatic(t *testing.T) {
	s := &Server{}
	handler := s.httpMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	// Static asset path should still work but not record metrics
	req := httptest.NewRequest(http.MethodGet, "/styles.css", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200 for static asset, got %d", rr.Code)
	}
}

func TestHTTPMetricsMiddlewareTracksErrors(t *testing.T) {
	s := &Server{}
	handler := s.httpMetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/scale", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != 500 {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/health", "/api/health"},
		{"/api/nodes/node1/pods", "/api/nodes/{node}/pods"},
		{"/api/pods/default/nginx-abc/logs", "/api/pods/{ns}/{name}/logs"},
		{"/api/diagnostics/report-123", "/api/diagnostics/{name}"},
		{"/api/diagnostics", "/api/diagnostics"},
		{"/api/audit/events/abc123", "/api/audit/events/abc123"},
		{"/styles.css", "/styles.css"},
		{"/api/pods/default/nginx/containers?follow=true", "/api/pods/{ns}/{name}/containers"},
	}

	for _, tt := range tests {
		got := normalizePath(tt.input)
		if got != tt.expected {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
