package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

func TestHandleHPAList_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/hpa", nil)
	rr := httptest.NewRecorder()

	s.handleHPAList(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestMetricStatusKey_Empty(t *testing.T) {
	// Empty metric status should return empty key
	got := metricStatusKey(autoscalingv2.MetricStatus{})
	if got != "" {
		t.Errorf("expected empty key for empty metric status, got %q", got)
	}
}

func TestMetricStatusKey_Resource(t *testing.T) {
	ms := autoscalingv2.MetricStatus{
		Type: autoscalingv2.ResourceMetricSourceType,
		Resource: &autoscalingv2.ResourceMetricStatus{
			Name: "cpu",
		},
	}
	got := metricStatusKey(ms)
	if got != "resource:cpu" {
		t.Errorf("expected 'resource:cpu', got %q", got)
	}
}
