package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapacityPlanningReportResult1884 generates a comprehensive capacity planning report.
type CapacityPlanningReportResult1884 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	Summary         CapacityPlanReportSummary   `json:"summary"`
	CurrentUtil     CapacityPlanForecastEntry   `json:"currentUtilization"`
	Forecast3Month  CapacityPlanForecastEntry   `json:"forecast3Month"`
	Forecast6Month  CapacityPlanForecastEntry   `json:"forecast6Month"`
	ByNamespace     []CapacityPlanReportNSEntry `json:"byNamespace"`
	Recommendations []string                    `json:"recommendations"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
}

type CapacityPlanReportSummary struct {
	TotalPods          int     `json:"totalPods"`
	GrowthRateMo       float64 `json:"growthRatePerMonth"`
	CPUHeadroom        float64 `json:"cpuHeadroomCores"`
	MemHeadroomGB      float64 `json:"memHeadroomGB"`
	NodeCount          int     `json:"nodeCount"`
	NodesNeeded6Mo1884 int     `json:"nodesNeededIn6Months"`
}

type CapacityPlanForecastEntry struct {
	CPUReqCores float64 `json:"cpuRequestCores"`
	MemReqGB    float64 `json:"memRequestGB"`
	PodCount    int     `json:"podCount"`
}

type CapacityPlanReportNSEntry struct {
	Namespace  string  `json:"namespace"`
	PodCount   int     `json:"podCount"`
	CPUReq     float64 `json:"cpuRequestCores"`
	MemReqGB   float64 `json:"memRequestGB"`
	GrowthRank int     `json:"growthRank"`
}

// handleCapacityPlanningReport handles GET /api/docs/capacity-planning-report
func (s *Server) handleCapacityPlanningReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CapacityPlanningReportResult1884{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Count nodes
	for _, node := range nodes.Items {
		isMaster := false
		for k := range node.Labels {
			if k == "node-role.kubernetes.io/control-plane" || k == "node-role.kubernetes.io/master" {
				isMaster = true
			}
		}
		if !isMaster {
			result.Summary.NodeCount++
		}
	}

	// Aggregate current usage
	nsMap := make(map[string]*CapacityPlanReportNSEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.CurrentUtil.PodCount++
		result.Summary.TotalPods++

		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &CapacityPlanReportNSEntry{Namespace: pod.Namespace}
		}
		nsMap[pod.Namespace].PodCount++

		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				v := req.AsApproximateFloat64()
				result.CurrentUtil.CPUReqCores += v
				nsMap[pod.Namespace].CPUReq += v
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				v := float64(req.Value()) / (1024 * 1024 * 1024)
				result.CurrentUtil.MemReqGB += v
				nsMap[pod.Namespace].MemReqGB += v
			}
		}
	}

	// Estimate growth rate (assume 5% monthly for active namespaces)
	growthRate := 0.05
	result.Summary.GrowthRateMo = growthRate * 100

	// Forecast 3 and 6 months
	forecast3 := 1 + growthRate*3
	forecast6 := 1 + growthRate*6
	result.Forecast3Month = CapacityPlanForecastEntry{
		CPUReqCores: result.CurrentUtil.CPUReqCores * forecast3,
		MemReqGB:    result.CurrentUtil.MemReqGB * forecast3,
		PodCount:    int(float64(result.CurrentUtil.PodCount) * forecast3),
	}
	result.Forecast6Month = CapacityPlanForecastEntry{
		CPUReqCores: result.CurrentUtil.CPUReqCores * forecast6,
		MemReqGB:    result.CurrentUtil.MemReqGB * forecast6,
		PodCount:    int(float64(result.CurrentUtil.PodCount) * forecast6),
	}

	// Estimate headroom (assume 16 cores and 64GB per node)
	totalCPUAlloc := float64(result.Summary.NodeCount) * 16
	totalMemAlloc := float64(result.Summary.NodeCount) * 64
	result.Summary.CPUHeadroom = totalCPUAlloc - result.CurrentUtil.CPUReqCores
	result.Summary.MemHeadroomGB = totalMemAlloc - result.CurrentUtil.MemReqGB

	// Nodes needed in 6 months (at 70% utilization target)
	cpuNeeded6mo := result.Forecast6Month.CPUReqCores / 0.70
	result.Summary.NodesNeeded6Mo1884 = int(cpuNeeded6mo/16) + 1
	if result.Summary.NodesNeeded6Mo1884 < result.Summary.NodeCount {
		result.Summary.NodesNeeded6Mo1884 = result.Summary.NodeCount
	}

	// Build namespace entries
	for _, e := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CPUReq > result.ByNamespace[j].CPUReq
	})

	// Score based on headroom
	headroomPct := 0.0
	if totalCPUAlloc > 0 {
		headroomPct = result.Summary.CPUHeadroom / totalCPUAlloc * 100
	}
	result.HealthScore = int(headroomPct)
	if result.HealthScore > 100 {
		result.HealthScore = 100
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("容量规划: %d Pod, 当前 %.1f cores / %.1fGB, 3月预测 %.1f cores, 6月需 %d 节点",
			result.Summary.TotalPods, result.CurrentUtil.CPUReqCores, result.CurrentUtil.MemReqGB,
			result.Forecast3Month.CPUReqCores, result.Summary.NodesNeeded6Mo1884),
	}
	if result.Summary.CPUHeadroom < 0 {
		result.Recommendations = append(result.Recommendations, "CPU 已超订阅, 立即扩容")
	}
	if result.Summary.NodesNeeded6Mo1884 > result.Summary.NodeCount {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("6个月内需要增加 %d 个节点", result.Summary.NodesNeeded6Mo1884-result.Summary.NodeCount))
	}
	writeJSON(w, result)
}
