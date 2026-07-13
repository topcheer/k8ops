package dashboard

import (
	"net/http"
)

// handleAlertmanagerWebhook receives Prometheus Alertmanager webhook alerts.
func (s *Server) handleAlertmanagerWebhook(w http.ResponseWriter, r *http.Request) {
	// Stub implementation — alerts are logged but no action taken
	writeJSON(w, map[string]string{"status": "received"})
}

// handleAlertTest is a test endpoint for alert delivery verification.
func (s *Server) handleAlertTest(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok", "message": "alert test endpoint working"})
}
