package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AlertNoiseResult is the alert noise & fatigue detection analysis.
type AlertNoiseResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         AlertNoiseSummary     `json:"summary"`
	TopAlerts       []AlertNoiseEntry     `json:"topAlerts"`
	ByNamespace     []AlertNoiseNSStat    `json:"byNamespace"`
	ByLabel         []AlertNoiseLabelStat `json:"byLabel"`
	Issues          []AlertNoiseIssue     `json:"issues"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// AlertNoiseSummary aggregates alert noise statistics.
type AlertNoiseSummary struct {
	TotalAlertEvents int     `json:"totalAlertEvents"`
	UniqueAlertNames int     `json:"uniqueAlertNames"`
	NoisyAlerts      int     `json:"noisyAlerts"`    // >10 events in window
	FlappingAlerts   int     `json:"flappingAlerts"` // fires+resolves repeatedly
	StaleSilences    int     `json:"staleSilences"`  // silences >7d
	AlertStorms      int     `json:"alertStorms"`    // >20 events in 5min window
	TopNoiseSource   string  `json:"topNoiseSource"`
	NoiseRatio       float64 `json:"noiseRatio"` // noisy alerts / total
}

// AlertNoiseEntry describes one alert's noise pattern.
type AlertNoiseEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Severity      string `json:"severity"`
	EventCount    int    `json:"eventCount"`
	FiringCount   int    `json:"firingCount"`
	ResolvedCount int    `json:"resolvedCount"`
	FlapScore     int    `json:"flapScore"` // count of fire->resolve->fire transitions
	LastEventAt   string `json:"lastEventAt"`
	IsNoisy       bool   `json:"isNoisy"`    // >10 events
	IsFlapping    bool   `json:"isFlapping"` // flapScore > 3
	RiskLevel     string `json:"riskLevel"`
}

// AlertNoiseNSStat per-namespace alert noise stats.
type AlertNoiseNSStat struct {
	Namespace      string `json:"namespace"`
	TotalEvents    int    `json:"totalEvents"`
	NoisyAlerts    int    `json:"noisyAlerts"`
	FlappingAlerts int    `json:"flappingAlerts"`
}

// AlertNoiseLabelStat aggregates by alert label (severity, alertname).
type AlertNoiseLabelStat struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	Count   int    `json:"count"`
	Percent int    `json:"percent"`
}

// AlertNoiseIssue describes a specific alert noise problem.
type AlertNoiseIssue struct {
	Severity   string `json:"severity"`
	AlertName  string `json:"alertName"`
	Namespace  string `json:"namespace"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion"`
}

// handleAlertNoise handles GET /api/operations/alert-noise
// Detects alert noise patterns, flapping alerts, alert storms, and fatigue indicators.
func (s *Server) handleAlertNoise(w http.ResponseWriter, r *http.Request) {
	result := s.auditAlertNoise(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) auditAlertNoise(ctx context.Context) *AlertNoiseResult {
	result := &AlertNoiseResult{ScannedAt: time.Now()}

	// Collect alert events from Warning events that look like alert-related
	alertEvents := s.collectAlertEvents(ctx)

	// Also check Alertmanager ConfigMaps for silence rules
	silenceIssues := s.checkStaleSilences(ctx)

	// Build per-alert stats
	alertMap := map[string]*AlertNoiseEntry{}
	for _, ev := range alertEvents {
		key := fmt.Sprintf("%s/%s", ev.Namespace, ev.Name)
		entry, ok := alertMap[key]
		if !ok {
			entry = &AlertNoiseEntry{
				Name:      ev.Name,
				Namespace: ev.Namespace,
				Severity:  ev.Severity,
			}
			alertMap[key] = entry
		}
		entry.EventCount++
		if ev.IsFiring {
			entry.FiringCount++
		} else {
			entry.ResolvedCount++
		}
		entry.LastEventAt = ev.LastEventAt
	}

	// Assess noise patterns
	for _, entry := range alertMap {
		if entry.EventCount > 10 {
			entry.IsNoisy = true
			result.Summary.NoisyAlerts++
		}
		// Flap score = min(firing, resolved) transitions
		if entry.FiringCount > 3 && entry.ResolvedCount > 3 {
			entry.IsFlapping = true
			entry.FlapScore = entry.FiringCount
			if entry.ResolvedCount < entry.FlapScore {
				entry.FlapScore = entry.ResolvedCount
			}
			result.Summary.FlappingAlerts++
		}
		entry.RiskLevel = assessAlertNoiseRisk(*entry)
		result.TopAlerts = append(result.TopAlerts, *entry)
	}

	// Sort by event count descending
	sort.Slice(result.TopAlerts, func(i, j int) bool {
		return result.TopAlerts[i].EventCount > result.TopAlerts[j].EventCount
	})

	// Limit to top 50
	if len(result.TopAlerts) > 50 {
		result.TopAlerts = result.TopAlerts[:50]
	}

	result.Summary.TotalAlertEvents = len(alertEvents)
	result.Summary.UniqueAlertNames = len(alertMap)

	// Detect alert storms (time-bucketed)
	result.Summary.AlertStorms = s.detectAlertStorms(alertEvents)

	// Top noise source
	if len(result.TopAlerts) > 0 && result.TopAlerts[0].EventCount > 10 {
		result.Summary.TopNoiseSource = fmt.Sprintf("%s/%s (%d events)",
			result.TopAlerts[0].Namespace, result.TopAlerts[0].Name, result.TopAlerts[0].EventCount)
	}

	// Noise ratio
	if result.Summary.TotalAlertEvents > 0 {
		noisyEvents := 0
		for _, a := range result.TopAlerts {
			if a.IsNoisy {
				noisyEvents += a.EventCount
			}
		}
		result.Summary.NoiseRatio = float64(noisyEvents) / float64(result.Summary.TotalAlertEvents)
	}

	// Add silence issues
	result.Summary.StaleSilences = len(silenceIssues)

	// Build per-namespace stats
	nsMap := map[string]*AlertNoiseNSStat{}
	for _, entry := range result.TopAlerts {
		ns, ok := nsMap[entry.Namespace]
		if !ok {
			ns = &AlertNoiseNSStat{Namespace: entry.Namespace}
			nsMap[entry.Namespace] = ns
		}
		ns.TotalEvents += entry.EventCount
		if entry.IsNoisy {
			ns.NoisyAlerts++
		}
		if entry.IsFlapping {
			ns.FlappingAlerts++
		}
	}
	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalEvents > result.ByNamespace[j].TotalEvents
	})

	// Build by-label stats (severity)
	sevMap := map[string]int{}
	for _, entry := range result.TopAlerts {
		sevMap[entry.Severity] += entry.EventCount
	}
	for sev, count := range sevMap {
		pct := 0
		if result.Summary.TotalAlertEvents > 0 {
			pct = count * 100 / result.Summary.TotalAlertEvents
		}
		result.ByLabel = append(result.ByLabel, AlertNoiseLabelStat{
			Label:   "severity",
			Value:   sev,
			Count:   count,
			Percent: pct,
		})
	}
	sort.Slice(result.ByLabel, func(i, j int) bool {
		return result.ByLabel[i].Count > result.ByLabel[j].Count
	})

	// Generate issues
	for _, entry := range result.TopAlerts {
		if entry.IsFlapping {
			result.Issues = append(result.Issues, AlertNoiseIssue{
				Severity:   "critical",
				AlertName:  entry.Name,
				Namespace:  entry.Namespace,
				Issue:      fmt.Sprintf("Alert %s is flapping (%d fire/resolve cycles)", entry.Name, entry.FlapScore),
				Suggestion: "Investigate root cause — likely threshold too sensitive or underlying service unstable",
			})
		}
		if entry.IsNoisy && !entry.IsFlapping {
			result.Issues = append(result.Issues, AlertNoiseIssue{
				Severity:   "warning",
				AlertName:  entry.Name,
				Namespace:  entry.Namespace,
				Issue:      fmt.Sprintf("Alert %s generated %d events (noisy)", entry.Name, entry.EventCount),
				Suggestion: "Consider tuning alert threshold, adding alert grouping, or converting to recording rule",
			})
		}
	}
	for _, si := range silenceIssues {
		result.Issues = append(result.Issues, si)
	}

	// Recommendations
	if result.Summary.NoisyAlerts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d noisy alert(s) detected — tune thresholds or add grouping to reduce alert fatigue", result.Summary.NoisyAlerts))
	}
	if result.Summary.FlappingAlerts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d flapping alert(s) detected — investigate root cause and consider increasing for_duration", result.Summary.FlappingAlerts))
	}
	if result.Summary.AlertStorms > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d alert storm(s) detected — consider alert grouping and inhibition rules to reduce noise during incidents", result.Summary.AlertStorms))
	}
	if result.Summary.StaleSilences > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d stale silence(s) detected (>7d) — clean up expired silences to avoid masking alerts", result.Summary.StaleSilences))
	}
	if result.Summary.NoiseRatio > 0.5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Alert noise ratio %.0f%% — more than half of alert events come from noisy alerts, consider alert review", result.Summary.NoiseRatio*100))
	}
	if result.Summary.TotalAlertEvents == 0 {
		result.Recommendations = append(result.Recommendations,
			"No alert events detected — ensure Alertmanager and Prometheus are properly configured and exporting events")
	}

	// Health score
	score := 100
	score -= result.Summary.NoisyAlerts * 5
	score -= result.Summary.FlappingAlerts * 10
	score -= result.Summary.AlertStorms * 5
	score -= result.Summary.StaleSilences * 5
	if result.Summary.NoiseRatio > 0.5 {
		score -= 15
	} else if result.Summary.NoiseRatio > 0.3 {
		score -= 8
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	return result
}

// alertEventInfo represents a single alert event extracted from K8s events.
type alertEventInfo struct {
	Name        string
	Namespace   string
	Severity    string
	IsFiring    bool
	LastEventAt string
}

// collectAlertEvents scans Kubernetes Warning events for alert-like patterns.
func (s *Server) collectAlertEvents(ctx context.Context) []alertEventInfo {
	var events []alertEventInfo

	if s.clientset == nil {
		return events
	}

	// Get Warning events from the last 24h
	eventList, err := s.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: "type=Warning",
		Limit:         1000,
	})
	if err != nil {
		return events
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	for _, ev := range eventList.Items {
		// Filter to last 24h
		if ev.LastTimestamp.Time.Before(cutoff) {
			continue
		}

		// Extract alert-like events (those mentioning "alert", "firing", "resolved", "AlertManager")
		reason := ev.Reason
		msg := ev.Message
		name := ev.InvolvedObject.Name
		ns := ev.InvolvedObject.Namespace

		// Check if this looks like an alert notification event
		isAlert := false
		severity := "warning"
		isFiring := true

		lowerMsg := strings.ToLower(msg)
		lowerReason := strings.ToLower(reason)

		if strings.Contains(lowerReason, "alert") || strings.Contains(lowerMsg, "alert") {
			isAlert = true
		}
		if strings.Contains(lowerMsg, "firing") {
			isAlert = true
			isFiring = true
		}
		if strings.Contains(lowerMsg, "resolved") || strings.Contains(lowerMsg, "pending") {
			isAlert = true
			isFiring = false
		}
		if strings.Contains(lowerReason, "failed") || strings.Contains(lowerReason, "backoff") {
			isAlert = true
			severity = "critical"
		}
		if strings.Contains(lowerReason, "oom") || strings.Contains(lowerReason, "kill") {
			isAlert = true
			severity = "critical"
		}

		if !isAlert {
			continue
		}

		events = append(events, alertEventInfo{
			Name:        name,
			Namespace:   ns,
			Severity:    severity,
			IsFiring:    isFiring,
			LastEventAt: ev.LastTimestamp.Format(time.RFC3339),
		})
	}

	return events
}

// detectAlertStorms counts 5-minute windows with >20 alert events.
func (s *Server) detectAlertStorms(events []alertEventInfo) int {
	if len(events) == 0 {
		return 0
	}

	// Parse timestamps and sort
	type tsEvent struct {
		ts     time.Time
		firing bool
	}
	var tsEvents []tsEvent
	for _, ev := range events {
		if t, err := time.Parse(time.RFC3339, ev.LastEventAt); err == nil {
			tsEvents = append(tsEvents, tsEvent{ts: t, firing: ev.IsFiring})
		}
	}
	sort.Slice(tsEvents, func(i, j int) bool {
		return tsEvents[i].ts.Before(tsEvents[j].ts)
	})

	storms := 0
	windowStart := time.Time{}
	count := 0

	for _, te := range tsEvents {
		if te.ts.Sub(windowStart) > 5*time.Minute {
			if count > 20 {
				storms++
			}
			windowStart = te.ts
			count = 1
		} else {
			count++
		}
	}
	// Check last window
	if count > 20 {
		storms++
	}

	return storms
}

// checkStaleSilences looks for Alertmanager ConfigMaps with long-running silence rules.
func (s *Server) checkStaleSilences(ctx context.Context) []AlertNoiseIssue {
	var issues []AlertNoiseIssue

	if s.clientset == nil {
		return issues
	}

	// Look for Alertmanager config ConfigMaps
	cmList, err := s.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=alertmanager",
	})
	if err != nil {
		return issues
	}

	cutoff := time.Now().Add(-7 * 24 * time.Hour)

	for _, cm := range cmList.Items {
		for key, val := range cm.Data {
			if !strings.Contains(key, "silence") && !strings.Contains(strings.ToLower(val), "silence") {
				continue
			}
			// Check if silence has been active for >7d
			// This is a heuristic since we can't parse actual silence state from ConfigMap
			if cm.CreationTimestamp.Time.Before(cutoff) {
				issues = append(issues, AlertNoiseIssue{
					Severity:   "warning",
					AlertName:  key,
					Namespace:  cm.Namespace,
					Issue:      fmt.Sprintf("Stale silence rule in ConfigMap %s (older than 7d)", cm.Name),
					Suggestion: "Review and remove expired silence rules to avoid masking active alerts",
				})
			}
		}
	}

	return issues
}

// assessAlertNoiseRisk determines risk level for an alert noise entry.
func assessAlertNoiseRisk(entry AlertNoiseEntry) string {
	if entry.IsFlapping {
		return "critical"
	}
	if entry.IsNoisy {
		return "warning"
	}
	if entry.EventCount > 5 {
		return "info"
	}
	return "healthy"
}

// formatAlertNoiseSummary returns a human-readable summary for audit output.
func formatAlertNoiseSummary(r *AlertNoiseResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Alert noise audit: %d total events, %d unique alerts, %d noisy, %d flapping, %d storms",
		r.Summary.TotalAlertEvents, r.Summary.UniqueAlertNames,
		r.Summary.NoisyAlerts, r.Summary.FlappingAlerts, r.Summary.AlertStorms)
	if r.Summary.NoiseRatio > 0 {
		fmt.Fprintf(&b, " | noise ratio: %.0f%%", r.Summary.NoiseRatio*100)
	}
	return b.String()
}
