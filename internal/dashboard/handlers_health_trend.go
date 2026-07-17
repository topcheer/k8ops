package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HealthTrendResult tracks cluster health metrics over time by comparing
// deployment creation timestamps, restart counts, and event patterns.
// It provides a temporal view of cluster stability.
type HealthTrendResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         HealthTrendSummary `json:"summary"`
	ByWeek          []WeekTrend        `json:"byWeek"`
	ByNamespace     []NSHealthTrend    `json:"byNamespace"`
	StabilityScore  int                `json:"stabilityScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type HealthTrendSummary struct {
	TotalPods         int     `json:"totalPods"`
	TotalRestarts     int     `json:"totalRestarts"`
	AvgRestartsPerPod float64 `json:"avgRestartsPerPod"`
	CrashRate         float64 `json:"crashRate"`
	NewWorkloads30d   int     `json:"newWorkloads30d"`
	FailedPods        int     `json:"failedPods"`
	PendingPods       int     `json:"pendingPods"`
}

type WeekTrend struct {
	Week       string `json:"week"`
	Pods       int    `json:"pods"`
	Restarts   int    `json:"restarts"`
	NewDeploys int    `json:"newDeploys"`
}

type NSHealthTrend struct {
	Namespace string  `json:"namespace"`
	Pods      int     `json:"pods"`
	Restarts  int     `json:"restarts"`
	CrashRate float64 `json:"crashRate"`
	Score     int     `json:"score"`
}

// handleHealthTrend handles GET /api/operations/health-trend
func (s *Server) handleHealthTrend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := HealthTrendResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	nsMap := make(map[string]*NSHealthTrend)

	// Aggregate pod stats
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		result.Summary.TotalPods++

		podRestarts := 0
		crashing := false
		for _, cs := range pod.Status.ContainerStatuses {
			podRestarts += int(cs.RestartCount)
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				crashing = true
			}
		}
		result.Summary.TotalRestarts += podRestarts

		switch pod.Status.Phase {
		case corev1.PodFailed:
			result.Summary.FailedPods++
		case corev1.PodPending:
			result.Summary.PendingPods++
		}

		if crashing {
			result.Summary.CrashRate++
		}

		// NS aggregation
		if _, ok := nsMap[pod.Namespace]; !ok {
			nsMap[pod.Namespace] = &NSHealthTrend{Namespace: pod.Namespace}
		}
		ns := nsMap[pod.Namespace]
		ns.Pods++
		ns.Restarts += podRestarts
		if crashing {
			ns.CrashRate++
		}
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRestartsPerPod = float64(result.Summary.TotalRestarts) / float64(result.Summary.TotalPods)
		result.Summary.CrashRate = result.Summary.CrashRate / float64(result.Summary.TotalPods) * 100
	}

	// New workloads in last 30 days
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		if now.Sub(d.CreationTimestamp.Time) < 30*24*time.Hour {
			result.Summary.NewWorkloads30d++
		}
	}

	// Weekly trend (last 8 weeks)
	for i := 7; i >= 0; i-- {
		weekStart := now.AddDate(0, 0, -7*(i+1))
		weekEnd := now.AddDate(0, 0, -7*i)
		weekPods := 0
		weekRestarts := 0
		weekDeploys := 0

		for _, pod := range pods.Items {
			if isSystemNamespace(pod.Namespace) {
				continue
			}
			ct := pod.CreationTimestamp.Time
			if ct.After(weekStart) && ct.Before(weekEnd) {
				weekPods++
			}
			for _, cs := range pod.Status.ContainerStatuses {
				// Approximate: distribute restarts evenly across pod lifetime
				if !pod.CreationTimestamp.Time.IsZero() {
					weeksAlive := int(now.Sub(pod.CreationTimestamp.Time).Hours()/(24*7)) + 1
					if weeksAlive > 0 {
						weekRestarts += int(cs.RestartCount) / weeksAlive
					}
				}
			}
		}

		for _, d := range deployments.Items {
			if isSystemNamespace(d.Namespace) {
				continue
			}
			ct := d.CreationTimestamp.Time
			if ct.After(weekStart) && ct.Before(weekEnd) {
				weekDeploys++
			}
		}

		result.ByWeek = append(result.ByWeek, WeekTrend{
			Week: weekStart.Format("2006-01-02"), Pods: weekPods,
			Restarts: weekRestarts, NewDeploys: weekDeploys,
		})
	}

	// NS breakdown
	for _, ns := range nsMap {
		ns.Score = 100
		if ns.Pods > 0 {
			ns.Score -= ns.Restarts / ns.Pods * 5
			if ns.CrashRate > 0 {
				ns.Score -= int(ns.CrashRate / float64(ns.Pods) * 100 * 10)
			}
		}
		if ns.Score < 0 {
			ns.Score = 0
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Score < result.ByNamespace[j].Score
	})

	// Stability score
	result.StabilityScore = 100
	if result.Summary.TotalPods > 0 {
		result.StabilityScore -= int(result.Summary.AvgRestartsPerPod * 3)
		result.StabilityScore -= int(result.Summary.CrashRate)
		result.StabilityScore -= result.Summary.FailedPods * 5
	}
	if result.StabilityScore < 0 {
		result.StabilityScore = 0
	}

	switch {
	case result.StabilityScore >= 80:
		result.Grade = "A"
	case result.StabilityScore >= 60:
		result.Grade = "B"
	case result.StabilityScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildHealthTrendRecs(&result)
	writeJSON(w, result)
}

func buildHealthTrendRecs(r *HealthTrendResult) []string {
	recs := []string{
		fmt.Sprintf("集群稳定性: %d/100 (%s)", r.StabilityScore, r.Grade),
	}
	if r.Summary.AvgRestartsPerPod > 3 {
		recs = append(recs, fmt.Sprintf("平均每个 Pod 重启 %.1f 次，建议排查稳定性问题", r.AvgRestartsPerPodSafe()))
	}
	if r.Summary.CrashRate > 5 {
		recs = append(recs, fmt.Sprintf("CrashLoopBackOff 比率 %.1f%%", r.Summary.CrashRate))
	}
	if r.Summary.NewWorkloads30d > 5 {
		recs = append(recs, fmt.Sprintf("近 30 天新增 %d 个工作负载，注意资源规划", r.Summary.NewWorkloads30d))
	}
	worstNS := ""
	worstScore := 100
	for _, ns := range r.ByNamespace {
		if ns.Score < worstScore {
			worstScore = ns.Score
			worstNS = ns.Namespace
		}
	}
	if worstNS != "" && worstScore < 60 {
		recs = append(recs, fmt.Sprintf("最不稳定命名空间: %s (score=%d)", worstNS, worstScore))
	}
	return recs
}

func (r *HealthTrendResult) AvgRestartsPerPodSafe() float64 {
	return r.Summary.AvgRestartsPerPod
}

var _ appsv1.DeploymentList
