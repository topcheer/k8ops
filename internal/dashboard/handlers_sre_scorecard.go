package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SREScorecardResult generates an SRE scorecard using error budget and service reliability metrics.
type SREScorecardResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SREScorecardSummary `json:"summary"`
	Indicators      []SREIndicator      `json:"indicators"`
	ByNamespace     []SRENamespaceEntry `json:"byNamespace"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type SREScorecardSummary struct {
	TotalServices     int     `json:"totalServices"`
	HealthyServices   int     `json:"healthyServices"`
	DegradedServices  int     `json:"degradedServices"`
	ErrorBudgetUsed   float64 `json:"errorBudgetUsedPct"`
	MTTRMinutes       int     `json:"mttrMinutes"`
	ChangeFailureRate float64 `json:"changeFailureRate"`
}

type SREIndicator struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Score  int    `json:"score"`
	Target string `json:"target"`
	Status string `json:"status"`
}

type SRENamespaceEntry struct {
	Namespace    string `json:"namespace"`
	PodCount     int    `json:"podCount"`
	CrashCount   int    `json:"crashCount"`
	RestartTotal int    `json:"restartTotal"`
	Reliability  int    `json:"reliabilityScore"`
}

// handleSREScorecard handles GET /api/docs/sre-scorecard
func (s *Server) handleSREScorecard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SREScorecardResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{Limit: 500})

	// Count by namespace
	nsMap := make(map[string]*SRENamespaceEntry)
	totalRestarts := 0
	crashingPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &SRENamespaceEntry{Namespace: pod.Namespace}
		}
		nsMap[pod.Namespace].PodCount++
		for _, cs := range pod.Status.ContainerStatuses {
			nsMap[pod.Namespace].RestartTotal += int(cs.RestartCount)
			totalRestarts += int(cs.RestartCount)
			if cs.RestartCount > 5 {
				crashingPods++
			}
		}
	}

	for _, e := range nsMap {
		if e.PodCount > 0 {
			crashRatio := float64(e.CrashCount) / float64(e.PodCount)
			restartRatio := float64(e.RestartTotal) / float64(e.PodCount)
			reliability := 100 - int(restartRatio*10) - int(crashRatio*50)
			if reliability < 0 {
				reliability = 0
			}
			e.Reliability = reliability
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Reliability < result.ByNamespace[j].Reliability
	})

	// Count events for error budget
	warningEvents := 0
	for _, ev := range events.Items {
		if ev.Type == corev1.EventTypeWarning {
			warningEvents++
		}
	}

	// Build SRE indicators
	totalPods := 0
	for _, e := range nsMap {
		totalPods += e.PodCount
	}
	result.Summary.TotalServices = len(services.Items)
	result.Summary.ErrorBudgetUsed = float64(warningEvents) / float64(len(events.Items)+1) * 100
	result.Summary.ChangeFailureRate = float64(crashingPods) / float64(totalPods+1) * 100

	// Availability: pods running vs failed
	runningPods := 0
	for _, pod := range pods.Items {
		if !isSystemNamespace(pod.Namespace) && pod.Status.Phase == corev1.PodRunning {
			runningPods++
		}
	}
	availability := 100.0
	if totalPods > 0 {
		availability = float64(runningPods) / float64(totalPods) * 100
	}

	result.Indicators = []SREIndicator{
		{Name: "Availability", Value: fmt.Sprintf("%.2f%%", availability), Score: int(availability), Target: "99.9%", Status: statusFromScore(int(availability))},
		{Name: "Error Budget Used", Value: fmt.Sprintf("%.1f%%", result.Summary.ErrorBudgetUsed), Score: 100 - int(result.Summary.ErrorBudgetUsed), Target: "<30%", Status: statusFromScore(100 - int(result.Summary.ErrorBudgetUsed))},
		{Name: "Change Failure Rate", Value: fmt.Sprintf("%.1f%%", result.Summary.ChangeFailureRate), Score: 100 - int(result.Summary.ChangeFailureRate), Target: "<15%", Status: statusFromScore(100 - int(result.Summary.ChangeFailureRate))},
		{Name: "Pod Stability", Value: fmt.Sprintf("%d/%d stable", totalPods-crashingPods, totalPods), Score: (totalPods - crashingPods) * 100 / (totalPods + 1), Target: "100%", Status: statusFromScore((totalPods - crashingPods) * 100 / (totalPods + 1))},
		{Name: "Restart Health", Value: fmt.Sprintf("%d total restarts", totalRestarts), Score: 100 - totalRestarts/(totalPods+1)*10, Target: "<1/pod", Status: statusFromScore(100 - totalRestarts/(totalPods+1)*10)},
	}

	scoreSum := 0
	for _, ind := range result.Indicators {
		scoreSum += ind.Score
	}
	result.HealthScore = scoreSum / len(result.Indicators)
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("SRE 记分卡: 可用性 %.1f%%, 错误预算 %.1f%%, 变更失败率 %.1f%%, 重启 %d",
			availability, result.Summary.ErrorBudgetUsed, result.Summary.ChangeFailureRate, totalRestarts),
	}
	for _, ind := range result.Indicators {
		if ind.Status == "critical" || ind.Status == "warning" {
			result.Recommendations = append(result.Recommendations, fmt.Sprintf("%s: %s (目标 %s)", ind.Name, ind.Value, ind.Target))
		}
	}
	writeJSON(w, result)
}

func statusFromScore(score int) string {
	if score >= 90 {
		return "healthy"
	}
	if score >= 70 {
		return "warning"
	}
	return "critical"
}
