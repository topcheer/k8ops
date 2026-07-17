package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodHealthIndexResult provides a per-pod health score aggregated by
// workload and namespace. It combines restart count, ready status, probe
// failures, resource pressure, and age into a single actionable metric.
type PodHealthIndexResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         PodHealthSummary `json:"summary"`
	Pods            []PodHealthEntry `json:"pods"`
	ByNamespace     []PodHealthNS    `json:"byNamespace"`
	Unhealthy       []PodHealthEntry `json:"unhealthy"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type PodHealthSummary struct {
	TotalPods     int     `json:"totalPods"`
	RunningPods   int     `json:"runningPods"`
	HealthyPods   int     `json:"healthyPods"`
	UnhealthyPods int     `json:"unhealthyPods"`
	PendingPods   int     `json:"pendingPods"`
	FailedPods    int     `json:"failedPods"`
	HighRestart   int     `json:"highRestartPods"`
	CrashLoop     int     `json:"crashLoopPods"`
	NotReady      int     `json:"notReadyPods"`
	AvgRestart    float64 `json:"avgRestartCount"`
}

type PodHealthEntry struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Node        string   `json:"node"`
	Phase       string   `json:"phase"`
	Ready       bool     `json:"ready"`
	Restarts    int      `json:"restarts"`
	Age         string   `json:"age"`
	HealthScore int      `json:"healthScore"`
	Issues      []string `json:"issues"`
	Kind        string   `json:"kind"`
	OwnerName   string   `json:"ownerName"`
}

type PodHealthNS struct {
	Namespace     string  `json:"namespace"`
	PodCount      int     `json:"podCount"`
	Unhealthy     int     `json:"unhealthyCount"`
	AvgScore      float64 `json:"avgScore"`
	TotalRestarts int     `json:"totalRestarts"`
}

// handlePodHealthIndex handles GET /api/operations/pod-health-index
func (s *Server) handlePodHealthIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PodHealthIndexResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Build node pressure map
	nodePressure := make(map[string]bool)
	for _, n := range nodes.Items {
		for _, cond := range n.Status.Conditions {
			if cond.Status == corev1.ConditionTrue &&
				(cond.Type == corev1.NodeMemoryPressure ||
					cond.Type == corev1.NodeDiskPressure ||
					cond.Type == corev1.NodePIDPressure) {
				nodePressure[n.Name] = true
			}
		}
	}

	nsMap := make(map[string]*PodHealthNS)
	var allPods []PodHealthEntry
	var unhealthyPods []PodHealthEntry
	totalRestarts := 0

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		entry := PodHealthEntry{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Node:      pod.Spec.NodeName,
			Phase:     string(pod.Status.Phase),
			Age:       svcAge(pvcAgeSafe(pod.CreationTimestamp.Time)),
			Issues:    []string{},
		}

		// Owner info
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				entry.Kind = ref.Kind
				entry.OwnerName = ref.Name
				break
			}
		}

		// Restart count
		for _, cs := range pod.Status.ContainerStatuses {
			entry.Restarts += int(cs.RestartCount)
			if cs.State.Waiting != nil {
				reason := cs.State.Waiting.Reason
				if reason == "CrashLoopBackOff" {
					entry.Issues = append(entry.Issues, "CrashLoopBackOff")
					result.Summary.CrashLoop++
				} else if reason == "ImagePullBackOff" {
					entry.Issues = append(entry.Issues, "ImagePullBackOff: "+cs.State.Waiting.Message)
				}
			}
		}
		totalRestarts += entry.Restarts

		// Ready status
		entry.Ready = true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				entry.Ready = false
				break
			}
		}

		// Phase classification
		result.Summary.TotalPods++
		switch pod.Status.Phase {
		case corev1.PodRunning:
			result.Summary.RunningPods++
		case corev1.PodPending:
			result.Summary.PendingPods++
			entry.Issues = append(entry.Issues, "Pod 处于 Pending 状态")
		case corev1.PodFailed:
			result.Summary.FailedPods++
			entry.Issues = append(entry.Issues, "Pod 失败")
		}

		// High restart detection
		if entry.Restarts >= 5 {
			entry.Issues = append(entry.Issues, fmt.Sprintf("重启次数 %d", entry.Restarts))
			result.Summary.HighRestart++
		}

		// Node pressure
		if nodePressure[pod.Spec.NodeName] {
			entry.Issues = append(entry.Issues, "节点资源压力")
		}

		// Health score calculation (100 = best)
		score := 100
		if pod.Status.Phase != corev1.PodRunning {
			score -= 40
		}
		if !entry.Ready {
			score -= 30
			result.Summary.NotReady++
		}
		if entry.Restarts > 0 {
			score -= entry.Restarts * 5
		}
		for range entry.Issues {
			score -= 5
		}
		if score < 0 {
			score = 0
		}
		entry.HealthScore = score

		if score >= 80 {
			result.Summary.HealthyPods++
		} else {
			result.Summary.UnhealthyPods++
			unhealthyPods = append(unhealthyPods, entry)
		}

		// Namespace aggregation
		if _, ok := nsMap[pod.Namespace]; !ok {
			nsMap[pod.Namespace] = &PodHealthNS{Namespace: pod.Namespace}
		}
		ns := nsMap[pod.Namespace]
		ns.PodCount++
		ns.TotalRestarts += entry.Restarts
		ns.AvgScore += float64(score)
		if score < 80 {
			ns.Unhealthy++
		}

		allPods = append(allPods, entry)
	}

	// Summary
	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRestart = float64(totalRestarts) / float64(result.Summary.TotalPods)
	}

	// Namespace breakdown
	for _, ns := range nsMap {
		if ns.PodCount > 0 {
			ns.AvgScore = ns.AvgScore / float64(ns.PodCount)
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].AvgScore < result.ByNamespace[j].AvgScore
	})

	// Overall score
	if result.Summary.TotalPods > 0 {
		result.HealthScore = result.Summary.HealthyPods * 100 / result.Summary.TotalPods
	}
	switch {
	case result.HealthScore >= 85:
		result.Grade = "A"
	case result.HealthScore >= 70:
		result.Grade = "B"
	case result.HealthScore >= 55:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Pods = allPods
	sort.Slice(unhealthyPods, func(i, j int) bool {
		return unhealthyPods[i].HealthScore < unhealthyPods[j].HealthScore
	})
	result.Unhealthy = unhealthyPods

	result.Recommendations = buildPodHealthRecs(&result)
	writeJSON(w, result)
}

func pvcAgeSafe(t time.Time) time.Time {
	return t
}

func buildPodHealthRecs(r *PodHealthIndexResult) []string {
	recs := []string{}
	if r.Summary.CrashLoop > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 处于 CrashLoopBackOff，建议检查应用日志", r.Summary.CrashLoop))
	}
	if r.Summary.HighRestart > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 重启次数 >= 5，存在稳定性问题", r.Summary.HighRestart))
	}
	if r.Summary.PendingPods > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 处于 Pending，可能因资源不足或调度失败", r.Summary.PendingPods))
	}
	if r.Summary.NotReady > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 未就绪，检查探针配置和应用状态", r.Summary.NotReady))
	}
	if len(recs) == 0 {
		recs = append(recs, "Pod 健康状态良好")
	}
	return recs
}
