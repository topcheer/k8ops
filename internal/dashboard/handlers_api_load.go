package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APILoadResult is the API server request throughput & load pressure analysis.
type APILoadResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         APILoadSummary       `json:"summary"`
	ByNamespace     []APILoadNSEntry     `json:"byNamespace"`
	HighActivityNS  []APILoadNSEntry     `json:"highActivityNS"`
	ControllerPods  []ControllerPodEntry `json:"controllerPods"`
	Recommendations []string             `json:"recommendations"`
}

// APILoadSummary aggregates API server load statistics.
type APILoadSummary struct {
	TotalNamespaces  int `json:"totalNamespaces"`
	TotalPods        int `json:"totalPods"`
	TotalControllers int `json:"totalControllers"` // pods with controller owner
	HighActivityNS   int `json:"highActivityNS"`   // namespaces with >50 pods or >10 controllers
	DenseNamespaces  int `json:"denseNamespaces"`  // >100 pods
	EmptyNamespaces  int `json:"emptyNamespaces"`  // 0 pods
	TotalEvents      int `json:"totalEvents"`
	WarningEvents    int `json:"warningEvents"`
	HealthScore      int `json:"healthScore"`
}

// APILoadNSEntry describes one namespace's API load profile.
type APILoadNSEntry struct {
	Namespace       string `json:"namespace"`
	PodCount        int    `json:"podCount"`
	ControllerCount int    `json:"controllerCount"`
	EventCount      int    `json:"eventCount"`
	WarningCount    int    `json:"warningCount"`
	ActivityLevel   string `json:"activityLevel"` // low, medium, high, critical
	RiskLevel       string `json:"riskLevel"`
}

// ControllerPodEntry identifies a controller-type pod.
type ControllerPodEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // Deployment, StatefulSet, etc.
}

// apiLoadAuditCore performs the analysis on pods and events (testable).
func apiLoadAuditCore(pods []corev1.Pod, events []corev1.Event) APILoadResult {
	result := APILoadResult{
		ScannedAt: time.Now(),
	}

	// Count pods and controllers per namespace
	podCountByNS := make(map[string]int)
	controllerCountByNS := make(map[string]int)
	var controllers []ControllerPodEntry

	for i := range pods {
		pod := &pods[i]
		ns := pod.Namespace
		podCountByNS[ns]++

		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "ReplicaSet" || ref.Kind == "Deployment" ||
				ref.Kind == "StatefulSet" || ref.Kind == "DaemonSet" ||
				ref.Kind == "Job" || ref.Kind == "CronJob" {
				controllerCountByNS[ns]++
				controllers = append(controllers, ControllerPodEntry{
					Name:      pod.Name,
					Namespace: ns,
					Kind:      ref.Kind,
				})
				break
			}
		}
	}

	// Count events per namespace
	eventCountByNS := make(map[string]int)
	warningCountByNS := make(map[string]int)
	totalEvents := 0
	warningEvents := 0

	for i := range events {
		ev := &events[i]
		ns := ev.Namespace
		eventCountByNS[ns]++
		totalEvents++
		if ev.Type == "Warning" {
			warningCountByNS[ns]++
			warningEvents++
		}
	}

	// Collect all namespaces
	allNS := make(map[string]bool)
	for ns := range podCountByNS {
		allNS[ns] = true
	}
	for ns := range eventCountByNS {
		allNS[ns] = true
	}

	result.Summary.TotalNamespaces = len(allNS)
	result.Summary.TotalPods = len(pods)
	result.Summary.TotalControllers = len(controllers)
	result.Summary.TotalEvents = totalEvents
	result.Summary.WarningEvents = warningEvents

	// Build namespace entries
	for ns := range allNS {
		podCount := podCountByNS[ns]
		ctrlCount := controllerCountByNS[ns]
		evCount := eventCountByNS[ns]
		warnCount := warningCountByNS[ns]

		entry := APILoadNSEntry{
			Namespace:       ns,
			PodCount:        podCount,
			ControllerCount: ctrlCount,
			EventCount:      evCount,
			WarningCount:    warnCount,
		}

		// Activity level based on pod count + event count
		activityScore := podCount + evCount/10
		switch {
		case activityScore > 150 || warnCount > 20:
			entry.ActivityLevel = "critical"
			entry.RiskLevel = "critical"
			result.Summary.HighActivityNS++
			if podCount > 100 {
				result.Summary.DenseNamespaces++
			}
			result.HighActivityNS = append(result.HighActivityNS, entry)
		case activityScore > 80 || warnCount > 10:
			entry.ActivityLevel = "high"
			entry.RiskLevel = "high"
			result.Summary.HighActivityNS++
			result.HighActivityNS = append(result.HighActivityNS, entry)
		case activityScore > 20:
			entry.ActivityLevel = "medium"
			entry.RiskLevel = "medium"
		default:
			entry.ActivityLevel = "low"
			entry.RiskLevel = "low"
			if podCount == 0 {
				result.Summary.EmptyNamespaces++
			}
		}

		result.ByNamespace = append(result.ByNamespace, entry)
	}

	// Sort by activity (highest first)
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PodCount+result.ByNamespace[i].EventCount >
			result.ByNamespace[j].PodCount+result.ByNamespace[j].EventCount
	})

	sort.Slice(result.HighActivityNS, func(i, j int) bool {
		return result.HighActivityNS[i].WarningCount > result.HighActivityNS[j].WarningCount
	})

	result.ControllerPods = controllers
	result.Summary.HealthScore = apiLoadScore(result.Summary)
	result.Recommendations = apiLoadRecommendations(result.Summary)

	return result
}

// apiLoadScore calculates health score.
func apiLoadScore(s APILoadSummary) int {
	base := 100
	if s.TotalNamespaces == 0 {
		return 100
	}
	// Dense namespaces put pressure on API server
	base -= s.DenseNamespaces * 10
	// High activity namespaces
	base -= s.HighActivityNS * 5
	// Warning events ratio penalty
	if s.TotalEvents > 0 {
		warnRatio := float64(s.WarningEvents) / float64(s.TotalEvents)
		if warnRatio > 0.3 {
			base -= 15
		} else if warnRatio > 0.1 {
			base -= 5
		}
	}
	// Empty namespaces waste API watch resources
	base -= s.EmptyNamespaces * 2

	if base < 0 {
		base = 0
	}
	return base
}

// apiLoadRecommendations generates recommendations.
func apiLoadRecommendations(s APILoadSummary) []string {
	var recs []string
	if s.DenseNamespaces > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have >100 pods — consider splitting or adding resource quotas to reduce API server watch pressure", s.DenseNamespaces))
	}
	if s.HighActivityNS > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have high API activity — monitor controller reconciliation loops and event storms", s.HighActivityNS))
	}
	if s.WarningEvents > 0 && s.TotalEvents > 0 {
		pct := float64(s.WarningEvents) / float64(s.TotalEvents) * 100
		recs = append(recs, fmt.Sprintf("%.0f%% of events are warnings (%d/%d) — investigate recurring warning patterns", pct, s.WarningEvents, s.TotalEvents))
	}
	if s.EmptyNamespaces > 0 {
		recs = append(recs, fmt.Sprintf("%d empty namespaces detected — clean up unused namespaces to reduce API server watch overhead", s.EmptyNamespaces))
	}
	if s.DenseNamespaces == 0 && s.HighActivityNS == 0 {
		recs = append(recs, "API server load is well distributed — no pressure hotspots detected")
	}
	return recs
}

// handleAPILoad audits API server request throughput & load pressure.
// GET /api/operations/api-load
func (s *Server) handleAPILoad(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})

	result := apiLoadAuditCore(pods.Items, events.Items)
	writeJSON(w, result)
}

// Suppress unused import
var _ = strings.Contains
