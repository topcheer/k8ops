package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BinpackEfficiencyResult analyzes node bin-packing efficiency and identifies
// consolidation opportunities for cost optimization.
type BinpackEfficiencyResult struct {
	ScannedAt            time.Time          `json:"scannedAt"`
	Summary              BinpackSummary     `json:"summary"`
	ByNode               []BinpackNodeEntry `json:"byNode"`
	ConsolidationTargets []BinpackNodeEntry `json:"consolidationTargets"`
	EfficiencyScore      int                `json:"efficiencyScore"`
	Grade                string             `json:"grade"`
	Recommendations      []string           `json:"recommendations"`
}

type BinpackSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	UtilizedNodes    int     `json:"utilizedNodes"`
	IdleNodes        int     `json:"idleNodes"`
	Underutilized    int     `json:"underutilizedNodes"`
	TotalPodDensity  float64 `json:"avgPodDensity"`
	CPUUtilization   float64 `json:"avgCPUUtilizationPct"`
	MemUtilization   float64 `json:"avgMemUtilizationPct"`
	PotentialSavings int     `json:"potentialNodesToDrain"`
}

type BinpackNodeEntry struct {
	NodeName         string  `json:"nodeName"`
	Role             string  `json:"role"`
	PodCount         int     `json:"podCount"`
	CPUAllocatable   float64 `json:"cpuAllocatable"`
	CPUUsed          float64 `json:"cpuUsed"`
	CPUUtilization   float64 `json:"cpuUtilizationPct"`
	MemAllocatableGB float64 `json:"memAllocatableGB"`
	MemUsedGB        float64 `json:"memUsedGB"`
	MemUtilization   float64 `json:"memUtilizationPct"`
	BinpackScore     int     `json:"binpackScore"`
	Status           string  `json:"status"`
}

// handleBinpackEfficiency handles GET /api/scalability/binpack-efficiency
func (s *Server) handleBinpackEfficiency(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := BinpackEfficiencyResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod-to-node resource map
	nodeResMap := make(map[string]*BinpackNodeEntry)

	// Initialize node entries
	for _, node := range nodes.Items {
		// Skip cordoned/unschedulable nodes
		if node.Spec.Unschedulable {
			continue
		}
		role := "worker"
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			role = "control-plane"
		}
		if _, ok := node.Labels["node-role.kubernetes.io/master"]; ok {
			role = "master"
		}
		if role == "control-plane" || role == "master" {
			continue
		}

		cpuAlloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		memAllocGB := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)

		nodeResMap[node.Name] = &BinpackNodeEntry{
			NodeName:         node.Name,
			Role:             role,
			CPUAllocatable:   cpuAlloc,
			MemAllocatableGB: memAllocGB,
		}
	}

	// Aggregate pod resource requests per node
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		entry, ok := nodeResMap[pod.Spec.NodeName]
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

	var entries []BinpackNodeEntry
	totalCPUUtil := 0.0
	totalMemUtil := 0.0
	totalDensity := 0.0
	utilizedCount := 0

	for _, e := range nodeResMap {
		// Calculate utilization percentages
		if e.CPUAllocatable > 0 {
			e.CPUUtilization = e.CPUUsed / e.CPUAllocatable * 100
		}
		if e.MemAllocatableGB > 0 {
			e.MemUtilization = e.MemUsedGB / e.MemAllocatableGB * 100
		}

		// Binpack score: weighted average of CPU and memory utilization
		e.BinpackScore = int((e.CPUUtilization + e.MemUtilization) / 2)

		// Classify node status
		switch {
		case e.PodCount == 0:
			e.Status = "idle"
			result.Summary.IdleNodes++
		case e.BinpackScore < 25:
			e.Status = "underutilized"
			result.Summary.Underutilized++
		case e.BinpackScore < 60:
			e.Status = "moderate"
		default:
			e.Status = "packed"
		}

		if e.PodCount > 0 {
			utilizedCount++
			totalCPUUtil += e.CPUUtilization
			totalMemUtil += e.MemUtilization
			totalDensity += float64(e.PodCount)
		}

		entries = append(entries, *e)
	}

	result.Summary.TotalNodes = len(entries)
	result.Summary.UtilizedNodes = utilizedCount

	if utilizedCount > 0 {
		result.Summary.TotalPodDensity = totalDensity / float64(utilizedCount)
		result.Summary.CPUUtilization = totalCPUUtil / float64(utilizedCount)
		result.Summary.MemUtilization = totalMemUtil / float64(utilizedCount)
	}

	// Sort by binpack score ascending (worst packed first = consolidation candidates)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].BinpackScore < entries[j].BinpackScore
	})
	result.ByNode = entries

	// Identify consolidation targets (idle + underutilized)
	for _, e := range entries {
		if e.Status == "idle" || e.Status == "underutilized" {
			result.ConsolidationTargets = append(result.ConsolidationTargets, e)
		}
	}
	result.Summary.PotentialSavings = len(result.ConsolidationTargets)

	// Efficiency score: inverse of underutilized ratio
	if result.Summary.TotalNodes > 0 {
		underutilRatio := float64(result.Summary.Underutilized+result.Summary.IdleNodes) / float64(result.Summary.TotalNodes)
		result.EfficiencyScore = int((1 - underutilRatio) * 100)
	}

	switch {
	case result.EfficiencyScore >= 80:
		result.Grade = "A"
	case result.EfficiencyScore >= 60:
		result.Grade = "B"
	case result.EfficiencyScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildBinpackRecs(&result)
	writeJSON(w, result)
}

func buildBinpackRecs(r *BinpackEfficiencyResult) []string {
	recs := []string{
		fmt.Sprintf("集群装箱效率: %d/%d 节点有效利用", r.Summary.UtilizedNodes, r.Summary.TotalNodes),
	}
	if r.Summary.IdleNodes > 0 {
		recs = append(recs, fmt.Sprintf("发现 %d 个空闲节点，可立即下线节省成本", r.Summary.IdleNodes))
	}
	if r.Summary.Underutilized > 0 {
		recs = append(recs, fmt.Sprintf("%d 个低利用率节点 (<25%%)，建议合并工作负载后下线", r.Summary.Underutilized))
	}
	if r.Summary.CPUUtilization < 40 {
		recs = append(recs, fmt.Sprintf("平均 CPU 利用率仅 %.1f%%，存在过度供给", r.Summary.CPUUtilization))
	}
	if len(r.ConsolidationTargets) > 0 {
		recs = append(recs, fmt.Sprintf("整合机会: 可节省约 %d 个节点", len(r.ConsolidationTargets)))
	}
	return recs
}
