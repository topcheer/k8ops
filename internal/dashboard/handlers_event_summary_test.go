package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleEventSummary_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/events/summary", nil)
	rr := httptest.NewRecorder()

	s.handleEventSummary(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}
