package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleQuickExec_PipeInjection(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods | cat"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("pipe injection: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_RedirectInjection(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods > /tmp/out"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("redirect injection: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_BacktickInjection(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods `+"`whoami`"+`"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("backtick injection: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_DollarInjection(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods $(whoami)"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("$() injection: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_SemicolonInjection(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods; rm -rf /"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("semicolon injection: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_AndInjection(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods && whoami"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("&& injection: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_SecretsBlocked(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get secrets"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("secrets access: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_ConfigMapsBlocked(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get configmaps"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("configmaps access: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_ServiceAccountsBlocked(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get serviceaccounts"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("serviceaccounts access: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_NewlineInjection(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl get pods\nrm -rf /"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("newline injection: status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleQuickExec_DescribeCommandAccepted(t *testing.T) {
	// kubectl describe should pass all security checks
	// (actual execution will fail in test env, but should return 200 not 403)
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/exec",
		strings.NewReader(`{"command":"kubectl describe node worker-1"}`))

	s.handleQuickExec(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("describe command: status = %d, want %d", w.Code, http.StatusOK)
	}
}
