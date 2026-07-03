package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/ggai/k8ops/internal/audit"
)

// AlertmanagerWebhook is the payload format sent by Prometheus Alertmanager.
// See: https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
type AlertmanagerWebhook struct {
	Version           string  `json:"version"`
	GroupKey          string  `json:"groupKey"`
	TruncatedAlerts   int     `json:"truncatedAlerts"`
	Status            string  `json:"status"`
	Receiver          string  `json:"receiver"`
	GroupLabels       KV      `json:"groupLabels"`
	CommonLabels      KV      `json:"commonLabels"`
	CommonAnnotations KV      `json:"commonAnnotations"`
	ExternalURL       string  `json:"externalURL"`
	Alerts            []Alert `json:"alerts"`
}

// Alert represents a single alert from Alertmanager.
type Alert struct {
	Status       string    `json:"status"`
	Labels       KV        `json:"labels"`
	Annotations  KV        `json:"annotations"`
	StartsAt     time.Time `json:"startsAt"`
	EndsAt       time.Time `json:"endsAt"`
	GeneratorURL string    `json:"generatorURL"`
	Fingerprint  string    `json:"fingerprint"`
}

// KV is a key-value map used by Alertmanager for labels and annotations.
type KV map[string]string

// AlertSummary is a simplified alert representation for the dashboard.
type AlertSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	Instance    string `json:"instance"`
	Namespace   string `json:"namespace"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	StartsAt    string `json:"startsAt"`
	Age         string `json:"age"`
	RunbookURL  string `json:"runbookURL"`
}

// handleAlertmanagerWebhook receives Prometheus Alertmanager alerts.
// POST /api/webhooks/alertmanager
//
// Alertmanager config example:
//
//	receivers:
//	  - name: k8ops
//	    webhook_configs:
//	      - url: http://k8ops.k8ops-system.svc:9090/api/webhooks/alertmanager
//	        send_resolved: true
func (s *Server) handleAlertmanagerWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Limit body size (Alertmanager payloads can be large)
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024) // 256KB

	var payload AlertmanagerWebhook
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	// Parse alerts into summaries
	summaries := make([]AlertSummary, 0, len(payload.Alerts))
	firingCount := 0
	resolvedCount := 0

	for _, alert := range payload.Alerts {
		summary := AlertSummary{
			ID:         alert.Fingerprint,
			Name:       alert.Labels["alertname"],
			Severity:   alert.Labels["severity"],
			Status:     alert.Status,
			Instance:   alert.Labels["instance"],
			Namespace:  alert.Labels["namespace"],
			Summary:    alert.Annotations["summary"],
			StartsAt:   alert.StartsAt.Format(time.RFC3339),
			Age:        ageTime(alert.StartsAt),
			RunbookURL: alert.Annotations["runbook_url"],
		}

		// Truncate description
		if desc, ok := alert.Annotations["description"]; ok {
			summary.Description = truncate(desc, 500)
		}

		if alert.Status == "firing" {
			firingCount++
		} else {
			resolvedCount++
		}

		summaries = append(summaries, summary)
	}

	// Log received alerts
	if s.log != nil {
		s.log.Info("alertmanager webhook received",
			"status", payload.Status,
			"firing", firingCount,
			"resolved", resolvedCount,
			"receiver", payload.Receiver,
			"alerts", len(payload.Alerts),
		)
	}

	// Log to audit trail
	if s.auditLog != nil {
		for _, sm := range summaries {
			sev := audit.SeverityInfo
			if sm.Severity == "critical" {
				sev = audit.SeverityCritical
			} else if sm.Severity == "warning" {
				sev = audit.SeverityWarning
			}
			s.auditLog.Log(r.Context(), audit.Event{
				Timestamp: time.Now().Format(time.RFC3339),
				Severity:  sev,
				Action:    "alert." + sm.Status,
				Target:    sm.Name,
				Success:   true,
				Detail: map[string]any{
					"alertname":   sm.Name,
					"severity":    sm.Severity,
					"instance":    sm.Instance,
					"namespace":   sm.Namespace,
					"summary":     sm.Summary,
					"fingerprint": sm.ID,
				},
			})
		}
	}

	// Build response with suggested investigation steps for firing alerts
	response := map[string]any{
		"received":    true,
		"timestamp":   time.Now().Format(time.RFC3339),
		"firing":      firingCount,
		"resolved":    resolvedCount,
		"totalAlerts": len(summaries),
		"alerts":      summaries,
	}

	// Add investigation hints for firing alerts
	if firingCount > 0 {
		response["investigation"] = buildInvestigationHints(summaries)
	}

	writeJSON(w, response)
}

// buildInvestigationHints generates suggested investigation steps based on alert types.
func buildInvestigationHints(alerts []AlertSummary) []map[string]string {
	hints := make([]map[string]string, 0)

	for _, a := range alerts {
		if a.Status != "firing" {
			continue
		}

		hint := map[string]string{
			"alert":    a.Name,
			"severity": a.Severity,
		}

		name := strings.ToLower(a.Name)

		switch {
		case strings.Contains(name, "cpu") || strings.Contains(name, "highcpu"):
			hint["action"] = "Check node resource usage: kubectl top nodes"
			hint["api"] = "/api/nodes and /api/capacity/planning"
		case strings.Contains(name, "memory") || strings.Contains(name, "oom"):
			hint["action"] = "Check pod memory: kubectl top pods --all-namespaces | sort --reverse --key 2"
			hint["api"] = "/api/namespaces/ranking"
		case strings.Contains(name, "disk") || strings.Contains(name, "storage") || strings.Contains(name, "pvc"):
			hint["action"] = "Check PVC capacity: kubectl get pvc --all-namespaces"
			hint["api"] = "/api/storage/capacity"
		case strings.Contains(name, "pod") && strings.Contains(name, "crash"):
			hint["action"] = "Check crash loops: kubectl get pods --all-namespaces --field-selector=status.phase=Failed"
			hint["api"] = "/api/pods?status=CrashLoopBackOff"
		case strings.Contains(name, "node") && (strings.Contains(name, "down") || strings.Contains(name, "notready")):
			hint["action"] = "Check node status: kubectl get nodes -o wide"
			hint["api"] = "/api/nodes"
		case strings.Contains(name, "certificate") || strings.Contains(name, "cert") || strings.Contains(name, "tls"):
			hint["action"] = "Check cert-manager: kubectl get certificates --all-namespaces"
			hint["api"] = "/api/resources?type=secrets"
		case strings.Contains(name, "connection") || strings.Contains(name, "timeout"):
			hint["action"] = "Check pod health and endpoints: kubectl get endpoints --all-namespaces"
			hint["api"] = "/api/pods"
		default:
			hint["action"] = "Investigate alert: kubectl describe events -n " + a.Namespace
			hint["api"] = "/api/events/summary"
		}

		if a.RunbookURL != "" {
			hint["runbook"] = a.RunbookURL
		}

		hints = append(hints, hint)
	}

	return hints
}

// handleAlertTest allows testing the alert receiver without Alertmanager.
// POST /api/webhooks/alertmanager/test
func (s *Server) handleAlertTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	testPayload := AlertmanagerWebhook{
		Version:  "4",
		Status:   "firing",
		Receiver: "k8ops",
		CommonLabels: KV{
			"alertname": "TestAlert",
			"severity":  "warning",
		},
		Alerts: []Alert{
			{
				Status: "firing",
				Labels: KV{
					"alertname": "HighCPUUsage",
					"severity":  "warning",
					"instance":  "node-1",
					"namespace": "kube-system",
				},
				Annotations: KV{
					"summary":     "CPU usage above 80% on node-1",
					"description": "Node node-1 has been above 80% CPU for 5 minutes",
				},
				StartsAt:    time.Now().Add(-5 * time.Minute),
				Fingerprint: "abc123def456",
			},
		},
	}

	// Re-marshal and process through the main handler
	body, _ := json.Marshal(testPayload)
	testReq := httptest.NewRequest(http.MethodPost, "/api/webhooks/alertmanager", strings.NewReader(string(body)))
	testRR := httptest.NewRecorder()

	s.handleAlertmanagerWebhook(testRR, testReq)

	if testRR.Code != http.StatusOK {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("test alert processing failed: %d", testRR.Code))
		return
	}

	writeJSON(w, map[string]any{
		"success":  true,
		"message":  "test alert processed successfully",
		"response": json.RawMessage(testRR.Body.Bytes()),
	})
}
