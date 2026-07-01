package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleQuickExec_MethodNotAllowed(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/exec", nil)

	s.handleQuickExec(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleQuickExec_EmptyCommand(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec", strings.NewReader(`{}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleQuickExec_NonKubectlRejected(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"rm -rf /"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_DeleteRejected(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl delete pods --all"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_InvalidJSON(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`not json`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleQuickExec_GetCommandAccepted(t *testing.T) {
	// This test verifies the security check passes for kubectl get.
	// Actual execution will fail in test env (no nsenter), but should
	// return 200 with error field (not 403).
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods"}`))

	s.handleQuickExec(w, r)

	// Should be 200 (with error in body) or 200 with output
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Either output or error should be present
	if resp["output"] == "" && resp["error"] == "" {
		t.Error("expected output or error in response")
	}
}
