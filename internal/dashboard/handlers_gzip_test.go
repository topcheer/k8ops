package dashboard

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGzipMiddleware_CompressesJSON(t *testing.T) {
	body := `{"message":"hello world this is a long enough string to compress well"}`

	s := &Server{log: testLogger()}
	handler := s.gzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))

	// Need to set path to /api/ for compression to trigger
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("Content-Encoding = %q, want %q", w.Header().Get("Content-Encoding"), "gzip")
	}

	gr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip reader error: %v", err)
	}
	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("read all error: %v", err)
	}
	if string(decompressed) != body {
		t.Errorf("body mismatch: got %q, want %q", string(decompressed), body)
	}
}

func TestGzipMiddleware_SkipsNonAPI(t *testing.T) {
	s := &Server{log: testLogger()}
	handler := s.gzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>hello</body></html>"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("static files should not be gzipped")
	}
}

func TestGzipMiddleware_SkipsNoAcceptEncoding(t *testing.T) {
	s := &Server{log: testLogger()}
	handler := s.gzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	// No Accept-Encoding header
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not gzip when client does not accept encoding")
	}
}

func TestGzipMiddleware_SkipsSSE(t *testing.T) {
	s := &Server{log: testLogger()}
	handler := s.gzipMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("data: hello\n\n"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/stream", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// SSE should not have gzip encoding
	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("SSE streams should not be gzipped")
	}

	// Body should be readable as plain text
	body := w.Body.String()
	if !strings.Contains(body, "data: hello") {
		t.Errorf("SSE body should contain event data, got: %q", body)
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	s := &Server{log: testLogger()}
	handler := s.securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options header")
	}
	if w.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("missing X-Frame-Options header")
	}
}
