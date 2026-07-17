package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChangeFreezeResult detects change-freeze windows and deployment risk during
// critical periods (holidays, peak traffic, maintenance windows). It evaluates
// cluster stability, active incidents, and workload criticality to determine
// if changes should be blocked or require approval.
type ChangeFreezeResult struct {
	ScannedAt        time.Time            `json:"scannedAt"`
	Summary          FreezeSummary        `json:"summary"`
	FreezeStatus     string               `json:"freezeStatus"` // active, upcoming, none
	CurrentRisk      string               `json:"currentRisk"`
	RecentChanges    []FreezeRecentChange `json:"recentChanges"`
	StabilitySignals []StabilitySignal    `json:"stabilitySignals"`
	FreezeWindows    []FreezeWindow       `json:"freezeWindows"`
	Verdict          string               `json:"verdict"` // proceed, caution, freeze
	Recommendations  []string             `json:"recommendations"`
}

// FreezeSummary aggregates change freeze statistics.
type FreezeSummary struct {
	ActiveIncidents   int     `json:"activeIncidents"`
	CrashLoopPods     int     `json:"crashLoopPods"`
	RecentDeployments int     `json:"recentDeployments24h"`
	FailedDeployments int     `json:"failedDeployments24h"`
	WarningEvents1h   int     `json:"warningEvents1h"`
	WarningEvents24h  int     `json:"warningEvents24h"`
	StabilityScore    int     `json:"stabilityScore"`
	AvgPodAge         float64 `json:"avgPodAgeHours"`
}

// RecentChange describes a recent deployment/change.
type FreezeRecentChange struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	AgeHours      int    `json:"ageHours"`
	Replicas      int    `json:"replicas"`
	ReadyReplicas int    `json:"readyReplicas"`
	Healthy       bool   `json:"healthy"`
}

// StabilitySignal describes one stability indicator.
type StabilitySignal struct {
	Name   string `json:"name"`
	Status string `json:"status"` // healthy, warning, critical
	Value  string `json:"value"`
	Impact string `json:"impact"`
}

// FreezeWindow describes a configured freeze period.
type FreezeWindow struct {
	Name   string `json:"name"`
	Starts string `json:"starts"`
	Ends   string `json:"ends"`
	Reason string `json:"reason"`
	Type   string `json:"type"`  // holiday, peak-traffic, maintenance
	Scope  string `json:"scope"` // cluster, namespace
}

// handleChangeFreeze handles GET /api/deployment/change-freeze
func (s *Server) handleChangeFreeze(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ChangeFreezeResult{ScannedAt: time.Now()}
	now := time.Now()

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Crash loops and pending pods
	crashLoop := 0
	pendingPods := 0
	totalPodAge := 0.0
	podCount := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		podCount++
		if pod.Status.Phase == corev1.PodPending {
			pendingPods++
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoop++
			}
		}
		if !pod.CreationTimestamp.IsZero() {
			totalPodAge += now.Sub(pod.CreationTimestamp.Time).Hours()
		}
	}
	result.Summary.CrashLoopPods = crashLoop
	if podCount > 0 {
		result.Summary.AvgPodAge = totalPodAge / float64(podCount)
	}

	// Warning events in last 1h and 24h
	warn1h := 0
	warn24h := 0
	for _, evt := range events.Items {
		if evt.Type != "Warning" {
			continue
		}
		age := now.Sub(evt.CreationTimestamp.Time)
		if age <= time.Hour {
			warn1h++
		}
		if age <= 24*time.Hour {
			warn24h++
		}
	}
	result.Summary.WarningEvents1h = warn1h
	result.Summary.WarningEvents24h = warn24h

	// Recent deployments (pods created in last 24h)
	recentDeps := 0
	failedDeps := 0
	var changes []FreezeRecentChange
	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		age := now.Sub(dep.CreationTimestamp.Time)
		if age <= 24*time.Hour {
			recentDeps++
			ready := int(dep.Status.ReadyReplicas)
			desired := 0
			if dep.Spec.Replicas != nil {
				desired = int(*dep.Spec.Replicas)
			}
			healthy := desired == 0 || ready == desired
			if !healthy {
				failedDeps++
			}
			changes = append(changes, FreezeRecentChange{
				Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
				AgeHours: int(age.Hours()), Replicas: desired, ReadyReplicas: ready, Healthy: healthy,
			})
		}
	}

	// Check for pods with recent restart timestamps (<1h)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				termTime := cs.LastTerminationState.Terminated.FinishedAt.Time
				if now.Sub(termTime) <= time.Hour {
					recentDeps++ // Count as recent change activity
				}
			}
		}
	}

	result.Summary.RecentDeployments = recentDeps
	result.Summary.FailedDeployments = failedDeps

	// Active incidents: crashloops + high warning events
	result.Summary.ActiveIncidents = crashLoop

	// Stability signals
	result.StabilitySignals = buildFreezeStabilitySignals(result.Summary)

	// Compute stability score
	result.Summary.StabilityScore = computeFreezeScore(result.Summary)

	// Determine freeze status
	result.FreezeStatus = "none"
	result.CurrentRisk = "low"
	if crashLoop > 0 {
		result.FreezeStatus = "active"
		result.CurrentRisk = "high"
	} else if warn1h > 10 {
		result.CurrentRisk = "medium"
	}

	// Verdict
	switch {
	case crashLoop > 0 || failedDeps > 0:
		result.Verdict = "freeze"
	case warn1h > 10 || pendingPods > 3:
		result.Verdict = "caution"
	default:
		result.Verdict = "proceed"
	}

	// Build freeze windows (holidays and peak periods)
	result.FreezeWindows = generateFreezeWindows(now)

	// Sort recent changes by age
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].AgeHours < changes[j].AgeHours
	})
	result.RecentChanges = changes

	// Recommendations
	result.Recommendations = generateFreezeRecs(result)

	writeJSON(w, result)
}

// buildFreezeStabilitySignals creates stability signal list.
func buildFreezeStabilitySignals(s FreezeSummary) []StabilitySignal {
	var signals []StabilitySignal

	// Crash loop signal
	status := "healthy"
	if s.CrashLoopPods > 0 {
		status = "critical"
	}
	signals = append(signals, StabilitySignal{
		Name: "crash-loops", Status: status,
		Value:  fmt.Sprintf("%d pods", s.CrashLoopPods),
		Impact: "Block all changes if >0",
	})

	// Warning events signal
	status = "healthy"
	if s.WarningEvents1h > 10 {
		status = "critical"
	} else if s.WarningEvents1h > 3 {
		status = "warning"
	}
	signals = append(signals, StabilitySignal{
		Name: "warning-events-1h", Status: status,
		Value:  fmt.Sprintf("%d events", s.WarningEvents1h),
		Impact: "High volume indicates instability",
	})

	// Failed deployments
	status = "healthy"
	if s.FailedDeployments > 0 {
		status = "critical"
	}
	signals = append(signals, StabilitySignal{
		Name: "failed-deployments", Status: status,
		Value:  fmt.Sprintf("%d in 24h", s.FailedDeployments),
		Impact: "Failed deploys indicate environment issues",
	})

	// Pod age stability
	status = "healthy"
	if s.AvgPodAge < 1 {
		status = "warning"
	}
	signals = append(signals, StabilitySignal{
		Name: "pod-stability", Status: status,
		Value:  fmt.Sprintf("%.1f hours avg age", s.AvgPodAge),
		Impact: "Fresh pods (<1h) indicate recent instability",
	})

	return signals
}

// computeFreezeScore computes stability score (0-100, higher = more stable).
func computeFreezeScore(s FreezeSummary) int {
	score := 100
	score -= minInt(s.CrashLoopPods*15, 45)
	if s.WarningEvents1h > 0 {
		score -= minInt(s.WarningEvents1h*2, 20)
	}
	if s.FailedDeployments > 0 {
		score -= minInt(s.FailedDeployments*10, 20)
	}
	if s.AvgPodAge < 1 && s.AvgPodAge >= 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateFreezeWindows creates seasonal freeze window recommendations.
func generateFreezeWindows(now time.Time) []FreezeWindow {
	var windows []FreezeWindow
	year := now.Year()

	// Holiday freeze periods (simplified: fixed dates)
	holidays := []struct {
		name  string
		start time.Time
		end   time.Time
	}{
		{"Christmas/New Year", time.Date(year, 12, 22, 0, 0, 0, 0, time.UTC), time.Date(year+1, 1, 2, 23, 59, 0, 0, time.UTC)},
		{"Black Friday/Cyber Monday", time.Date(year, 11, 24, 0, 0, 0, 0, time.UTC), time.Date(year, 11, 30, 23, 59, 0, 0, time.UTC)},
	}

	for _, h := range holidays {
		if now.After(h.start.Add(-48*time.Hour)) && now.Before(h.end.Add(48*time.Hour)) {
			status := "upcoming"
			if now.After(h.start) && now.Before(h.end) {
				status = "active"
			}
			windows = append(windows, FreezeWindow{
				Name: h.name, Starts: h.start.Format("2006-01-02"),
				Ends: h.end.Format("2006-01-02"), Reason: status + " freeze period",
				Type: "holiday", Scope: "cluster",
			})
		}
	}

	return windows
}

// generateFreezeRecs produces recommendations.
func generateFreezeRecs(r ChangeFreezeResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Change freeze status: %s (verdict: %s, stability: %d/100)",
		r.FreezeStatus, r.Verdict, r.Summary.StabilityScore))

	if r.Verdict == "freeze" {
		if r.Summary.CrashLoopPods > 0 {
			recs = append(recs, fmt.Sprintf("FREEZE ACTIVE: %d crash-loop pod(s) — resolve before any changes", r.Summary.CrashLoopPods))
		}
		if r.Summary.FailedDeployments > 0 {
			recs = append(recs, fmt.Sprintf("%d failed deployment(s) in 24h — investigate root cause", r.Summary.FailedDeployments))
		}
	}

	if r.Summary.WarningEvents1h > 5 {
		recs = append(recs, fmt.Sprintf("%d warning events in last hour — cluster is unstable", r.Summary.WarningEvents1h))
	}

	if r.Summary.AvgPodAge < 1 && r.Summary.AvgPodAge >= 0 {
		recs = append(recs, fmt.Sprintf("Average pod age is %.1f hours — recent mass restart detected", r.Summary.AvgPodAge))
	}

	if len(r.FreezeWindows) > 0 {
		for _, fw := range r.FreezeWindows {
			recs = append(recs, fmt.Sprintf("%s: %s to %s — %s", fw.Name, fw.Starts, fw.Ends, fw.Reason))
		}
	}

	if r.Verdict == "proceed" {
		recs = append(recs, "Cluster is stable — changes can proceed safely")
	}

	return recs
}

// Suppress unused imports
var _ = strings.Contains
var _ appsv1.Deployment
