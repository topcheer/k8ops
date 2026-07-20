package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OvercommitRiskResult evaluates cluster overcommit risk by comparing
// resource requests vs limits vs actual allocatable capacity.
type OvercommitRiskResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         OvercommitRiskSummary `json:"summary"`
	ByNamespace     []OvercommitNsEntry   `json:"byNamespace"`
	RiskScore       int                   `json:"riskScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type OvercommitRiskSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	TotalCPUAlloc    float64 `json:"totalCPUAllocatable"`
	TotalMemAllocGB  float64 `json:"totalMemAllocatableGB"`
	CPURequested     float64 `json:"cpuRequested"`
	MemRequestedGB   float64 `json:"memRequestedGB"`
	CPULimited       float64 `json:"cpuLimited"`
	MemLimitedGB     float64 `json:"memLimitedGB"`
	CPUOvercommitPct float64 `json:"cpuOvercommitPct"`
	MemOvercommitPct float64 `json:"memOvercommitPct"`
}

type OvercommitNsEntry struct {
	Namespace     string  `json:"namespace"`
	CPURequest    float64 `json:"cpuRequest"`
	CPULimit      float64 `json:"cpuLimit"`
	MemRequestGB  float64 `json:"memRequestGB"`
	MemLimitGB    float64 `json:"memLimitGB"`
	LimitReqRatio float64 `json:"limitRequestRatio"`
	RiskLevel     string  `json:"riskLevel"`
}

// handleOvercommitRisk handles GET /api/scalability/overcommit-risk
func (s *Server) handleOvercommitRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := OvercommitRiskResult{ScannedAt: time.Now()}
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	for _, node := range nodes.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		result.Summary.TotalNodes++
		result.Summary.TotalCPUAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		result.Summary.TotalMemAllocGB += float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
	}

	nsMap := make(map[string]*OvercommitNsEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if _, ok := nsMap[pod.Namespace]; !ok {
			nsMap[pod.Namespace] = &OvercommitNsEntry{Namespace: pod.Namespace}
		}
		e := nsMap[pod.Namespace]
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				e.CPURequest += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				e.MemRequestGB += float64(req.Value()) / (1024 * 1024 * 1024)
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				e.CPULimit += lim.AsApproximateFloat64()
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				e.MemLimitGB += float64(lim.Value()) / (1024 * 1024 * 1024)
			}
			result.Summary.CPURequested = addToTotal(result.Summary.CPURequested, c, corev1.ResourceCPU, true)
			result.Summary.CPULimited = addToTotal(result.Summary.CPULimited, c, corev1.ResourceCPU, false)
			result.Summary.MemRequestedGB += getMem(c, true)
			result.Summary.MemLimitedGB += getMem(c, false)
		}
	}

	var entries []OvercommitNsEntry
	for _, e := range nsMap {
		if e.CPURequest > 0 {
			e.LimitReqRatio = e.CPULimit / e.CPURequest
		}
		switch {
		case e.LimitReqRatio > 5:
			e.RiskLevel = "critical"
		case e.LimitReqRatio > 3:
			e.RiskLevel = "high"
		case e.LimitReqRatio > 1.5:
			e.RiskLevel = "medium"
		default:
			e.RiskLevel = "low"
		}
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].LimitReqRatio > entries[j].LimitReqRatio })
	result.ByNamespace = entries

	if result.Summary.TotalCPUAlloc > 0 {
		result.Summary.CPUOvercommitPct = result.Summary.CPULimited / result.Summary.TotalCPUAlloc * 100
	}
	if result.Summary.TotalMemAllocGB > 0 {
		result.Summary.MemOvercommitPct = result.Summary.MemLimitedGB / result.Summary.TotalMemAllocGB * 100
	}

	result.RiskScore = 100
	if result.Summary.CPUOvercommitPct > 200 {
		result.RiskScore -= 30
	}
	if result.Summary.MemOvercommitPct > 200 {
		result.RiskScore -= 30
	}
	if result.Summary.CPUOvercommitPct > 300 {
		result.RiskScore -= 20
	}
	if result.RiskScore < 0 {
		result.RiskScore = 0
	}
	gradeFromScore(&result.Grade, result.RiskScore)

	result.Recommendations = []string{
		fmt.Sprintf("超配风险: CPU 限制 %.1f / 可分配 %.1f (%.0f%%), 内存限制 %.1fGB / 可分配 %.1fGB (%.0f%%)",
			result.Summary.CPULimited, result.Summary.TotalCPUAlloc, result.Summary.CPUOvercommitPct,
			result.Summary.MemLimitedGB, result.Summary.TotalMemAllocGB, result.Summary.MemOvercommitPct),
	}
	if result.Summary.CPUOvercommitPct > 200 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("CPU 超配 %.0f%% 过高, 存在 throttle 风险", result.Summary.CPUOvercommitPct))
	}
	if result.Summary.MemOvercommitPct > 200 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("内存超配 %.0f%% 过高, OOM 风险高", result.Summary.MemOvercommitPct))
	}
	if result.RiskScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 降低 limit/request 比率, 确保 limits 不超过节点容量")
	}
	writeJSON(w, result)
}

func addToTotal(total float64, c corev1.Container, res corev1.ResourceName, isRequest bool) float64 {
	if isRequest {
		if v, ok := c.Resources.Requests[res]; ok {
			return total + v.AsApproximateFloat64()
		}
	} else {
		if v, ok := c.Resources.Limits[res]; ok {
			return total + v.AsApproximateFloat64()
		}
	}
	return total
}

func getMem(c corev1.Container, isRequest bool) float64 {
	if isRequest {
		if v, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			return float64(v.Value()) / (1024 * 1024 * 1024)
		}
	} else {
		if v, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			return float64(v.Value()) / (1024 * 1024 * 1024)
		}
	}
	return 0
}
