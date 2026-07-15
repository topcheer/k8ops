package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ThreatTimelineResult is the security event timeline & threat detection pattern audit.
type ThreatTimelineResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         ThreatSummary   `json:"summary"`
	Events          []ThreatEvent   `json:"events"`
	ByNamespace     []ThreatNSStat  `json:"byNamespace"`
	Patterns        []ThreatPattern `json:"patterns"`
	Risks           []ThreatRisk    `json:"risks"`
	Recommendations []string        `json:"recommendations"`
	HealthScore     int             `json:"healthScore"`
}

// ThreatSummary aggregates security event metrics.
type ThreatSummary struct {
	TotalEvents      int `json:"totalEvents"`
	RBACChanges      int `json:"rbacChanges"`      // Role/ClusterRole/Binding create/update/delete
	AdmissionDenied  int `json:"admissionDenied"`  // admission webhook denials
	SecretAccess     int `json:"secretAccess"`     // secret access events
	ConfigMapChanges int `json:"configMapChanges"` // ConfigMap create/update
	Forbidden        int `json:"forbidden"`        // 403 forbidden events
	HighSeverity     int `json:"highSeverity"`     // events with high severity
	Last24h          int `json:"last24h"`
	Last1h           int `json:"last1h"`
}

// ThreatEvent describes a security-related event.
type ThreatEvent struct {
	Time      string `json:"time"`
	Namespace string `json:"namespace,omitempty"`
	Resource  string `json:"resource"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Type      string `json:"type"`     // rbac-change, admission-denied, secret-access, forbidden, config-change
	Severity  string `json:"severity"` // info, warning, critical
}

// ThreatNSStat per-namespace security event stats.
type ThreatNSStat struct {
	Namespace       string `json:"namespace"`
	TotalEvents     int    `json:"totalEvents"`
	RBACChanges     int    `json:"rbacChanges"`
	AdmissionDenied int    `json:"admissionDenied"`
	Forbidden       int    `json:"forbidden"`
	RiskLevel       string `json:"riskLevel"`
}

// ThreatPattern describes a recurring threat pattern.
type ThreatPattern struct {
	Pattern  string `json:"pattern"`
	Count    int    `json:"count"`
	Severity string `json:"severity"`
}

// ThreatRisk describes a security risk.
type ThreatRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleThreatTimeline audits security event timeline & threat detection patterns.
// GET /api/security/threat-timeline
func (s *Server) handleThreatTimeline(w http.ResponseWriter, r *http.Request) {
	result := ThreatTimelineResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	events, err := rc.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list events: %v", err))
		return
	}

	now := time.Now()
	nsStats := map[string]*ThreatNSStat{}
	patternMap := map[string]*ThreatPattern{}

	// Security-related event keywords
	rbacKeywords := []string{"role", "clusterrole", "rolebinding", "clusterrolebinding", "rbac"}
	deniedKeywords := []string{"denied", "rejected", "forbidden", "unauthorized"}
	secretKeywords := []string{"secret", "token", "credential"}
	configKeywords := []string{"configmap", "configuration", "config-map"}
	forbiddenKeywords := []string{"forbidden", "403", "unauthorized", "unauthenticated"}

	for _, evt := range events.Items {
		msgLower := strings.ToLower(evt.Message)
		reasonLower := strings.ToLower(evt.Reason)
		combined := msgLower + " " + reasonLower
		resLower := strings.ToLower(evt.InvolvedObject.Kind)

		// Classify event type
		var eventType string
		severity := "info"

		isRBAC := false
		for _, kw := range rbacKeywords {
			if strings.Contains(combined, kw) || strings.Contains(resLower, kw) {
				isRBAC = true
				eventType = "rbac-change"
				severity = "warning"
				break
			}
		}

		isDenied := false
		for _, kw := range deniedKeywords {
			if strings.Contains(combined, kw) {
				isDenied = true
				eventType = "admission-denied"
				severity = "warning"
				break
			}
		}

		isForbidden := false
		for _, kw := range forbiddenKeywords {
			if strings.Contains(combined, kw) {
				isForbidden = true
				if eventType == "" {
					eventType = "forbidden"
				}
				severity = "critical"
				break
			}
		}

		isSecret := false
		for _, kw := range secretKeywords {
			if strings.Contains(combined, kw) || strings.Contains(resLower, kw) {
				isSecret = true
				if eventType == "" {
					eventType = "secret-access"
				}
				severity = "warning"
				break
			}
		}

		isConfig := false
		for _, kw := range configKeywords {
			if strings.Contains(combined, kw) || strings.Contains(resLower, kw) {
				isConfig = true
				if eventType == "" {
					eventType = "config-change"
				}
				severity = "info"
				break
			}
		}

		if !isRBAC && !isDenied && !isForbidden && !isSecret && !isConfig {
			continue
		}

		result.Summary.TotalEvents++
		if isRBAC {
			result.Summary.RBACChanges++
		}
		if isDenied {
			result.Summary.AdmissionDenied++
		}
		if isForbidden {
			result.Summary.Forbidden++
		}
		if isSecret {
			result.Summary.SecretAccess++
		}
		if isConfig {
			result.Summary.ConfigMapChanges++
		}
		if severity == "critical" || severity == "warning" {
			result.Summary.HighSeverity++
		}

		// Time stats
		ts := evt.LastTimestamp.Time
		if ts.IsZero() {
			ts = evt.CreationTimestamp.Time
		}
		if now.Sub(ts) <= 24*time.Hour {
			result.Summary.Last24h++
		}
		if now.Sub(ts) <= 1*time.Hour {
			result.Summary.Last1h++
		}

		entry := ThreatEvent{
			Time:      ts.Format(time.RFC3339),
			Namespace: evt.Namespace,
			Resource:  fmt.Sprintf("%s/%s", evt.InvolvedObject.Kind, evt.InvolvedObject.Name),
			Reason:    evt.Reason,
			Message:   evt.Message,
			Type:      eventType,
			Severity:  severity,
		}
		result.Events = append(result.Events, entry)

		// Namespace stats
		ns := evt.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &ThreatNSStat{Namespace: ns, RiskLevel: "low"}
		}
		nsStats[ns].TotalEvents++
		if isRBAC {
			nsStats[ns].RBACChanges++
		}
		if isDenied {
			nsStats[ns].AdmissionDenied++
		}
		if isForbidden {
			nsStats[ns].Forbidden++
		}

		// Pattern tracking
		if patternMap[eventType] == nil {
			patternMap[eventType] = &ThreatPattern{Pattern: eventType, Severity: severity}
		}
		patternMap[eventType].Count++

		// Add risks for critical events
		if severity == "critical" {
			result.Risks = append(result.Risks, ThreatRisk{
				Namespace: ns,
				Issue:     fmt.Sprintf("Forbidden/unauthorized access attempt: %s", evt.Message),
				Severity:  "critical",
			})
		}
	}

	// Namespace risk levels
	for _, stat := range nsStats {
		if stat.Forbidden > 0 {
			stat.RiskLevel = "critical"
		} else if stat.AdmissionDenied > 2 {
			stat.RiskLevel = "high"
		} else if stat.RBACChanges > 0 {
			stat.RiskLevel = "medium"
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].TotalEvents > result.ByNamespace[j].TotalEvents
	})

	// Build patterns
	for _, p := range patternMap {
		result.Patterns = append(result.Patterns, *p)
	}
	sort.Slice(result.Patterns, func(i, j int) bool {
		return result.Patterns[i].Count > result.Patterns[j].Count
	})

	// Sort events by time (most recent first)
	sort.Slice(result.Events, func(i, j int) bool {
		return result.Events[i].Time > result.Events[j].Time
	})

	// Health score
	score := 100
	if result.Summary.Forbidden > 0 {
		score -= min(30, result.Summary.Forbidden*10)
	}
	if result.Summary.AdmissionDenied > 0 {
		score -= min(15, result.Summary.AdmissionDenied*3)
	}
	if result.Summary.RBACChanges > 5 {
		score -= min(15, (result.Summary.RBACChanges-5)*2)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.Forbidden > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d forbidden/unauthorized access attempt(s) — investigate potential attack vectors", result.Summary.Forbidden))
	}
	if result.Summary.RBACChanges > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d RBAC change event(s) — verify all permission changes are authorized", result.Summary.RBACChanges))
	}
	if result.Summary.AdmissionDenied > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d admission denial event(s) — check for policy violations or misconfigured workloads", result.Summary.AdmissionDenied))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"No security threats detected in recent events — cluster appears secure")
	}

	writeJSON(w, result)
}
