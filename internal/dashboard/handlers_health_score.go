package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HealthScoreResult is the aggregated cluster health report.
type HealthScoreResult struct {
	Timestamp    time.Time        `json:"timestamp"`
	OverallScore int              `json:"overallScore"` // 0-100
	Grade        string           `json:"grade"`        // A/B/C/D/F
	Status       string           `json:"status"`       // healthy / warning / critical
	Categories   []HealthCategory `json:"categories"`
	TopIssues    []HealthIssue    `json:"topIssues"`
	Summary      HealthSummary    `json:"summary"`
}

// HealthCategory is one dimension of cluster health.
type HealthCategory struct {
	Name       string `json:"name"`
	Score      int    `json:"score"`  // 0-100
	Weight     int    `json:"weight"` // contribution to overall
	Status     string `json:"status"` // healthy / warning / critical
	Detail     string `json:"detail"`
	IssueCount int    `json:"issueCount"`
}

// HealthIssue is a specific actionable problem.
type HealthIssue struct {
	Category string `json:"category"`
	Severity string `json:"severity"` // critical / warning / info
	Message  string `json:"message"`
}

// HealthSummary provides quick-glance counts.
type HealthSummary struct {
	TotalNodes        int `json:"totalNodes"`
	ReadyNodes        int `json:"readyNodes"`
	TotalPods         int `json:"totalPods"`
	RunningPods       int `json:"runningPods"`
	CrashLoopPods     int `json:"crashLoopPods"`
	PendingPods       int `json:"pendingPods"`
	FailedPods        int `json:"failedPods"`
	RestartingPods    int `json:"restartingPods"`
	TotalWorkloads    int `json:"totalWorkloads"`
	NotReadyWorkloads int `json:"notReadyWorkloads"`
	RecentWarnings    int `json:"recentWarnings"` // last 1h
}

// handleHealthScore aggregates all health signals into one score.
// GET /api/operations/health-score
func (s *Server) handleHealthScore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := HealthScoreResult{Timestamp: time.Now()}
	now := time.Now()

	// --- Fetch cluster state ---
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("type=Warning,lastTimestamp>%s", now.Add(-time.Hour).Format(time.RFC3339)),
	})

	// --- Category 1: Node Health (weight 25) ---
	nodeCat := assessNodeHealth(nodes)
	result.Categories = append(result.Categories, nodeCat)

	// --- Category 2: Pod Health (weight 25) ---
	podCat, podSummary := assessPodHealth(pods)
	result.Categories = append(result.Categories, podCat)
	result.Summary = podSummary

	// --- Category 3: Workload Health (weight 20) ---
	wlCat := assessWorkloadHealth(deployments, statefulsets, daemonsets)
	result.Categories = append(result.Categories, wlCat)

	// --- Category 4: Event Activity (weight 15) ---
	eventCat := assessEventHealth(events, now)
	result.Categories = append(result.Categories, eventCat)
	result.Summary.RecentWarnings = eventCat.IssueCount

	// --- Category 5: API Server Health (weight 15) ---
	apiCat := assessAPIServerHealth(ctx, rc)
	result.Categories = append(result.Categories, apiCat)

	// Fill node summary
	if nodes != nil {
		result.Summary.TotalNodes = len(nodes.Items)
		result.Summary.ReadyNodes = result.Summary.TotalNodes - nodeCat.IssueCount
	}
	result.Summary.TotalWorkloads = wlCat.IssueCount + (wlCat.IssueCount * 0) // placeholder
	result.Summary.NotReadyWorkloads = wlCat.IssueCount

	// Calculate overall score
	totalWeight := 0
	weightedSum := 0
	for _, cat := range result.Categories {
		weightedSum += cat.Score * cat.Weight
		totalWeight += cat.Weight
	}
	if totalWeight > 0 {
		result.OverallScore = weightedSum / totalWeight
	}

	// Grade and status
	result.Grade = scoreToGrade(result.OverallScore)
	result.Status = scoreToStatus(result.OverallScore)

	// Collect top issues
	for _, cat := range result.Categories {
		if cat.Status != "healthy" && cat.IssueCount > 0 {
			severity := "warning"
			if cat.Score < 40 {
				severity = "critical"
			}
			result.TopIssues = append(result.TopIssues, HealthIssue{
				Category: cat.Name,
				Severity: severity,
				Message:  cat.Detail,
			})
		}
	}
	sort.Slice(result.TopIssues, func(i, j int) bool {
		return severityRank(result.TopIssues[i].Severity) < severityRank(result.TopIssues[j].Severity)
	})

	writeJSON(w, result)
}

// assessNodeHealth checks node readiness.
func assessNodeHealth(nodes *corev1.NodeList) HealthCategory {
	cat := HealthCategory{Name: "Node Health", Weight: 25, Score: 100, Status: "healthy"}
	if nodes == nil || len(nodes.Items) == 0 {
		cat.Detail = "No nodes found"
		cat.Score = 0
		cat.Status = "critical"
		return cat
	}

	total := len(nodes.Items)
	notReady := 0
	for _, node := range nodes.Items {
		if !isNodeReady(&node) {
			notReady++
		}
	}

	cat.Score = int(float64(total-notReady) / float64(total) * 100)
	cat.IssueCount = notReady

	if notReady == 0 {
		cat.Detail = fmt.Sprintf("All %d nodes ready", total)
		cat.Status = "healthy"
	} else if notReady < total {
		cat.Detail = fmt.Sprintf("%d/%d nodes not ready", notReady, total)
		cat.Status = "warning"
		if cat.Score < 50 {
			cat.Status = "critical"
		}
	} else {
		cat.Detail = "All nodes not ready"
		cat.Status = "critical"
	}

	return cat
}

// assessPodHealth checks pod phases and crash loops.
func assessPodHealth(pods *corev1.PodList) (HealthCategory, HealthSummary) {
	cat := HealthCategory{Name: "Pod Health", Weight: 25, Score: 100, Status: "healthy"}
	summary := HealthSummary{}

	if pods == nil || len(pods.Items) == 0 {
		cat.Detail = "No pods found"
		cat.Score = 0
		cat.Status = "critical"
		return cat, summary
	}

	total := len(pods.Items)
	running := 0
	crashLoop := 0
	pending := 0
	failed := 0
	restarting := 0

	for _, pod := range pods.Items {
		summary.TotalPods++
		switch pod.Status.Phase {
		case corev1.PodRunning:
			running++
			totalRestarts := int32(0)
			for _, cs := range pod.Status.ContainerStatuses {
				totalRestarts += cs.RestartCount
			}
			if totalRestarts >= 5 {
				restarting++
			}
		case corev1.PodPending:
			pending++
		case corev1.PodFailed:
			failed++
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashLoop++
				break
			}
		}
	}

	summary.RunningPods = running
	summary.CrashLoopPods = crashLoop
	summary.PendingPods = pending
	summary.FailedPods = failed
	summary.RestartingPods = restarting

	issues := crashLoop*5 + pending*2 + failed*3 + restarting*2
	if total > 0 {
		cat.Score = 100 - (issues*100)/total
	}
	if cat.Score < 0 {
		cat.Score = 0
	}

	cat.IssueCount = crashLoop + pending + failed + restarting

	if cat.IssueCount == 0 {
		cat.Detail = fmt.Sprintf("All %d pods running healthy", running)
		cat.Status = "healthy"
	} else if crashLoop > 0 {
		cat.Detail = fmt.Sprintf("%d crash loops, %d pending, %d failed, %d high-restart", crashLoop, pending, failed, restarting)
		cat.Status = "critical"
	} else {
		cat.Detail = fmt.Sprintf("%d pending, %d failed, %d high-restart", pending, failed, restarting)
		cat.Status = "warning"
	}

	return cat, summary
}

// assessWorkloadHealth checks deployment/statefulset/daemonset readiness.
func assessWorkloadHealth(deployments *appsv1.DeploymentList, statefulsets *appsv1.StatefulSetList, daemonsets *appsv1.DaemonSetList) HealthCategory {
	cat := HealthCategory{Name: "Workload Health", Weight: 20, Score: 100, Status: "healthy"}
	total := 0
	notReady := 0

	if deployments != nil {
		for _, dep := range deployments.Items {
			total++
			desired := int32(1)
			if dep.Spec.Replicas != nil {
				desired = *dep.Spec.Replicas
			}
			if dep.Status.ReadyReplicas < desired {
				notReady++
			}
		}
	}
	if statefulsets != nil {
		for _, sts := range statefulsets.Items {
			total++
			desired := int32(1)
			if sts.Spec.Replicas != nil {
				desired = *sts.Spec.Replicas
			}
			if sts.Status.ReadyReplicas < desired {
				notReady++
			}
		}
	}
	if daemonsets != nil {
		for _, ds := range daemonsets.Items {
			total++
			if ds.Status.NumberReady < ds.Status.DesiredNumberScheduled {
				notReady++
			}
		}
	}

	if total == 0 {
		cat.Detail = "No workloads found"
		cat.Score = 100
		return cat
	}

	cat.Score = int(float64(total-notReady) / float64(total) * 100)
	cat.IssueCount = notReady

	if notReady == 0 {
		cat.Detail = fmt.Sprintf("All %d workloads ready", total)
		cat.Status = "healthy"
	} else {
		cat.Detail = fmt.Sprintf("%d/%d workloads not ready", notReady, total)
		cat.Status = "warning"
		if cat.Score < 50 {
			cat.Status = "critical"
		}
	}

	return cat
}

// assessEventHealth checks for excessive warning events.
func assessEventHealth(events *corev1.EventList, now time.Time) HealthCategory {
	cat := HealthCategory{Name: "Event Activity", Weight: 15, Score: 100, Status: "healthy"}

	recentCount := 0
	if events != nil {
		for _, event := range events.Items {
			if event.LastTimestamp.After(now.Add(-time.Hour)) {
				recentCount++
			}
		}
	}

	cat.IssueCount = recentCount

	switch {
	case recentCount == 0:
		cat.Detail = "No warning events in the last hour"
		cat.Score = 100
	case recentCount < 10:
		cat.Detail = fmt.Sprintf("%d warning events in the last hour", recentCount)
		cat.Score = 85
	case recentCount < 30:
		cat.Detail = fmt.Sprintf("%d warning events — elevated activity", recentCount)
		cat.Score = 65
		cat.Status = "warning"
	case recentCount < 60:
		cat.Detail = fmt.Sprintf("%d warning events — high activity", recentCount)
		cat.Score = 40
		cat.Status = "warning"
	default:
		cat.Detail = fmt.Sprintf("%d warning events — event storm likely", recentCount)
		cat.Score = 15
		cat.Status = "critical"
	}

	return cat
}

// assessAPIServerHealth does a quick API server latency check.
func assessAPIServerHealth(ctx context.Context, rc *requestClients) HealthCategory {
	cat := HealthCategory{Name: "API Server", Weight: 15, Score: 100, Status: "healthy"}

	if rc == nil || rc.clientset == nil {
		cat.Detail = "Unable to reach API server"
		cat.Score = 0
		cat.Status = "critical"
		return cat
	}

	start := time.Now()
	_, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	latency := time.Since(start)

	if err != nil {
		cat.Detail = fmt.Sprintf("API server error: %v", err)
		cat.Score = 0
		cat.Status = "critical"
		cat.IssueCount = 1
		return cat
	}

	ms := latency.Milliseconds()
	switch {
	case ms < 100:
		cat.Detail = fmt.Sprintf("API server responsive (%dms)", ms)
		cat.Score = 100
	case ms < 300:
		cat.Detail = fmt.Sprintf("API server normal (%dms)", ms)
		cat.Score = 85
	case ms < 1000:
		cat.Detail = fmt.Sprintf("API server slow (%dms)", ms)
		cat.Score = 60
		cat.Status = "warning"
	case ms < 3000:
		cat.Detail = fmt.Sprintf("API server very slow (%dms)", ms)
		cat.Score = 30
		cat.Status = "warning"
	default:
		cat.Detail = fmt.Sprintf("API server critically slow (%dms)", ms)
		cat.Score = 10
		cat.Status = "critical"
	}

	return cat
}

// scoreToGrade converts a 0-100 score to letter grade.
func scoreToGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}

// scoreToStatus converts score to status string.
func scoreToStatus(score int) string {
	switch {
	case score >= 70:
		return "healthy"
	case score >= 40:
		return "warning"
	default:
		return "critical"
	}
}

// severityRank ranks severity for sorting.
func severityRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}
