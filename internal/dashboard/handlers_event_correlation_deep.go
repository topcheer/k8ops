package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EventCorrelationDeepResult performs deep correlation analysis of Kubernetes
// events to find causal chains: which event triggered which, and identify
// root causes of cascading failures.
type EventCorrelationDeepResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         EventCorrSummary   `json:"summary"`
	Correlations    []EventCorrelation `json:"correlations"`
	RootCauses      []EventRootCause   `json:"rootCauses"`
	ByKind          []EventCorrKind    `json:"byKind"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type EventCorrSummary struct {
	TotalEvents      int `json:"totalEvents"`
	WarningEvents    int `json:"warningEvents"`
	CorrelatedEvents int `json:"correlatedEvents"`
	RootCauseCount   int `json:"rootCauseCount"`
	CascadeChains    int `json:"cascadeChains"`
}

type EventCorrelation struct {
	Pattern    string   `json:"pattern"`
	Events     []string `json:"events"`
	Count      int      `json:"count"`
	Severity   string   `json:"severity"`
	Suggestion string   `json:"suggestion"`
}

type EventRootCause struct {
	Reason    string `json:"reason"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Count     int    `json:"count"`
	FirstSeen string `json:"firstSeen"`
	LastSeen  string `json:"lastSeen"`
	RootCause string `json:"rootCause"`
	Fix       string `json:"fix"`
}

type EventCorrKind struct {
	Kind     string `json:"kind"`
	Count    int    `json:"count"`
	Warnings int    `json:"warnings"`
}

// handleEventCorrelationDeep handles GET /api/operations/event-correlation-deep
func (s *Server) handleEventCorrelationDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := EventCorrelationDeepResult{ScannedAt: time.Now()}

	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})

	// Build patterns
	patternMap := make(map[string]*EventCorrelation)
	reasonMap := make(map[string][]*corev1.Event)
	kindMap := make(map[string]*EventCorrKind)

	for i := range events.Items {
		ev := &events.Items[i]
		if isSystemNamespace(ev.Namespace) {
			continue
		}
		result.Summary.TotalEvents++
		isWarning := ev.Type == "Warning"
		if isWarning {
			result.Summary.WarningEvents++
		}

		// Kind stats
		kindStr := ev.InvolvedObject.Kind
		if _, ok := kindMap[kindStr]; !ok {
			kindMap[kindStr] = &EventCorrKind{Kind: kindStr}
		}
		kindMap[kindStr].Count++
		if isWarning {
			kindMap[kindStr].Warnings++
		}

		// Group by reason for root cause analysis
		reasonKey := ev.Reason
		if reasonKey == "" {
			reasonKey = "Unknown"
		}
		reasonMap[reasonKey] = append(reasonMap[reasonKey], ev)

		// Build correlation patterns (reason -> kind combinations)
		patternKey := ev.Reason + "->" + ev.InvolvedObject.Kind
		if _, ok := patternMap[patternKey]; !ok {
			patternMap[patternKey] = &EventCorrelation{
				Pattern:  fmt.Sprintf("%s on %s", ev.Reason, ev.InvolvedObject.Kind),
				Events:   []string{},
				Severity: "medium",
			}
			if isWarning {
				patternMap[patternKey].Severity = "high"
			}
			patternMap[patternKey].Suggestion = correlationSuggestion(ev.Reason)
		}
		patternMap[patternKey].Count++
		if len(patternMap[patternKey].Events) < 3 {
			patternMap[patternKey].Events = append(patternMap[patternKey].Events,
				fmt.Sprintf("%s/%s", ev.Namespace, ev.InvolvedObject.Name))
		}
	}

	// Build correlations
	for _, pc := range patternMap {
		if pc.Count >= 2 {
			result.Summary.CorrelatedEvents += pc.Count
			result.Correlations = append(result.Correlations, *pc)
		}
	}
	sort.Slice(result.Correlations, func(i, j int) bool {
		return result.Correlations[i].Count > result.Correlations[j].Count
	})
	if len(result.Correlations) > 20 {
		result.Correlations = result.Correlations[:20]
	}

	// Root causes
	for reason, evs := range reasonMap {
		if len(evs) < 3 {
			continue
		}
		var firstSeen, lastSeen time.Time
		ns := ""
		kindStr := ""
		for _, ev := range evs {
			if firstSeen.IsZero() || ev.FirstTimestamp.Time.Before(firstSeen) {
				firstSeen = ev.FirstTimestamp.Time
			}
			if ev.LastTimestamp.Time.After(lastSeen) {
				lastSeen = ev.LastTimestamp.Time
			}
			if ns == "" {
				ns = ev.Namespace
				kindStr = ev.InvolvedObject.Kind
			}
		}

		rc := EventRootCause{
			Reason: reason, Kind: kindStr, Namespace: ns,
			Count:     len(evs),
			FirstSeen: firstSeen.Format("01-02 15:04"),
			LastSeen:  lastSeen.Format("01-02 15:04"),
			RootCause: rootCauseText(reason),
			Fix:       correlationSuggestion(reason),
		}
		result.RootCauses = append(result.RootCauses, rc)
		result.Summary.RootCauseCount++
	}
	sort.Slice(result.RootCauses, func(i, j int) bool {
		return result.RootCauses[i].Count > result.RootCauses[j].Count
	})
	if len(result.RootCauses) > 15 {
		result.RootCauses = result.RootCauses[:15]
	}

	result.Summary.CascadeChains = len(result.Correlations)

	// Kind stats
	for _, k := range kindMap {
		result.ByKind = append(result.ByKind, *k)
	}
	sort.Slice(result.ByKind, func(i, j int) bool {
		return result.ByKind[i].Count > result.ByKind[j].Count
	})

	// Score
	if result.Summary.TotalEvents > 0 {
		correlationRate := result.Summary.CorrelatedEvents * 100 / result.Summary.TotalEvents
		result.HealthScore = 100 - correlationRate/3
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildEventCorrRecs(&result)
	writeJSON(w, result)
}

func correlationSuggestion(reason string) string {
	suggestions := map[string]string{
		"FailedScheduling": "Add worker nodes or reduce pod resource requests",
		"BackOff":          "Verify image exists and registry credentials are valid",
		"Unhealthy":        "Review probe configuration and application startup",
		"FailedMount":      "Verify referenced Secret/ConfigMap/PVC exists",
		"Evicted":          "Check node pressure (disk, memory) and resource limits",
		"FailedSync":       "Check controller logs and CRD status",
		"PolicyViolation":  "Review PSA and admission webhook policies",
		"Pulling":          "Check image registry availability and network",
	}
	if s, ok := suggestions[reason]; ok {
		return s
	}
	return "Investigate event details with kubectl describe"
}

func rootCauseText(reason string) string {
	causes := map[string]string{
		"FailedScheduling": "Insufficient cluster resources or scheduling constraints",
		"BackOff":          "Container image pull failure or application crash loop",
		"Unhealthy":        "Liveness/readiness probe failing",
		"FailedMount":      "Volume mount failure due to missing resource",
		"Evicted":          "Node resource pressure (kubelet eviction)",
		"PolicyViolation":  "Pod Security Admission or policy controller rejection",
	}
	if c, ok := causes[reason]; ok {
		return c
	}
	return "Multiple events suggest systematic issue"
}

func buildEventCorrRecs(r *EventCorrelationDeepResult) []string {
	recs := []string{
		fmt.Sprintf("事件关联: %d 个关联模式, %d 个根因", r.Summary.CascadeChains, r.Summary.RootCauseCount),
	}
	if r.Summary.CorrelatedEvents > 0 {
		rate := r.Summary.CorrelatedEvents * 100 / maxInt2(r.Summary.TotalEvents, 1)
		recs = append(recs, fmt.Sprintf("%d%% 事件有关联性 (可能级联)", rate))
	}
	for _, rc := range r.RootCauses {
		if rc.Count > 5 {
			recs = append(recs, fmt.Sprintf("[%s] %s: %s -> %s", rc.Kind, rc.Reason, rc.RootCause, rc.Fix))
			break
		}
	}
	return recs
}

func maxInt2(a, b int) int {
	if a > b {
		return a
	}
	return b
}
