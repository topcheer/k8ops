package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleBackupList_NoDir(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/backup", nil)
	rr := httptest.NewRecorder()

	s.handleBackupList(rr, req)

	// Should return 200 with empty list (directory doesn't exist in test env)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleBackupDispatch_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPut, "/api/system/backup", nil)
	rr := httptest.NewRecorder()

	s.handleBackupDispatch(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleBackupDispatch_GetList(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/system/backup", nil)
	rr := httptest.NewRecorder()

	s.handleBackupDispatch(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleBackupDelete_NoName(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodDelete, "/api/system/backup", nil)
	rr := httptest.NewRecorder()

	s.handleBackupDelete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleBackupRestore_NoName(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/system/backup/restore", nil)
	rr := httptest.NewRecorder()

	s.handleBackupRestore(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
