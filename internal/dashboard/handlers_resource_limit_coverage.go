package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceLimitCoverageResult audits what fraction of containers have resource requests and limits set.
type ResourceLimitCoverageResult struct {
	ScannedAt           time.Time           `json:"scannedAt"`
	Summary             RLCSummary          `json:"summary"`
	ByNamespace         []RLCNamespaceEntry `json:"byNamespace"`
	UnlimitedContainers []RLCEntry          `json:"unlimitedContainers"`
	HealthScore         int                 `json:"healthScore"`
	Grade               string              `json:"grade"`
	Recommendations     []string            `json:"recommendations"`
}

type RLCSummary struct {
	TotalContainers int     `json:"totalContainers"`
	WithCPULimit    int     `json:"withCPULimit"`
	WithMemLimit    int     `json:"withMemLimit"`
	WithCPURequest  int     `json:"withCPURequest"`
	WithMemRequest  int     `json:"withMemRequest"`
	CPULimitPct     float64 `json:"cpuLimitPct"`
	MemLimitPct     float64 `json:"memLimitPct"`
	CPUReqPct       float64 `json:"cpuRequestPct"`
	MemReqPct       float64 `json:"memRequestPct"`
}

type RLCNamespaceEntry struct {
	Namespace      string  `json:"namespace"`
	ContainerCount int     `json:"containerCount"`
	WithLimits     int     `json:"withLimits"`
	CoveragePct    float64 `json:"coveragePct"`
}

type RLCEntry struct {
	PodName    string `json:"podName"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	MissingCPU bool   `json:"missingCPULimit"`
	MissingMem bool   `json:"missingMemLimit"`
}

// handleResourceLimitCoverage handles GET /api/deployment/resource-limit-coverage
func (s *Server) handleResourceLimitCoverage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ResourceLimitCoverageResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := make(map[string]*RLCNamespaceEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			entry := RLCEntry{PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name}

			// CPU limit
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				result.Summary.WithCPULimit++
			} else {
				entry.MissingCPU = true
			}
			// Memory limit
			if _, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				result.Summary.WithMemLimit++
			} else {
				entry.MissingMem = true
			}
			// Requests
			if _, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				result.Summary.WithCPURequest++
			}
			if _, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				result.Summary.WithMemRequest++
			}

			if entry.MissingCPU || entry.MissingMem {
				result.UnlimitedContainers = append(result.UnlimitedContainers, entry)
			}

			// Namespace tracking
			if nsMap[pod.Namespace] == nil {
				nsMap[pod.Namespace] = &RLCNamespaceEntry{Namespace: pod.Namespace}
			}
			nsMap[pod.Namespace].ContainerCount++
			if !entry.MissingCPU && !entry.MissingMem {
				nsMap[pod.Namespace].WithLimits++
			}
		}
	}

	if result.Summary.TotalContainers > 0 {
		t := float64(result.Summary.TotalContainers)
		result.Summary.CPULimitPct = float64(result.Summary.WithCPULimit) / t * 100
		result.Summary.MemLimitPct = float64(result.Summary.WithMemLimit) / t * 100
		result.Summary.CPUReqPct = float64(result.Summary.WithCPURequest) / t * 100
		result.Summary.MemReqPct = float64(result.Summary.WithMemRequest) / t * 100
	}

	for _, e := range nsMap {
		if e.ContainerCount > 0 {
			e.CoveragePct = float64(e.WithLimits) / float64(e.ContainerCount) * 100
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CoveragePct < result.ByNamespace[j].CoveragePct
	})

	result.HealthScore = int(result.Summary.CPULimitPct+result.Summary.MemLimitPct) / 2
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("资源限制覆盖率: %d 容器, CPU限制 %.0f%%, 内存限制 %.0f%%, CPU请求 %.0f%%, 内存请求 %.0f%%",
			result.Summary.TotalContainers, result.Summary.CPULimitPct, result.Summary.MemLimitPct,
			result.Summary.CPUReqPct, result.Summary.MemReqPct),
	}
	if len(result.UnlimitedContainers) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个容器缺少资源限制", len(result.UnlimitedContainers)))
	}
	if result.HealthScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 为所有容器设置 resources.limits 和 resources.requests")
	}
	writeJSON(w, result)
}
