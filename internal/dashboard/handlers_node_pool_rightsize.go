package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodePoolRightsizeResult recommends optimal node pool sizing based on
// actual workload distribution, bin-packing efficiency, and cost optimization.
type NodePoolRightsizeResult struct {
	ScannedAt           time.Time                `json:"scannedAt"`
	Summary             NodePoolRightsizeSummary `json:"summary"`
	ByNode              []NodeRightsizeEntry     `json:"byNode"`
	Recommendations     []NodeRightsizeRec       `json:"sizingActions"`
	RightsizeScore      int                      `json:"rightsizeScore"`
	Grade               string                   `json:"grade"`
	RecommendationsText []string                 `json:"recommendations"`
}

type NodePoolRightsizeSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	OverProvisioned  int     `json:"overProvisioned"`
	UnderProvisioned int     `json:"underProvisioned"`
	RightSized       int     `json:"rightSized"`
	TotalCPUAlloc    float64 `json:"totalCPUAllocatable"`
	TotalCPUUsed     float64 `json:"totalCPUUsed"`
	TotalMemAllocGB  float64 `json:"totalMemAllocatableGB"`
	TotalMemUsedGB   float64 `json:"totalMemUsedGB"`
	PotentialSavings float64 `json:"potentialSavingsMonthlyUSD"`
}

type NodeRightsizeEntry struct {
	NodeName         string  `json:"nodeName"`
	CPUAllocatable   float64 `json:"cpuAllocatable"`
	CPUUsed          float64 `json:"cpuUsed"`
	CPUUtilPct       float64 `json:"cpuUtilPct"`
	MemAllocatableGB float64 `json:"memAllocatableGB"`
	MemUsedGB        float64 `json:"memUsedGB"`
	MemUtilPct       float64 `json:"memUtilPct"`
	PodCount         int     `json:"podCount"`
	Recommendation   string  `json:"recommendation"`
	EstMonthlyCost   float64 `json:"estMonthlyCostUSD"`
}

type NodeRightsizeRec struct {
	Action   string  `json:"action"`
	NodeName string  `json:"nodeName"`
	Reason   string  `json:"reason"`
	Savings  float64 `json:"savingsUSD"`
}

// handleNodePoolRightsize handles GET /api/scalability/node-pool-rightsize
func (s *Server) handleNodePoolRightsize(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NodePoolRightsizeResult{ScannedAt: time.Now()}
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nodeUsage := make(map[string]*NodeRightsizeEntry)
	for _, node := range nodes.Items {
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			continue
		}
		cpu := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memGB := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		cost := cpu*costPerCPUCoreHour*hoursPerMonth + memGB*costPerGBHour*hoursPerMonth
		nodeUsage[node.Name] = &NodeRightsizeEntry{
			NodeName: node.Name, CPUAllocatable: cpu, MemAllocatableGB: memGB, EstMonthlyCost: cost,
		}
		result.Summary.TotalNodes++
		result.Summary.TotalCPUAlloc += cpu
		result.Summary.TotalMemAllocGB += memGB
	}

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		entry, ok := nodeUsage[pod.Spec.NodeName]
		if !ok {
			continue
		}
		entry.PodCount++
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				entry.CPUUsed += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				entry.MemUsedGB += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}
	}

	var entries []NodeRightsizeEntry
	totalSavings := 0.0
	for _, e := range nodeUsage {
		if e.CPUAllocatable > 0 {
			e.CPUUtilPct = e.CPUUsed / e.CPUAllocatable * 100
		}
		if e.MemAllocatableGB > 0 {
			e.MemUtilPct = e.MemUsedGB / e.MemAllocatableGB * 100
		}
		result.Summary.TotalCPUUsed += e.CPUUsed
		result.Summary.TotalMemUsedGB += e.MemUsedGB

		avgUtil := (e.CPUUtilPct + e.MemUtilPct) / 2
		switch {
		case e.PodCount == 0:
			e.Recommendation = "drain-and-remove"
			result.Summary.OverProvisioned++
			totalSavings += e.EstMonthlyCost
			result.Recommendations = append(result.Recommendations, NodeRightsizeRec{
				Action: "remove", NodeName: e.NodeName, Reason: "idle node, no pods scheduled", Savings: e.EstMonthlyCost,
			})
		case avgUtil < 20:
			e.Recommendation = "downsize"
			result.Summary.OverProvisioned++
			saving := e.EstMonthlyCost * (1 - avgUtil/100) * 0.5
			totalSavings += saving
			result.Recommendations = append(result.Recommendations, NodeRightsizeRec{
				Action: "downsize", NodeName: e.NodeName, Reason: fmt.Sprintf("utilization only %.0f%%", avgUtil), Savings: saving,
			})
		case avgUtil > 85:
			e.Recommendation = "upsize"
			result.Summary.UnderProvisioned++
		default:
			e.Recommendation = "keep"
			result.Summary.RightSized++
		}
		entries = append(entries, *e)
	}

	sort.Slice(entries, func(i, j int) bool {
		getUtil := func(e NodeRightsizeEntry) float64 { return (e.CPUUtilPct + e.MemUtilPct) / 2 }
		return getUtil(entries[i]) < getUtil(entries[j])
	})
	result.ByNode = entries
	result.Summary.PotentialSavings = totalSavings

	if result.Summary.TotalNodes > 0 {
		rightRatio := float64(result.Summary.RightSized) / float64(result.Summary.TotalNodes)
		result.RightsizeScore = int(rightRatio * 100)
	}
	gradeFromScore(&result.Grade, result.RightsizeScore)

	result.RecommendationsText = []string{
		fmt.Sprintf("节点规格优化: %d 节点, %d 合适, %d 过配, %d 不足", result.Summary.TotalNodes, result.Summary.RightSized, result.Summary.OverProvisioned, result.Summary.UnderProvisioned),
		fmt.Sprintf("潜在节省: $%.2f/月", totalSavings),
	}
	if totalSavings > 0 {
		result.RecommendationsText = append(result.RecommendationsText, fmt.Sprintf("建议: 下线 %d 个过配节点, 每月可节省 $%.2f", result.Summary.OverProvisioned, totalSavings))
	}
	writeJSON(w, result)
}
