package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodStormResult detects pod restart storms - cascading restarts
// across multiple workloads within a short window indicating systemic issues.
type PodStormResult struct {
	ScannedAt        time.Time              `json:"scannedAt"`
	Summary          PodRestartStormSummary `json:"summary"`
	StormDetected    bool                   `json:"stormDetected"`
	ByWorkload       []StormEntry           `json:"byWorkload"`
	CorrelatedEvents []StormCorrelation     `json:"correlatedEvents"`
	StormScore       int                    `json:"stormScore"`
	Grade            string                 `json:"grade"`
	Recommendations  []string               `json:"recommendations"`
}

type PodRestartStormSummary struct {
	TotalPods         int     `json:"totalPods"`
	RestartingPods    int     `json:"restartingPods"`
	TotalRestarts     int     `json:"totalRestarts"`
	WorkloadsAffected int     `json:"workloadsAffected"`
	NamespacesHit     int     `json:"namespacesHit"`
	MaxPerPod         int     `json:"maxRestartsPerPod"`
	AvgRestarts       float64 `json:"avgRestartsPerPod"`
	StormSeverity     string  `json:"stormSeverity"`
}

type StormEntry struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Restarts  int    `json:"restarts"`
	PodCount  int    `json:"podCount"`
	Severity  string `json:"severity"`
}

type StormCorrelation struct {
	Namespace   string   `json:"namespace"`
	Pattern     string   `json:"pattern"`
	Workloads   []string `json:"workloads"`
	LikelyCause string   `json:"likelyCause"`
}

// handlePodRestartStorm handles GET /api/operations/pod-restart-storm
func (s *Server) handlePodRestartStorm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PodStormResult{ScannedAt: time.Now()}
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	wlMap := make(map[string]*StormEntry)
	nsSet := make(map[string]bool)
	totalRestarts := 0
	maxRestarts := 0

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}

		podRestarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			podRestarts += int(cs.RestartCount)
		}

		if podRestarts > 0 {
			result.Summary.RestartingPods++
			totalRestarts += podRestarts
			if podRestarts > maxRestarts {
				maxRestarts = podRestarts
			}
			nsSet[pod.Namespace] = true

			key := pod.Namespace + "/" + wlName
			if _, ok := wlMap[key]; !ok {
				wlMap[key] = &StormEntry{Workload: wlName, Namespace: pod.Namespace}
			}
			wlMap[key].Restarts += podRestarts
			wlMap[key].PodCount++
		}
	}

	result.Summary.TotalRestarts = totalRestarts
	result.Summary.MaxPerPod = maxRestarts
	result.Summary.NamespacesHit = len(nsSet)
	result.Summary.WorkloadsAffected = len(wlMap)
	if result.Summary.RestartingPods > 0 {
		result.Summary.AvgRestarts = float64(totalRestarts) / float64(result.Summary.RestartingPods)
	}

	var entries []StormEntry
	for _, e := range wlMap {
		switch {
		case e.Restarts > 50:
			e.Severity = "critical"
		case e.Restarts > 20:
			e.Severity = "high"
		case e.Restarts > 5:
			e.Severity = "medium"
		default:
			e.Severity = "low"
		}
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Restarts > entries[j].Restarts })
	result.ByWorkload = entries

	// Storm detection: >30% pods restarting OR >5 workloads affected OR avg >5
	restartRatio := float64(result.Summary.RestartingPods) / float64(maxInt(result.Summary.TotalPods, 1))
	if restartRatio > 0.3 || result.Summary.WorkloadsAffected > 5 || result.Summary.AvgRestarts > 5 {
		result.StormDetected = true
		switch {
		case restartRatio > 0.5 || result.Summary.WorkloadsAffected > 10:
			result.Summary.StormSeverity = "critical"
		case restartRatio > 0.3:
			result.Summary.StormSeverity = "high"
		default:
			result.Summary.StormSeverity = "medium"
		}
	} else {
		result.Summary.StormSeverity = "none"
	}

	// Correlation analysis
	for _, e := range entries {
		if e.Severity == "critical" || e.Severity == "high" {
			result.CorrelatedEvents = append(result.CorrelatedEvents, StormCorrelation{
				Namespace: e.Namespace, Pattern: "concurrent-restarts",
				Workloads: []string{e.Workload}, LikelyCause: "check node pressure, config changes, or dependency failures",
			})
			break
		}
	}

	// Score
	if result.Summary.TotalPods > 0 {
		result.StormScore = int((1 - restartRatio) * 100)
	}
	gradeFromScore(&result.Grade, result.StormScore)

	result.Recommendations = []string{
		fmt.Sprintf("重启风暴检测: %d/%d Pod 重启 (%.1f%%), %d 工作负载受影响", result.Summary.RestartingPods, result.Summary.TotalPods, restartRatio*100, result.Summary.WorkloadsAffected),
	}
	if result.StormDetected {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("警告: 检测到重启风暴 (严重度: %s)", result.Summary.StormSeverity))
	}
	if len(entries) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("最严重: %s/%s (%d 次重启)", entries[0].Namespace, entries[0].Workload, entries[0].Restarts))
	}
	if result.StormScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 检查节点资源压力, 配置变更, 和外部依赖可用性")
	}
	writeJSON(w, result)
}
