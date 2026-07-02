package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleScale_MethodNotAllowed(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/scale", nil)
	s.handleScale(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleScale_EmptyFields(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/scale",
		strings.NewReader(`{"namespace":"","name":"","replicas":3}`))
	s.handleScale(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleScale_InvalidReplicas(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/scale",
		strings.NewReader(`{"namespace":"default","name":"nginx","replicas":-1}`))
	s.handleScale(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleScale_ReplicasTooMany(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/scale",
		strings.NewReader(`{"namespace":"default","name":"nginx","replicas":9999}`))
	s.handleScale(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleScale_InvalidKind(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/scale",
		strings.NewReader(`{"namespace":"default","name":"nginx","kind":"pod","replicas":3}`))
	s.handleScale(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleScale_InvalidJSON(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/scale",
		strings.NewReader(`not json`))
	s.handleScale(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlePodDelete_MethodNotAllowed(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/pod/delete", nil)
	s.handlePodDelete(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlePodDelete_EmptyFields(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/pod/delete",
		strings.NewReader(`{"namespace":"","name":""}`))
	s.handlePodDelete(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandlePodDelete_InvalidJSON(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/pod/delete",
		strings.NewReader(`bad`))
	s.handlePodDelete(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRolloutRestart_MethodNotAllowed(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/rollout/restart", nil)
	s.handleRolloutRestart(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleRolloutRestart_EmptyFields(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/rollout/restart",
		strings.NewReader(`{"namespace":"","name":"","kind":"deployment"}`))
	s.handleRolloutRestart(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRolloutRestart_InvalidKind(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/rollout/restart",
		strings.NewReader(`{"namespace":"default","name":"nginx","kind":"pod"}`))
	s.handleRolloutRestart(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRolloutRestart_InvalidJSON(t *testing.T) {
	s := &Server{log: testLogger()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/rollout/restart",
		strings.NewReader(`not json`))
	s.handleRolloutRestart(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
