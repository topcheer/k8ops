package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleAlertmanagerWebhook_MethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/webhooks/alertmanager", nil)
	rr := httptest.NewRecorder()

	s.handleAlertmanagerWebhook(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}

func TestHandleAlertmanagerWebhook_InvalidJSON(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/alertmanager",
		strings.NewReader("not json"))
	rr := httptest.NewRecorder()

	s.handleAlertmanagerWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleAlertmanagerWebhook_ValidPayload(t *testing.T) {
	s := &Server{}
	payload := `{
		"version": "4",
		"status": "firing",
		"receiver": "k8ops",
		"alerts": [
			{
				"status": "firing",
				"labels": {"alertname": "HighCPUUsage", "severity": "warning", "instance": "node-1"},
				"annotations": {"summary": "CPU above 80%"},
				"startsAt": "2024-01-01T00:00:00Z",
				"fingerprint": "abc123"
			},
			{
				"status": "resolved",
				"labels": {"alertname": "HighMemoryUsage", "severity": "critical", "instance": "node-2"},
				"annotations": {"summary": "Memory above 90%"},
				"startsAt": "2024-01-01T00:05:00Z",
				"endsAt": "2024-01-01T00:10:00Z",
				"fingerprint": "def456"
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/alertmanager",
		strings.NewReader(payload))
	rr := httptest.NewRecorder()

	s.handleAlertmanagerWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify response has expected fields
	var resp map[string]any
	if err := parseJSON(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["firing"].(float64) != 1 {
		t.Errorf("expected 1 firing, got %v", resp["firing"])
	}
	if resp["resolved"].(float64) != 1 {
		t.Errorf("expected 1 resolved, got %v", resp["resolved"])
	}

	// Should have investigation hints for firing alerts
	if _, ok := resp["investigation"]; !ok {
		t.Error("expected investigation field for firing alerts")
	}
}

func TestHandleAlertmanagerWebhook_EmptyAlerts(t *testing.T) {
	s := &Server{}
	payload := `{
		"version": "4",
		"status": "resolved",
		"receiver": "k8ops",
		"alerts": []
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks/alertmanager",
		strings.NewReader(payload))
	rr := httptest.NewRecorder()

	s.handleAlertmanagerWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := parseJSON(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["totalAlerts"].(float64) != 0 {
		t.Errorf("expected 0 alerts, got %v", resp["totalAlerts"])
	}
}

func TestBuildInvestigationHints_CPUAlert(t *testing.T) {
	alerts := []AlertSummary{
		{
			Name:     "HighCPUUsage",
			Severity: "warning",
			Status:   "firing",
			Instance: "node-1",
		},
	}
	hints := buildInvestigationHints(alerts)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0]["alert"] != "HighCPUUsage" {
		t.Errorf("expected alert name 'HighCPUUsage', got %q", hints[0]["alert"])
	}
	if !strings.Contains(hints[0]["api"], "capacity") {
		t.Error("expected API hint to contain 'capacity' for CPU alert")
	}
}

func TestBuildInvestigationHints_ResolvedIgnored(t *testing.T) {
	alerts := []AlertSummary{
		{Name: "Alert1", Status: "firing"},
		{Name: "Alert2", Status: "resolved"},
	}
	hints := buildInvestigationHints(alerts)
	if len(hints) != 1 {
		t.Errorf("expected 1 hint (firing only), got %d", len(hints))
	}
}

// parseJSON is a test helper.
func parseJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
