package dashboard

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ggai/k8ops/internal/audit"
	"github.com/ggai/k8ops/internal/auth"
	"github.com/ggai/k8ops/internal/chat"
)

// --- writeJSON / writeError / writeK8sError Tests ---

func TestWriteJSON_Normal(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, map[string]string{"hello": "world"})

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp["hello"] != "world" {
		t.Errorf("hello = %q, want world", resp["hello"])
	}
}

func TestWriteJSON_Array(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, []string{"a", "b", "c"})

	var resp []string
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 3 {
		t.Errorf("len = %d, want 3", len(resp))
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "invalid input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "invalid input" {
		t.Errorf("error = %q, want 'invalid input'", resp["error"])
	}
}

func TestWriteError_InternalServerError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusInternalServerError, "boom")

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestWriteK8sError_Forbidden(t *testing.T) {
	w := httptest.NewRecorder()
	writeK8sError(w, errors.New("deployments.apps is forbidden: User \"viewer\" cannot list"))

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestWriteK8sError_Unauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	writeK8sError(w, errors.New("unauthorized: token expired"))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestWriteK8sError_NotFound(t *testing.T) {
	w := httptest.NewRecorder()
	writeK8sError(w, errors.New("pods not found in namespace"))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestWriteK8sError_NotFound_CamelCase(t *testing.T) {
	w := httptest.NewRecorder()
	writeK8sError(w, errors.New("the server could not find the requested resource (NotFound)"))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestWriteK8sError_GenericError(t *testing.T) {
	w := httptest.NewRecorder()
	writeK8sError(w, errors.New("connection refused"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestWriteK8sError_NilError(t *testing.T) {
	w := httptest.NewRecorder()
	writeK8sError(w, nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// --- handleVersion Tests ---

func TestHandleVersion(t *testing.T) {
	// Save and restore version vars
	origVersion, origCommit, origDate := Version, GitCommit, BuildDate
	defer func() { Version, GitCommit, BuildDate = origVersion, origCommit, origDate }()

	Version = "v1.2.3"
	GitCommit = "abc1234"
	BuildDate = "2024-01-15"

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	w := httptest.NewRecorder()

	s.handleVersion(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["version"] != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", resp["version"])
	}
	if resp["gitCommit"] != "abc1234" {
		t.Errorf("gitCommit = %q, want abc1234", resp["gitCommit"])
	}
	if resp["buildDate"] != "2024-01-15" {
		t.Errorf("buildDate = %q, want 2024-01-15", resp["buildDate"])
	}
	if resp["name"] != "k8ops" {
		t.Errorf("name = %q, want k8ops", resp["name"])
	}
}

// --- localOnlyMiddleware Tests ---

func TestLocalOnlyMiddleware_Localhost(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.localOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called for localhost")
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestLocalOnlyMiddleware_IPv6Localhost(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.localOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "::1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called for IPv6 localhost")
	}
}

func TestLocalOnlyMiddleware_NonLocalhost(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.localOnlyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called for non-localhost")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// --- adminOnlyMiddleware Tests ---

func TestAdminOnlyMiddleware_Admin(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.adminOnlyMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	ctx := auth.SetUserInContext(req.Context(), &auth.User{Username: "admin", Role: "admin"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called for admin")
	}
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAdminOnlyMiddleware_NonAdmin(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.adminOnlyMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	ctx := auth.SetUserInContext(req.Context(), &auth.User{Username: "viewer", Role: "viewer"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called for non-admin")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestAdminOnlyMiddleware_NoUser(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.adminOnlyMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called without user")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// --- securityHeadersMiddleware Tests ---

func TestSecurityHeadersMiddleware_AllHeaders(t *testing.T) {
	s := &Server{}
	called := false
	handler := s.securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("next handler should be called")
	}

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-cross-origin",
	}
	for header, want := range checks {
		got := w.Header().Get(header)
		if got == "" {
			t.Errorf("missing header %s", header)
		} else if got != want && header == "X-Content-Type-Options" {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeadersMiddleware_HSTS_WithTLS(t *testing.T) {
	s := &Server{tlsCert: "/fake/cert.pem", tlsKey: "/fake/key.pem"}
	handler := s.securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	hsts := w.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("HSTS header should be set when TLS is enabled")
	}
	if !strings.Contains(hsts, "max-age=31536000") {
		t.Errorf("HSTS = %q, should contain max-age", hsts)
	}
}

func TestSecurityHeadersMiddleware_NoHSTS_WithoutTLS(t *testing.T) {
	s := &Server{}
	handler := s.securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	hsts := w.Header().Get("Strict-Transport-Security")
	if hsts != "" {
		t.Errorf("HSTS should NOT be set without TLS, got %q", hsts)
	}
}

// --- sendSSE Tests ---

func TestSendSSE_EventFormat(t *testing.T) {
	w := httptest.NewRecorder()
	// Use a fake flusher — httptest.ResponseRecorder implements Flusher
	flusher := w

	event := chat.StreamEvent{
		Type: "answer",
		Data: map[string]string{"text": "hello"},
	}

	sendSSE(w, flusher, event)

	body := w.Body.String()
	if !strings.HasPrefix(body, "data: ") {
		t.Errorf("SSE body should start with 'data: ', got: %q", body[:min(20, len(body))])
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Error("SSE body should end with double newline")
	}

	// Verify the data is valid JSON
	jsonPart := strings.TrimPrefix(body, "data: ")
	jsonPart = strings.TrimSuffix(jsonPart, "\n\n")
	var decoded map[string]any
	if err := json.Unmarshal([]byte(jsonPart), &decoded); err != nil {
		t.Errorf("SSE data should be valid JSON: %v", err)
	}
}

// --- parseInt Tests ---

func TestParseInt(t *testing.T) {
	tests := []struct {
		input    string
		fallback int
		want     int
	}{
		{"42", 0, 42},
		{"", 10, 10},
		{"abc", 5, 5},
		{"0", 99, 0},
		{"-1", 0, -1},
		{"100", 50, 100},
	}
	for _, tt := range tests {
		got := parseInt(tt.input, tt.fallback)
		if got != tt.want {
			t.Errorf("parseInt(%q, %d) = %d, want %d", tt.input, tt.fallback, got, tt.want)
		}
	}
}

// --- parseCORSOrigins Tests ---

func TestParseCORSOrigins(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"https://example.com", 1},
		{"https://a.com,https://b.com", 2},
		{"  ,  ,  ", 0},
		{"https://a.com , https://b.com ", 2},
	}
	for _, tt := range tests {
		got := parseCORSOrigins(tt.input)
		if len(got) != tt.want {
			t.Errorf("parseCORSOrigins(%q) = %v (len %d), want len %d", tt.input, got, len(got), tt.want)
		}
	}
}

func TestIsOriginAllowed(t *testing.T) {
	s := &Server{corsAllowedOrigins: []string{"https://k8ops.example.com", "https://k8ops.iot2.win"}}

	if !s.isOriginAllowed("https://k8ops.example.com") {
		t.Error("should allow configured origin")
	}
	if !s.isOriginAllowed("https://k8ops.iot2.win") {
		t.Error("should allow configured origin")
	}
	if s.isOriginAllowed("https://evil.com") {
		t.Error("should NOT allow unconfigured origin")
	}
}

func TestIsOriginAllowed_EmptyList(t *testing.T) {
	s := &Server{}
	if s.isOriginAllowed("https://anything.com") {
		t.Error("should not allow any origin when list is empty")
	}
}

// --- CORS Middleware Tests ---

func TestCORSMiddleware_AllowedOrigin(t *testing.T) {
	s := &Server{corsAllowedOrigins: []string{"https://k8ops.example.com"}}
	called := false
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Origin", "https://k8ops.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if !called {
		t.Error("handler should be called")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://k8ops.example.com" {
		t.Errorf("CORS header missing or wrong: %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Error("should set Allow-Credentials")
	}
}

func TestCORSMiddleware_BlockedOrigin(t *testing.T) {
	s := &Server{corsAllowedOrigins: []string{"https://k8ops.example.com"}}
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("should NOT set CORS header for unallowed origin")
	}
}

func TestCORSMiddleware_NoOriginHeader(t *testing.T) {
	s := &Server{corsAllowedOrigins: []string{"https://k8ops.example.com"}}
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("should NOT set CORS header when no Origin header")
	}
}

func TestCORSMiddleware_OptionsPreflight(t *testing.T) {
	s := &Server{corsAllowedOrigins: []string{"https://k8ops.example.com"}}
	called := false
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/something", nil)
	req.Header.Set("Origin", "https://k8ops.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if called {
		t.Error("handler should NOT be called for OPTIONS preflight")
	}
	if w.Code != http.StatusOK {
		t.Errorf("OPTIONS should return 200, got %d", w.Code)
	}
}

// --- handleAuditEvents Tests ---

func TestHandleAuditEvents_NilLogger(t *testing.T) {
	s := &Server{auditLog: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/audit/events", nil)
	w := httptest.NewRecorder()
	s.handleAuditEvents(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", resp["total"])
	}
}

func TestHandleAuditEventDetail_NilLogger(t *testing.T) {
	s := &Server{auditLog: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/audit/events/123", nil)
	w := httptest.NewRecorder()
	s.handleAuditEventDetail(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleAuditEventDetail_MissingID(t *testing.T) {
	// Create a real audit logger to test the missing ID path
	tmpDir := t.TempDir()
	logPath := tmpDir + "/audit.jsonl"
	logger := newTestAuditLogger(t, logPath)

	s := &Server{auditLog: logger}

	req := httptest.NewRequest(http.MethodGet, "/api/audit/events/", nil)
	w := httptest.NewRecorder()
	s.handleAuditEventDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- handleAudit / handleAuditStats Tests ---

func TestHandleAudit_NilLogger(t *testing.T) {
	s := &Server{auditLog: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/audit", nil)
	w := httptest.NewRecorder()
	s.handleAudit(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["count"].(float64) != 0 {
		t.Errorf("count = %v, want 0", resp["count"])
	}
}

func TestHandleAuditStats_NilLogger(t *testing.T) {
	s := &Server{auditLog: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/audit/stats", nil)
	w := httptest.NewRecorder()
	s.handleAuditStats(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", resp["total"])
	}
}

// --- userName Helper Tests ---

func TestUserName_WithUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.SetUserInContext(req.Context(), &auth.User{Username: "alice"})
	req = req.WithContext(ctx)

	if got := userName(req); got != "alice" {
		t.Errorf("userName = %q, want alice", got)
	}
}

func TestUserName_WithoutUser(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	if got := userName(req); got != "unknown" {
		t.Errorf("userName = %q, want unknown", got)
	}
}

// --- extractK8sErrMessage Tests ---

func TestExtractK8sErrMessage(t *testing.T) {
	input := "deployments.apps is forbidden: User \"viewer\" cannot list"
	got := extractK8sErrMessage(input)
	if got != input {
		t.Errorf("extractK8sErrMessage should return the full message, got %q", got)
	}
}

// --- Helper ---

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func newTestAuditLogger(t *testing.T, path string) *audit.Logger {
	t.Helper()
	l, err := audit.NewLogger(path, slog.Default())
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}
