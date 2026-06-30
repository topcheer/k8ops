package dashboard

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ggai/k8ops/internal/auth"
)

// --- handleSlackWebhook Tests ---

func TestHandleSlackWebhook_MethodNotAllowed(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/webhooks/slack", nil)

	s.handleSlackWebhook(w, r)

	if w.Code != 405 {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSlackWebhook_NoAuth(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/webhooks/slack", strings.NewReader(`{}`))

	s.handleSlackWebhook(w, r)

	if w.Code != 403 {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestHandleSlackWebhook_NonAdmin(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/webhooks/slack", strings.NewReader(`{}`))

	// Set a non-admin user in context
	ctx := auth.SetUserInContext(r.Context(), &auth.User{Username: "viewer", Role: "viewer"})
	r = r.WithContext(ctx)

	s.handleSlackWebhook(w, r)

	if w.Code != 403 {
		t.Errorf("status = %d, want 403 for non-admin", w.Code)
	}
}

func TestHandleSlackWebhook_NoWebhookURL(t *testing.T) {
	orig := slackWebhookURL
	slackWebhookURL = ""
	defer func() { slackWebhookURL = orig }()

	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/webhooks/slack", strings.NewReader(`{}`))

	ctx := auth.SetUserInContext(r.Context(), &auth.User{Username: "admin", Role: "admin"})
	r = r.WithContext(ctx)

	s.handleSlackWebhook(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when SLACK_WEBHOOK_URL not set", w.Code)
	}
}

func TestHandleSlackWebhook_InvalidBody(t *testing.T) {
	orig := slackWebhookURL
	slackWebhookURL = "https://hooks.slack.com/test"
	defer func() { slackWebhookURL = orig }()

	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/webhooks/slack", strings.NewReader(`not json`))

	ctx := auth.SetUserInContext(r.Context(), &auth.User{Username: "admin", Role: "admin"})
	r = r.WithContext(ctx)

	s.handleSlackWebhook(w, r)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400 for invalid JSON", w.Code)
	}
}

func TestHandleSlackWebhook_EmptyMessage(t *testing.T) {
	orig := slackWebhookURL
	slackWebhookURL = "https://hooks.slack.com/test"
	defer func() { slackWebhookURL = orig }()

	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/webhooks/slack",
		strings.NewReader(`{"type":"test","message":""}`))

	ctx := auth.SetUserInContext(r.Context(), &auth.User{Username: "admin", Role: "admin"})
	r = r.WithContext(ctx)

	s.handleSlackWebhook(w, r)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400 for empty message", w.Code)
	}
}

// --- cache.go Tests ---

func TestResponseCache_SetGet(t *testing.T) {
	c := newResponseCache(5 * time.Minute)

	// Empty cache returns miss
	if _, ok := c.get("/api/test"); ok {
		t.Error("expected cache miss on empty cache")
	}

	// Set and get
	c.set("/api/test", []byte(`{"hello":"world"}`))
	data, ok := c.get("/api/test")
	if !ok {
		t.Fatal("expected cache hit after set")
	}
	if string(data) != `{"hello":"world"}` {
		t.Errorf("cached data = %q, want {\"hello\":\"world\"}", string(data))
	}
}

func TestResponseCache_Invalidate(t *testing.T) {
	c := newResponseCache(5 * time.Minute)

	c.set("/api/pods", []byte(`{}`))
	c.set("/api/nodes", []byte(`{}`))

	c.invalidate("/api/pods")

	if _, ok := c.get("/api/pods"); ok {
		t.Error("expected cache miss after invalidate")
	}
	if _, ok := c.get("/api/nodes"); !ok {
		t.Error("expected /api/nodes to still be cached")
	}
}

func TestResponseCache_InvalidatePrefix(t *testing.T) {
	c := newResponseCache(5 * time.Minute)

	c.set("/api/v1/pods", []byte(`{}`))
	c.set("/api/v1/nodes", []byte(`{}`))
	c.set("/api/v2/pods", []byte(`{}`))

	c.invalidatePrefix("/api/v1/")

	if _, ok := c.get("/api/v1/pods"); ok {
		t.Error("expected /api/v1/pods invalidated")
	}
	if _, ok := c.get("/api/v1/nodes"); ok {
		t.Error("expected /api/v1/nodes invalidated")
	}
	if _, ok := c.get("/api/v2/pods"); !ok {
		t.Error("expected /api/v2/pods to still be cached")
	}
}

func TestResponseCache_Expiry(t *testing.T) {
	c := newResponseCache(50 * time.Millisecond)

	c.set("/api/test", []byte(`{}`))

	if _, ok := c.get("/api/test"); !ok {
		t.Fatal("expected cache hit immediately after set")
	}

	time.Sleep(60 * time.Millisecond)

	if _, ok := c.get("/api/test"); ok {
		t.Error("expected cache miss after expiry")
	}
}

// --- formatDuration Tests ---

func TestFormatDuration_Detailed(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want string
	}{
		{"seconds", 45 * time.Second, "1m"}, // formatDuration rounds 45s to 1m
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3 * time.Hour, "3h"},
		{"days", 48*time.Hour + 5*time.Minute, "2d"},
		{"zero", 0, "0m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.dur)
			if !strings.Contains(got, tt.want) {
				t.Errorf("formatDuration(%v) = %q, want to contain %q", tt.dur, got, tt.want)
			}
		})
	}
}

// --- handleConversations Tests (no chat engine returns empty list) ---

func TestHandleConversations_NoEngine(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/chat/conversations", nil)

	s.handleConversations(w, r)

	// With no engine, returns 200 + empty list
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp["conversations"] == nil {
		t.Error("expected conversations key")
	}
}

// --- handleChat Tests (no chat engine) ---

func TestHandleChat_NoEngine(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/chat", nil)

	s.handleChat(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// --- handleProviderStatus Tests (no provider manager) ---

func TestHandleProviderStatus_NoManager(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/provider/status", nil)

	s.handleProviderStatus(w, r)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["active"] != false {
		t.Errorf("active = %v, want false", resp["active"])
	}
}

// --- handleRemediations Tests (no k8s client — should panic) ---
// handleRemediations calls clientsFromReq which returns shared clients.
// With nil ctrlClient, it panics. We skip this test since it requires
// a real k8s client to function properly.

// --- handleDiagnostics Tests ---
// handleDiagnostics requires a real k8s client (ctrlClient.List), so we
// can only test method validation here. The full handler needs a controller
// runtime client which isn't available in unit tests without a fake client.

// --- handleToolList Tests ---

func TestHandleToolList(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tools", nil)

	s.handleToolList(w, r)

	// Should return 200 with empty list or some response
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- helper ---

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testServer() *Server {
	return &Server{
		log:   testLogger(),
		cache: newResponseCache(10 * time.Minute),
	}
}

// --- parseCORSOrigins Tests (supplemental) ---

func TestParseCORSOrigins_EdgeCases(t *testing.T) {
	// Only commas
	got := parseCORSOrigins(" , , ")
	if len(got) != 0 {
		t.Errorf("only commas: got %v, want empty", got)
	}

	// Multiple with spaces
	got = parseCORSOrigins(" https://a.com , https://b.com ")
	if len(got) != 2 {
		t.Errorf("with spaces: got %v (len %d), want len 2", got, len(got))
	}
}

// --- SetTLS / IsTLS Tests (supplements tls_test.go) ---

func TestSetTLS_IsTLS_RoundTrip(t *testing.T) {
	s := &Server{}

	if s.IsTLS() {
		t.Error("expected IsTLS=false before SetTLS")
	}

	s.SetTLS("cert.pem", "key.pem")
	if !s.IsTLS() {
		t.Error("expected IsTLS=true after SetTLS")
	}
}

// --- cacheMiddleware Tests ---

func TestCacheMiddleware_BasicCaching(t *testing.T) {
	s := testServer()

	callCount := 0
	handler := s.cacheMiddleware(5*time.Minute, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeJSON(w, map[string]int{"count": callCount})
	})

	// First call — should invoke handler
	w1 := httptest.NewRecorder()
	r1 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler(w1, r1)
	if callCount != 1 {
		t.Errorf("after first call, callCount = %d, want 1", callCount)
	}

	// Second call — should serve from cache (same URL+method)
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler(w2, r2)
	if callCount != 1 {
		t.Errorf("after cached call, callCount = %d, want 1 (served from cache)", callCount)
	}
}

// --- SetTLS / IsTLS Tests (supplement existing tls_test.go) ---

func TestSetTLS_IsTLS_InitialState(t *testing.T) {
	s := &Server{}
	if s.IsTLS() {
		t.Error("expected IsTLS=false on new Server")
	}
}
