package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleComplianceScan_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/security/compliance", nil)
	rr := httptest.NewRecorder()

	s.handleComplianceScan(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestHandleComplianceReport_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/security/compliance/report", nil)
	rr := httptest.NewRecorder()

	s.handleComplianceReport(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}
