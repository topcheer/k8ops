package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ggai/k8ops/internal/auth"
)

// === Diagnostics handler tests ===

func TestHandleDiagnostics_NilClient(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)

	s.handleDiagnostics(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleDiagnosticsHistory_NilClient(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/diagnostics/history", nil)

	s.handleDiagnosticsHistory(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleDiagnosticDetail_BadPath(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/diagnostics/", nil)

	s.handleDiagnosticDetail(w, r)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// === Remediations handler tests ===

func TestHandleRemediations_NilClient(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/remediation", nil)

	s.handleRemediations(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleRemediationAction_WrongMethod(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/remediation/default/plan1/approve", nil)

	s.handleRemediationAction(w, r)

	if w.Code != 405 {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleRemediationAction_BadPath(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/remediation/default", nil)

	s.handleRemediationAction(w, r)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleRemediationAction_InvalidAction(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/remediation/default/plan1/execute", nil)

	s.handleRemediationAction(w, r)

	if w.Code != 400 {
		t.Errorf("status = %d, want 400 for invalid action", w.Code)
	}
}

// === Optimizations handler tests ===

func TestHandleOptimizations_NilClient(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/optimizations", nil)

	s.handleOptimizations(w, r)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// === Chat handler tests ===

func TestHandleChat_NilEngine(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"message":"hi"}`))

	s.handleChat(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when chatEngine is nil", w.Code)
	}
}

func TestHandleChat_WrongMethod(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/chat", nil)

	s.handleChat(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when chatEngine is nil", w.Code)
	}
}

func TestHandleChat_InvalidBody(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`invalid json`))

	s.handleChat(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when chatEngine is nil", w.Code)
	}
}

func TestHandleConversations_NilEngine(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)

	s.handleConversations(w, r)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	convs, ok := resp["conversations"].([]any)
	if !ok {
		t.Fatal("conversations should be an array")
	}
	if len(convs) != 0 {
		t.Errorf("conversations len = %d, want 0 when engine is nil", len(convs))
	}
}

// handleConversations DELETE path requires non-nil chatEngine — tested in chat package

func TestBuildImpersonatedRegistry_NilClientTool(t *testing.T) {
	s := &Server{log: testLogger()}
	r := httptest.NewRequest(http.MethodPost, "/api/chat", nil)

	registry := s.buildImpersonatedRegistry(r)

	if registry != nil {
		t.Error("expected nil registry when no impersonation configured")
	}
}

// === Provider handler tests ===

func TestHandleProviderStatus_NilManager(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/provider/status", nil)

	s.handleProviderStatus(w, r)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["active"].(bool) != false {
		t.Errorf("active = %v, want false when providerMgr is nil", resp["active"])
	}
}

func TestHandleProviderUpdate_NilManager(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/provider/update",
		strings.NewReader(`{"type":"openai","model":"gpt-4","apiKey":"sk-xxx"}`))

	s.handleProviderUpdate(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when providerMgr is nil", w.Code)
	}
}

func TestHandleProviderUpdate_WrongMethod(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/provider/update", nil)

	s.handleProviderUpdate(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when providerMgr is nil", w.Code)
	}
}

func TestHandleProviderUpdate_InvalidBody(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/provider/update",
		strings.NewReader(`invalid json`))

	s.handleProviderUpdate(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when providerMgr is nil", w.Code)
	}
}

func TestHandleProviderReload_NilManager(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/provider/reload", nil)

	s.handleProviderReload(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 when providerMgr is nil", w.Code)
	}
}

// === Tool list handler tests ===

func TestHandleToolList_NilK8sClient(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/tools", nil)

	s.handleToolList(w, r)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	count, ok := resp["count"].(float64)
	if !ok {
		t.Fatal("count should be a number")
	}
	if count < 2 {
		t.Errorf("count = %v, want >= 2 (host tools always registered)", count)
	}
}

// === providerConfigFromRequest helper tests ===

func TestProviderConfigFromRequest_Defaults(t *testing.T) {
	cfg := providerConfigFromRequest("openai", "gpt-4", "sk-xxx", "", 0, 0)

	if cfg.Type != "openai" {
		t.Errorf("Type = %q, want 'openai'", cfg.Type)
	}
	if cfg.Model != "gpt-4" {
		t.Errorf("Model = %q, want 'gpt-4'", cfg.Model)
	}
	if cfg.APIKey != "sk-xxx" {
		t.Errorf("APIKey = %q, want 'sk-xxx'", cfg.APIKey)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096 (default)", cfg.MaxTokens)
	}
}

func TestProviderConfigFromRequest_CustomValues(t *testing.T) {
	cfg := providerConfigFromRequest("deepseek", "deepseek-chat", "sk-yyy",
		"https://api.deepseek.com", 8192, 0.5)

	if cfg.Endpoint != "https://api.deepseek.com" {
		t.Errorf("Endpoint = %q, want custom endpoint", cfg.Endpoint)
	}
	if cfg.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", cfg.Temperature)
	}
}

// === Additional handler edge case tests ===

// handleClusterOverview panics on nil clientset — can't test without fake client

// handleConfig panics on nil ctrlClient — same for handleNodes/Events/Pods (nil clientset)

// handleNodes panics on nil clientset

// handleEvents panics on nil clientset

// handlePods panics on nil clientset

func TestUserName_NoUser(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	if name := userName(r); name != "unknown" {
		t.Errorf("userName = %q, want 'unknown'", name)
	}
}

func TestHandleChat_WithUserContext(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"message":"hi"}`))

	ctx := auth.SetUserInContext(r.Context(), &auth.User{Username: "testuser", Role: "admin"})
	r = r.WithContext(ctx)

	s.handleChat(w, r)

	if w.Code != 503 {
		t.Errorf("status = %d, want 503 (chatEngine nil check first)", w.Code)
	}
}
