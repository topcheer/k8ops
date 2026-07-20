package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceRequestSaturationResult analyzes resource request vs node capacity saturation.
type ResourceRequestSaturationResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         RequestSatSummary     `json:"summary"`
	ByNode          []RequestSatNodeEntry `json:"byNode"`
	ByNamespace     []RequestSatNsEntry   `json:"byNamespace"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type RequestSatSummary struct {
	TotalNodes          int     `json:"totalNodes"`
	TotalCPUReq         float64 `json:"totalCPURequest"`
	TotalMemReqGB       float64 `json:"totalMemRequestGB"`
	TotalCPUAlloc       float64 `json:"totalCPUAllocatable"`
	TotalMemAllocGB     float64 `json:"totalMemAllocatableGB"`
	CPUSaturationPct    float64 `json:"cpuSaturationPct"`
	MemSaturationPct    float64 `json:"memSaturationPct"`
	OverSubscribedNodes int     `json:"overSubscribedNodes"`
}

type RequestSatNodeEntry struct {
	NodeName       string  `json:"nodeName"`
	CPUAllocatable float64 `json:"cpuAllocatable"`
	CPURequest     float64 `json:"cpuRequest"`
	CPUSaturation  float64 `json:"cpuSaturationPct"`
	MemAllocGB     float64 `json:"memAllocatableGB"`
	MemRequestGB   float64 `json:"memRequestGB"`
	MemSaturation  float64 `json:"memSaturationPct"`
	PodCount       int     `json:"podCount"`
	RiskLevel      string  `json:"riskLevel"`
}

type RequestSatNsEntry struct {
	Namespace  string  `json:"namespace"`
	CPURequest float64 `json:"cpuRequest"`
	MemReqGB   float64 `json:"memRequestGB"`
	PodCount   int     `json:"podCount"`
}

// handleResourceRequestSaturation handles GET /api/scalability/resource-request-saturation
func (s *Server) handleResourceRequestSaturation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ResourceRequestSaturationResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node capacity and pod-to-node map
	nodeData := make(map[string]*RequestSatNodeEntry)
	for _, node := range nodes.Items {
		isMaster := false
		for k := range node.Labels {
			if k == "node-role.kubernetes.io/control-plane" || k == "node-role.kubernetes.io/master" {
				isMaster = true
			}
		}
		if isMaster {
			continue
		}
		result.Summary.TotalNodes++
		entry := &RequestSatNodeEntry{
			NodeName:       node.Name,
			CPUAllocatable: node.Status.Allocatable.Cpu().AsApproximateFloat64(),
			MemAllocGB:     float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024),
		}
		result.Summary.TotalCPUAlloc += entry.CPUAllocatable
		result.Summary.TotalMemAllocGB += entry.MemAllocGB
		nodeData[node.Name] = entry
	}

	// Aggregate requests
	nsMap := make(map[string]*RequestSatNsEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed || pod.Spec.NodeName == "" {
			continue
		}
		ne := nodeData[pod.Spec.NodeName]
		if ne == nil {
			continue
		}
		ne.PodCount++

		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &RequestSatNsEntry{Namespace: pod.Namespace}
		}
		nsMap[pod.Namespace].PodCount++

		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				v := req.AsApproximateFloat64()
				ne.CPURequest += v
				result.Summary.TotalCPUReq += v
				nsMap[pod.Namespace].CPURequest += v
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				v := float64(req.Value()) / (1024 * 1024 * 1024)
				ne.MemRequestGB += v
				result.Summary.TotalMemReqGB += v
				nsMap[pod.Namespace].MemReqGB += v
			}
		}
	}

	// Calculate saturation per node
	for _, ne := range nodeData {
		if ne.CPUAllocatable > 0 {
			ne.CPUSaturation = ne.CPURequest / ne.CPUAllocatable * 100
		}
		if ne.MemAllocGB > 0 {
			ne.MemSaturation = ne.MemRequestGB / ne.MemAllocGB * 100
		}

		if ne.CPUSaturation > 100 || ne.MemSaturation > 100 {
			ne.RiskLevel = "critical"
			result.Summary.OverSubscribedNodes++
		} else if ne.CPUSaturation > 80 || ne.MemSaturation > 80 {
			ne.RiskLevel = "high"
		} else if ne.CPUSaturation > 60 || ne.MemSaturation > 60 {
			ne.RiskLevel = "medium"
		} else {
			ne.RiskLevel = "low"
		}
		result.ByNode = append(result.ByNode, *ne)
	}

	for _, e := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].CPUSaturation > result.ByNode[j].CPUSaturation
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CPURequest > result.ByNamespace[j].CPURequest
	})

	if result.Summary.TotalCPUAlloc > 0 {
		result.Summary.CPUSaturationPct = result.Summary.TotalCPUReq / result.Summary.TotalCPUAlloc * 100
	}
	if result.Summary.TotalMemAllocGB > 0 {
		result.Summary.MemSaturationPct = result.Summary.TotalMemReqGB / result.Summary.TotalMemAllocGB * 100
	}

	sat := (result.Summary.CPUSaturationPct + result.Summary.MemSaturationPct) / 2
	result.HealthScore = 100 - int(sat)
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("资源请求饱和度: %.1f cores / %.1f (%.0f%%), %.1fGB / %.1fGB (%.0f%%)",
			result.Summary.TotalCPUReq, result.Summary.TotalCPUAlloc, result.Summary.CPUSaturationPct,
			result.Summary.TotalMemReqGB, result.Summary.TotalMemAllocGB, result.Summary.MemSaturationPct),
	}
	if result.Summary.OverSubscribedNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个节点超额订阅", result.Summary.OverSubscribedNodes))
	}
	if sat > 70 {
		result.Recommendations = append(result.Recommendations, "饱和度超过 70%, 建议扩容或优化资源请求")
	}
	writeJSON(w, result)
}
