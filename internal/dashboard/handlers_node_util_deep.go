package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeUtilizationDeepResult provides a deep analysis of per-node resource
// utilization: CPU/memory requests vs capacity, pod density, IP utilization,
// and identifies which workloads consume the most on each node.
type NodeUtilizationDeepResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         NodeUtilDeepSummary `json:"summary"`
	ByNode          []NodeUtilDeepEntry `json:"byNode"`
	TopConsumers    []NodeUtilConsumer  `json:"topConsumers"`
	ImbalanceScore  int                 `json:"imbalanceScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type NodeUtilDeepSummary struct {
	TotalNodes      int     `json:"totalNodes"`
	AvgCPUUtil      float64 `json:"avgCPUUtilPct"`
	AvgMemUtil      float64 `json:"avgMemUtilPct"`
	AvgPodDensity   float64 `json:"avgPodDensityPct"`
	MaxCPUUtil      float64 `json:"maxCPUUtilPct"`
	MaxMemUtil      float64 `json:"maxMemUtilPct"`
	OverloadedNodes int     `json:"overloadedNodes"`
	IdleNodes       int     `json:"idleNodes"`
}

type NodeUtilDeepEntry struct {
	Node        string  `json:"node"`
	CPUReq      float64 `json:"cpuRequested"`
	CPUCap      float64 `json:"cpuCapacity"`
	CPUUtilPct  float64 `json:"cpuUtilPct"`
	MemReq      float64 `json:"memRequestedGB"`
	MemCap      float64 `json:"memCapacityGB"`
	MemUtilPct  float64 `json:"memUtilPct"`
	PodCount    int     `json:"podCount"`
	PodCap      int     `json:"podCapacity"`
	PodDensity  float64 `json:"podDensityPct"`
	TopConsumer string  `json:"topConsumer"`
	Status      string  `json:"status"`
}

type NodeUtilConsumer struct {
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Node      string `json:"node"`
	CPUReq    string `json:"cpuReq"`
	MemReq    string `json:"memReq"`
}

// handleNodeUtilizationDeep handles GET /api/scalability/node-utilization-deep
func (s *Server) handleNodeUtilizationDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NodeUtilizationDeepResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	type nodeAgg struct {
		cpuReq, memReq float64
		cpuCap, memCap float64
		podCap         int
		topConsumer    string
		topConsumerCPU float64
	}
	nodeMap := make(map[string]*nodeAgg)
	var workerNames []string

	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		na := &nodeAgg{
			cpuCap: n.Status.Allocatable.Cpu().AsApproximateFloat64(),
			memCap: n.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9,
		}
		if v, ok := n.Status.Allocatable["pods"]; ok {
			na.podCap = int(v.AsApproximateFloat64())
		}
		nodeMap[n.Name] = na
		workerNames = append(workerNames, n.Name)
	}

	// Aggregate pod requests per node
	nodePodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		na, ok := nodeMap[pod.Spec.NodeName]
		if !ok {
			continue
		}
		nodePodCount[pod.Spec.NodeName]++

		var podCPU, podMem float64
		for _, c := range pod.Spec.Containers {
			if v, ok := c.Resources.Requests["cpu"]; ok {
				podCPU += v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Requests["memory"]; ok {
				podMem += v.AsApproximateFloat64() / 1e9
			}
		}
		na.cpuReq += podCPU
		na.memReq += podMem

		// Track top consumer
		wlName := pod.Name
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if podCPU > na.topConsumerCPU {
			na.topConsumerCPU = podCPU
			na.topConsumer = fmt.Sprintf("%s/%s (%.2f cores)", pod.Namespace, wlName, podCPU)
		}
	}

	// Build entries
	var entries []NodeUtilDeepEntry
	var consumers []NodeUtilConsumer
	totalCPU, totalMem := 0.0, 0.0

	for _, name := range workerNames {
		na := nodeMap[name]
		pods := nodePodCount[name]
		cpuPct := 0.0
		if na.cpuCap > 0 {
			cpuPct = na.cpuReq / na.cpuCap * 100
		}
		memPct := 0.0
		if na.memCap > 0 {
			memPct = na.memReq / na.memCap * 100
		}
		podPct := 0.0
		if na.podCap > 0 {
			podPct = float64(pods) / float64(na.podCap) * 100
		}

		status := "balanced"
		if cpuPct > 80 || memPct > 80 || podPct > 80 {
			status = "overloaded"
		} else if cpuPct < 20 && memPct < 20 {
			status = "idle"
		}

		entries = append(entries, NodeUtilDeepEntry{
			Node: name, CPUReq: na.cpuReq, CPUCap: na.cpuCap, CPUUtilPct: cpuPct,
			MemReq: na.memReq, MemCap: na.memCap, MemUtilPct: memPct,
			PodCount: pods, PodCap: na.podCap, PodDensity: podPct,
			TopConsumer: na.topConsumer, Status: status,
		})

		if na.topConsumer != "" {
			consumers = append(consumers, NodeUtilConsumer{
				Node: name, Workload: na.topConsumer,
				CPUReq: fmt.Sprintf("%.2f", na.topConsumerCPU),
				MemReq: fmt.Sprintf("%.1f GB", na.memReq),
			})
		}

		totalCPU += cpuPct
		totalMem += memPct
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CPUUtilPct > entries[j].CPUUtilPct
	})
	result.ByNode = entries

	result.TopConsumers = consumers
	sort.Slice(result.TopConsumers, func(i, j int) bool {
		return result.TopConsumers[i].CPUReq > result.TopConsumers[j].CPUReq
	})

	// Summary
	wn := len(workerNames)
	result.Summary.TotalNodes = wn
	if wn > 0 {
		result.Summary.AvgCPUUtil = totalCPU / float64(wn)
		result.Summary.AvgMemUtil = totalMem / float64(wn)
	}
	for _, e := range entries {
		if e.CPUUtilPct > result.Summary.MaxCPUUtil {
			result.Summary.MaxCPUUtil = e.CPUUtilPct
		}
		if e.MemUtilPct > result.Summary.MaxMemUtil {
			result.Summary.MaxMemUtil = e.MemUtilPct
		}
		if e.Status == "overloaded" {
			result.Summary.OverloadedNodes++
		}
		if e.Status == "idle" {
			result.Summary.IdleNodes++
		}
		result.Summary.AvgPodDensity += e.PodDensity
	}
	if wn > 0 {
		result.Summary.AvgPodDensity /= float64(wn)
	}

	// Imbalance score
	result.ImbalanceScore = 100
	if result.Summary.OverloadedNodes > 0 {
		result.ImbalanceScore -= result.Summary.OverloadedNodes * 20
	}
	spread := result.Summary.MaxCPUUtil - result.Summary.AvgCPUUtil
	if spread > 30 {
		result.ImbalanceScore -= int(spread)
	}
	if result.ImbalanceScore < 0 {
		result.ImbalanceScore = 0
	}

	switch {
	case result.ImbalanceScore >= 80:
		result.Grade = "A"
	case result.ImbalanceScore >= 60:
		result.Grade = "B"
	case result.ImbalanceScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildNodeUtilDeepRecs(&result)
	writeJSON(w, result)
}

func buildNodeUtilDeepRecs(r *NodeUtilizationDeepResult) []string {
	recs := []string{
		fmt.Sprintf("平均利用率: CPU %.0f%%, Mem %.0f%%, Pod %.0f%%",
			r.Summary.AvgCPUUtil, r.Summary.AvgMemUtil, r.Summary.AvgPodDensity),
	}
	if r.Summary.OverloadedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d 个节点过载 (CPU/Mem/Pod > 80%%)", r.Summary.OverloadedNodes))
	}
	if r.Summary.IdleNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d 个节点空闲 (利用率 < 20%%)，可考虑缩容", r.Summary.IdleNodes))
	}
	spread := r.Summary.MaxCPUUtil - r.Summary.AvgCPUUtil
	if spread > 30 {
		recs = append(recs, fmt.Sprintf("CPU 利用率差异 %.0f%%，负载不均衡", spread))
	}
	return recs
}
