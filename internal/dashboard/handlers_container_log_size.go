package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContainerLogSizeResult estimates container log disk usage and identifies oversized logs.
type ContainerLogSizeResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         ContainerLogSizeSummary `json:"summary"`
	ByNamespace     []ContainerLogNSEntry   `json:"byNamespace"`
	HighLogPods     []ContainerLogEntry     `json:"highLogPods"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type ContainerLogSizeSummary struct {
	TotalPods     int     `json:"totalPods"`
	WithLogLimit  int     `json:"podsWithLogLimit"`
	HighVerbosity int     `json:"highVerbosityPods"`
	NoLogPolicy   int     `json:"withoutLogPolicy"`
	EstTotalLogGB float64 `json:"estimatedTotalLogGB"`
}

type ContainerLogNSEntry struct {
	Namespace string  `json:"namespace"`
	PodCount  int     `json:"podCount"`
	EstLogGB  float64 `json:"estimatedLogGB"`
	RiskLevel string  `json:"riskLevel"`
}

type ContainerLogEntry struct {
	PodName     string  `json:"podName"`
	Namespace   string  `json:"namespace"`
	Containers  int     `json:"containerCount"`
	Restarts    int     `json:"restarts"`
	EstLogMB    float64 `json:"estimatedLogMB"`
	HasLogLimit bool    `json:"hasLogLimit"`
	RiskLevel   string  `json:"riskLevel"`
}

// handleContainerLogSize handles GET /api/operations/container-log-size
func (s *Server) handleContainerLogSize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ContainerLogSizeResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := make(map[string]*ContainerLogNSEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += int(cs.RestartCount)
		}

		// Estimate log size: ~1MB per restart + 5MB base per container
		containerCount := len(pod.Spec.Containers)
		estLogMB := float64(containerCount*5) + float64(totalRestarts)*1.0

		// Check for log limits in pod annotations
		hasLogLimit := false
		for k := range pod.Annotations {
			if containsStr1876(k, "log") && (containsStr1876(k, "limit") || containsStr1876(k, "max")) {
				hasLogLimit = true
				break
			}
		}

		entry := ContainerLogEntry{
			PodName: pod.Name, Namespace: pod.Namespace,
			Containers: containerCount, Restarts: totalRestarts,
			EstLogMB: estLogMB, HasLogLimit: hasLogLimit,
		}

		if hasLogLimit {
			result.Summary.WithLogLimit++
		} else {
			result.Summary.NoLogPolicy++
		}

		// Risk based on log size estimate
		switch {
		case estLogMB > 100:
			entry.RiskLevel = "high"
			result.Summary.HighVerbosity++
			result.HighLogPods = append(result.HighLogPods, entry)
		case estLogMB > 50:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		// Namespace tracking
		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &ContainerLogNSEntry{Namespace: pod.Namespace}
		}
		nsMap[pod.Namespace].PodCount++
		nsMap[pod.Namespace].EstLogGB += estLogMB / 1024
		result.Summary.EstTotalLogGB += estLogMB / 1024
	}

	for _, e := range nsMap {
		switch {
		case e.EstLogGB > 5:
			e.RiskLevel = "high"
		case e.EstLogGB > 1:
			e.RiskLevel = "medium"
		default:
			e.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].EstLogGB > result.ByNamespace[j].EstLogGB
	})

	if result.Summary.TotalPods > 0 {
		result.HealthScore = result.Summary.WithLogLimit * 100 / result.Summary.TotalPods
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("容器日志大小: %d Pod, %d 有日志策略, %d 无策略, %d 高冗余, 预估 %.1fGB",
			result.Summary.TotalPods, result.Summary.WithLogLimit,
			result.Summary.NoLogPolicy, result.Summary.HighVerbosity,
			result.Summary.EstTotalLogGB),
	}
	if result.Summary.NoLogPolicy > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Pod 无日志轮转策略, 日志可能占满磁盘", result.Summary.NoLogPolicy))
	}
	writeJSON(w, result)
}
