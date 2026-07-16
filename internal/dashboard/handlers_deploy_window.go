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

// DeployWindowResult analyzes cluster activity patterns to recommend the safest
// time windows for deploying changes. It evaluates event density, pod restart
// patterns, warning events, and workload criticality to identify low-risk windows.
type DeployWindowResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         DWSummary           `json:"summary"`
	RecommendedWindows []DWWindow       `json:"recommendedWindows"`
	HighRiskWindows    []DWWindow       `json:"highRiskWindows"`
	ActivityByHour    []DWHourActivity `json:"activityByHour"`
	CurrentRisk       string             `json:"currentRisk"`
	BestWindow        string             `json:"bestWindow"`
	WorstWindow       string             `json:"worstWindow"`
	Verdict           string             `json:"verdict"` // safe-to-deploy, caution, wait
	Recommendations   []string           `json:"recommendations"`
}

// DWSummary aggregates deployment window statistics.
type DWSummary struct {
	TotalEvents        int     `json:"totalEvents"`
	WarningEvents      int     `json:"warningEvents"`
	NormalEvents       int     `json:"normalEvents"`
	ActiveRestarts     int     `json:"activeRestarts"`
	CrashLoopPods      int     `json:"crashLoopPods"`
	PendingPods        int     `json:"pendingPods"`
	CriticalWorkloads  int     `json:"criticalWorkloads"`
	AvgHourlyActivity  float64 `json:"avgHourlyActivity"`
}

// DWWindow describes a time window for deployment.
type DWWindow struct {
	StartHour    int     `json:"startHour"`
	EndHour      int     `json:"endHour"`
	DayOfWeek    string  `json:"dayOfWeek"` // weekday, weekend, any
	RiskScore    int     `json:"riskScore"`  // 0-100 (lower = safer)
	RiskLevel    string  `json:"riskLevel"`
	EventDensity float64 `json:"eventDensity"`
	Reason       string  `json:"reason"`
}

// DWHourActivity shows activity level per hour.
type DWHourActivity struct {
	Hour         int     `json:"hour"`
	EventCount   int     `json:"eventCount"`
	WarningCount int     `json:"warningCount"`
	RestartCount int     `json:"restartCount"`
	ActivityScore int    `json:"activityScore"`
}

// handleDeployWindow handles GET /api/deployment/deploy-window
func (s *Server) handleDeployWindow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DeployWindowResult{ScannedAt: time.Now()}
	now := time.Now()

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Build hourly activity map (last 7 days)
	hourlyActivity := [24]*DWHourActivity{}
	for i := range hourlyActivity {
		hourlyActivity[i] = &DWHourActivity{Hour: i}
	}

	totalEvents := 0
	warningEvents := 0
	activeRestarts := 0
	crashLoopPods := 0
	pendingPods := 0

	// Analyze events by hour
	for _, evt := range events.Items {
		age := now.Sub(evt.CreationTimestamp.Time)
		if age > 7*24*time.Hour {
			continue
		}
		hour := evt.CreationTimestamp.Time.Hour()
		if hour < 0 || hour > 23 {
			continue
		}
		totalEvents++
		hourlyActivity[hour].EventCount++
		if evt.Type == "Warning" {
			warningEvents++
			hourlyActivity[hour].WarningCount++
		}
	}
	result.Summary.TotalEvents = totalEvents
	result.Summary.WarningEvents = warningEvents
	result.Summary.NormalEvents = totalEvents - warningEvents

	// Analyze pod restarts
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			activeRestarts += int(cs.RestartCount)
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoopPods++
			}
		}
		if pod.Status.Phase == corev1.PodPending {
			pendingPods++
		}
	}
	result.Summary.ActiveRestarts = activeRestarts
	result.Summary.CrashLoopPods = crashLoopPods
	result.Summary.PendingPods = pendingPods

	// Count critical workloads (deployments with replicas >= 3)
	criticalWorkloads := 0
	for _, dep := range deployments.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		if dep.Spec.Replicas != nil && *dep.Spec.Replicas >= 3 {
			criticalWorkloads++
		}
	}
	result.Summary.CriticalWorkloads = criticalWorkloads

	// Compute activity scores per hour
	maxEvents := 1
	for _, ha := range hourlyActivity {
		if ha.EventCount > maxEvents {
			maxEvents = ha.EventCount
		}
	}
	totalActivity := 0
	for _, ha := range hourlyActivity {
		ha.ActivityScore = int(float64(ha.EventCount) / float64(maxEvents) * 100)
		totalActivity += ha.EventCount
		result.ActivityByHour = append(result.ActivityByHour, *ha)
	}
	if len(result.ActivityByHour) > 0 {
		result.Summary.AvgHourlyActivity = float64(totalActivity) / 24.0
	}

	// Find low-activity windows (consecutive hours with lowest activity)
	windowSize := 2 // 2-hour windows
	type windowScore struct {
		start int
		score int
	}
	var scores []windowScore
	for start := 0; start <= 24-windowSize; start++ {
		ws := 0
		for h := start; h < start+windowSize; h++ {
			realH := h % 24
			ws += hourlyActivity[realH].ActivityScore
		}
		scores = append(scores, windowScore{start: start, score: ws / windowSize})
	}

	// Sort by score (ascending = safest first)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score < scores[j].score
	})

	// Top 3 recommended windows
	for i := 0; i < minInt(3, len(scores)); i++ {
		start := scores[i].start
		end := (start + windowSize) % 24
		risk := scores[i].score
		riskLevel := "low"
		if risk > 50 {
			riskLevel = "medium"
		}
		if risk > 75 {
			riskLevel = "high"
		}
		dayType := "any"
		reason := fmt.Sprintf("Low activity window (%d%% avg) — %d events in typical 7-day window", risk, hourlyActivity[start].EventCount+hourlyActivity[(start+1)%24].EventCount)
		if start >= 0 && start < 6 {
			dayType = "off-hours"
			reason = fmt.Sprintf("Off-peak window %02d:00-%02d:00 — minimal cluster activity", start, end)
		}
		result.RecommendedWindows = append(result.RecommendedWindows, DWWindow{
			StartHour: start, EndHour: end, DayOfWeek: dayType,
			RiskScore: risk, RiskLevel: riskLevel,
			EventDensity: float64(hourlyActivity[start].EventCount+hourlyActivity[(start+1)%24].EventCount) / float64(windowSize),
			Reason: reason,
		})
	}

	// Top 3 high-risk windows (bottom of sorted list)
	for i := len(scores) - 1; i >= maxInt(0, len(scores)-3); i-- {
		start := scores[i].start
		end := (start + windowSize) % 24
		risk := scores[i].score
		result.HighRiskWindows = append(result.HighRiskWindows, DWWindow{
			StartHour: start, EndHour: end, DayOfWeek: "peak-hours",
			RiskScore: risk, RiskLevel: "high",
			EventDensity: float64(hourlyActivity[start].EventCount+hourlyActivity[(start+1)%24].EventCount) / float64(windowSize),
			Reason: fmt.Sprintf("Peak activity window %02d:00-%02d:00 — %d events in typical 7-day window", start, end, hourlyActivity[start].EventCount+hourlyActivity[(start+1)%24].EventCount),
		})
	}

	// Current risk level
	currentHour := now.Hour()
	currentScore := hourlyActivity[currentHour].ActivityScore
	result.CurrentRisk = "low"
	if currentScore > 50 {
		result.CurrentRisk = "medium"
	}
	if currentScore > 75 {
		result.CurrentRisk = "high"
	}

	// Best and worst windows
	if len(scores) > 0 {
		best := scores[0]
		result.BestWindow = fmt.Sprintf("%02d:00-%02d:00 (risk: %d%%)", best.start, (best.start+windowSize)%24, best.score)
		worst := scores[len(scores)-1]
		result.WorstWindow = fmt.Sprintf("%02d:00-%02d:00 (risk: %d%%)", worst.start, (worst.start+windowSize)%24, worst.score)
	}

	// Overall verdict
	if crashLoopPods > 0 {
		result.Verdict = "wait"
	} else if currentScore > 60 {
		result.Verdict = "caution"
	} else {
		result.Verdict = "safe-to-deploy"
	}

	// Recommendations
	result.Recommendations = generateDWRecs(result)

	writeJSON(w, result)
}

// generateDWRecs produces deployment window recommendations.
func generateDWRecs(r DeployWindowResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Current deployment risk: %s (verdict: %s)", r.CurrentRisk, r.Verdict))

	if r.BestWindow != "" {
		recs = append(recs, fmt.Sprintf("Best deployment window: %s", r.BestWindow))
	}
	if r.WorstWindow != "" {
		recs = append(recs, fmt.Sprintf("Avoid deploying during: %s", r.WorstWindow))
	}

	if r.Summary.CrashLoopPods > 0 {
		recs = append(recs, fmt.Sprintf("WAIT: %d pod(s) in CrashLoopBackOff — resolve before deploying", r.Summary.CrashLoopPods))
	}

	if r.Summary.PendingPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) pending — cluster may have resource pressure", r.Summary.PendingPods))
	}

	if r.Summary.WarningEvents > totalEventsThreshold(r.Summary.TotalEvents) {
		recs = append(recs, fmt.Sprintf("High warning event ratio (%d/%d) — cluster is unstable", r.Summary.WarningEvents, r.Summary.TotalEvents))
	}

	if r.Summary.CriticalWorkloads > 5 {
		recs = append(recs, fmt.Sprintf("%d critical workloads detected — use progressive deployment (canary/blue-green)", r.Summary.CriticalWorkloads))
	}

	for _, w := range r.RecommendedWindows[:minInt(2, len(r.RecommendedWindows))] {
		recs = append(recs, fmt.Sprintf("Recommended: %02d:00-%02d:00 (%s, risk %d%%) — %s", w.StartHour, w.EndHour, w.DayOfWeek, w.RiskScore, w.Reason))
	}

	return recs
}

// totalEventsThreshold computes a warning ratio threshold.
func totalEventsThreshold(total int) int {
	if total == 0 {
		return 1
	}
	threshold := total * 30 / 100
	if threshold < 1 {
		threshold = 1
	}
	return threshold
}

// Suppress unused import warning
var _ = strings.Contains
