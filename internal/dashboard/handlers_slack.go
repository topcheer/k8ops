package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ggai/k8ops/internal/auth"
)

// --- Slack Webhook ---

// SlackWebhookPayload is the JSON body accepted by POST /api/webhooks/slack.
type SlackWebhookPayload struct {
	Type    string `json:"type"`    // diagnostic_complete, remediation_executed, anomaly_event
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// slackWebhookURL is the configured webhook URL (from SLACK_WEBHOOK_URL env var).
var slackWebhookURL string

func init() {
	slackWebhookURL = os.Getenv("SLACK_WEBHOOK_URL")
}

func (s *Server) handleSlackWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, "method not allowed")
		return
	}

	// Defense in depth: adminOnlyMiddleware already checks, but verify again
	user := auth.UserFromRequest(r)
	if user == nil || user.Role != "admin" {
		writeError(w, 403, "admin role required")
		return
	}

	if slackWebhookURL == "" {
		writeError(w, 503, "SLACK_WEBHOOK_URL not configured")
		return
	}

	var payload SlackWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, 400, "invalid request body")
		return
	}
	if payload.Message == "" {
		writeError(w, 400, "message is required")
		return
	}

	// Build Slack message blocks
	slackMsg := map[string]any{
		"text": payload.Message,
		"blocks": []map[string]any{
			{
				"type": "header",
				"text": map[string]any{
					"type": "plain_text",
					"text": payload.Type,
				},
			},
			{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": payload.Message,
				},
			},
		},
	}

	// Add details as a code block if present
	if payload.Details != nil {
		detailsJSON, err := json.MarshalIndent(payload.Details, "", "  ")
		if err != nil {
			s.log.Warn("failed to marshal slack details, skipping", "error", err)
		} else {
			slackMsg["blocks"] = append(slackMsg["blocks"].([]map[string]any), map[string]any{
				"type": "section",
				"text": map[string]any{
					"type": "mrkdwn",
					"text": fmt.Sprintf("```json\n%s\n```", string(detailsJSON)),
				},
			})
		}
	}

	body, err := json.Marshal(slackMsg)
	if err != nil {
		s.log.Error("failed to marshal slack message", "error", err)
		writeError(w, 500, "failed to build slack message")
		return
	}

	// Use a client with timeout to avoid blocking if Slack API is slow/down
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(slackWebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		s.log.Error("slack webhook request failed", "error", err)
		writeError(w, 502, "failed to send slack notification")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		s.log.Error("slack webhook returned non-2xx",
			"status", resp.StatusCode,
			"body", string(respBody),
		)
		writeError(w, 502, fmt.Sprintf("slack webhook returned %d", resp.StatusCode))
		return
	}

	s.log.Info("slack notification sent",
		"type", payload.Type,
		"status", resp.StatusCode,
	)
	writeJSON(w, map[string]any{"status": "sent", "type": payload.Type})
}

// slackLogHelper returns a slog.Logger that can be used in tests.
// Kept for backward compatibility — the Server already has a log field.
func slackLogHelper(log *slog.Logger) *slog.Logger {
	if log == nil {
		return slog.Default()
	}
	return log
}
