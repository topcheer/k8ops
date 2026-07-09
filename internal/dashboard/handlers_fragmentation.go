package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FragResult is the resource fragmentation & bin-packing efficiency analysis.
type FragResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         FragSummary    `json:"summary"`
	ByNode          []FragNodeStat `json:"byNode"`
	FragmentedNodes []FragNodeStat `json:"fragmentedNodes"`
	StrandedNodes   []FragNodeStat `json:"strandedNodes"` // nodes with stranded resources
	PodSimulations  []FragPodSim   `json:"podSimulations"`
	Recommendations []string       `json:"recommendations"`
}

// FragSummary aggregates cluster-wide fragmentation metrics.
type FragSummary struct {
	TotalNodes         int     `json:"totalNodes"`
	SchedulableNodes   int     `json:"schedulableNodes"`
	AvgCPUEfficiency   float64 `json:"avgCpuEfficiency"` // requested / allocatable (%)
	AvgMemEfficiency   float64 `json:"avgMemEfficiency"`
	AvgPodSlotUsage    float64 `json:"avgPodSlotUsage"`    // pods / max pods (%)
	FragmentedNodes    int     `json:"fragmentedNodes"`    // nodes with high internal fragmentation
	StrandedCPUMilli   int64   `json:"strandedCPUMilli"`   // total CPU that can't be used due to fragmentation
	StrandedMemMi      int64   `json:"strandedMemMi"`      // total memory that can't be used
	BinPackingScore    int     `json:"binPackingScore"`    // 0-100 (100 = optimal packing)
	FragmentationScore int     `json:"fragmentationScore"` // 0-100 (100 = no fragmentation)
}

// FragNodeStat describes fragmentation for one node.
type FragNodeStat struct {
	NodeName          string  `json:"nodeName"`
	AllocatableCPU    int64   `json:"allocatableCPUMilli"`
	AllocatableMem    int64   `json:"allocatableMemMi"`
	RequestedCPU      int64   `json:"requestedCPUMilli"`
	RequestedMem      int64   `json:"requestedMemMi"`
	AvailableCPU      int64   `json:"availableCPUMilli"`
	AvailableMem      int64   `json:"availableMemMi"`
	MaxPods           int64   `json:"maxPods"`
	CurrentPods       int     `json:"currentPods"`
	CPUEfficiency     float64 `json:"cpuEfficiency"`      // requested / allocatable (%)
	MemEfficiency     float64 `json:"memEfficiency"`      // requested / allocatable (%)
	PodSlotUsage      float64 `json:"podSlotUsage"`       // pods / max (%)
	LargestFitCPU     int64   `json:"largestFitCPUMilli"` // largest CPU-only pod that fits
	LargestFitMem     int64   `json:"largestFitMemMi"`    // largest mem-only pod that fits
	FragScore         int     `json:"fragScore"`          // 0-100 (higher = more fragmented)
	StrandedResources bool    `json:"strandedResources"`  // has allocatable space but can't fit typical pod
}

// FragPodSim simulates whether a pod of a given size can be scheduled.
type FragPodSim struct {
	PodSize       string  `json:"podSize"` // small / medium / large / xlarge
	CPUMilli      int64   `json:"cpuMilli"`
	MemMi         int64   `json:"memMi"`
	NodesThatFit  int     `json:"nodesThatFit"` // how many nodes can fit this pod
	TotalNodes    int     `json:"totalNodes"`
	FitPercent    float64 `json:"fitPercent"`
	BlockedReason string  `json:"blockedReason"` // why it can't fit on some nodes
}

// handleFragmentation analyzes resource fragmentation and bin-packing efficiency.
// GET /api/scalability/fragmentation
func (s *Server) handleFragmentation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build pod resource requests per node
	nodeRequests := map[string]*nodeResourceTotals{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		nrt, ok := nodeRequests[pod.Spec.NodeName]
		if !ok {
			nrt = &nodeResourceTotals{}
			nodeRequests[pod.Spec.NodeName] = nrt
		}
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Requests.Cpu().IsZero() {
				nrt.cpuMilli += c.Resources.Requests.Cpu().MilliValue()
			}
			if !c.Resources.Requests.Memory().IsZero() {
				nrt.memMi += c.Resources.Requests.Memory().Value() / (1024 * 1024)
			}
		}
		nrt.podCount++
	}

	now := time.Now()
	result := FragResult{ScannedAt: now}

	// Standard pod sizes for simulation
	podSims := []struct {
		name     string
		cpuMilli int64
		memMi    int64
	}{
		{"small", 100, 128},
		{"medium", 500, 512},
		{"large", 1000, 2048},
		{"xlarge", 4000, 8192},
	}

	var totalCPUEff, totalMemEff, totalPodSlot float64
	var schedulableCount int

	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}

		allocatableCPU := node.Status.Allocatable.Cpu().MilliValue()
		allocatableMem := node.Status.Allocatable.Memory().Value() / (1024 * 1024)
		maxPods := node.Status.Allocatable.Pods().Value()

		nrt := nodeRequests[node.Name]
		reqCPU := int64(0)
		reqMem := int64(0)
		podCount := 0
		if nrt != nil {
			reqCPU = nrt.cpuMilli
			reqMem = nrt.memMi
			podCount = nrt.podCount
		}

		availCPU := allocatableCPU - reqCPU
		if availCPU < 0 {
			availCPU = 0
		}
		availMem := allocatableMem - reqMem
		if availMem < 0 {
			availMem = 0
		}
		availPods := maxPods - int64(podCount)
		if availPods < 0 {
			availPods = 0
		}

		cpuEff := 0.0
		if allocatableCPU > 0 {
			cpuEff = float64(reqCPU) / float64(allocatableCPU) * 100
		}
		memEff := 0.0
		if allocatableMem > 0 {
			memEff = float64(reqMem) / float64(allocatableMem) * 100
		}
		podSlot := 0.0
		if maxPods > 0 {
			podSlot = float64(podCount) / float64(maxPods) * 100
		}

		// Fragmentation score: how much wasted space is there relative to available?
		// A node is fragmented if it has significant available resources but can't
		// accommodate pods of common sizes due to one resource being exhausted.
		fragScore := computeFragScore(availCPU, availMem, availPods, allocatableCPU, allocatableMem, maxPods)

		// Stranded resources: has CPU available but no pod slots, or vice versa
		stranded := (availCPU > 500 && availMem > 512 && availPods == 0) ||
			(availCPU > 500 && availPods > 0 && availMem < 128) ||
			(availMem > 512 && availPods > 0 && availCPU < 100)

		ns := FragNodeStat{
			NodeName:          node.Name,
			AllocatableCPU:    allocatableCPU,
			AllocatableMem:    allocatableMem,
			RequestedCPU:      reqCPU,
			RequestedMem:      reqMem,
			AvailableCPU:      availCPU,
			AvailableMem:      availMem,
			MaxPods:           maxPods,
			CurrentPods:       podCount,
			CPUEfficiency:     roundTo2(cpuEff),
			MemEfficiency:     roundTo2(memEff),
			PodSlotUsage:      roundTo2(podSlot),
			LargestFitCPU:     availCPU,
			LargestFitMem:     availMem,
			FragScore:         fragScore,
			StrandedResources: stranded,
		}

		result.ByNode = append(result.ByNode, ns)

		if fragScore > 50 {
			result.Summary.FragmentedNodes++
			result.FragmentedNodes = append(result.FragmentedNodes, ns)
		}
		if stranded {
			result.StrandedNodes = append(result.StrandedNodes, ns)
			// Count stranded resources
			result.Summary.StrandedCPUMilli += availCPU
			result.Summary.StrandedMemMi += availMem
		}

		totalCPUEff += cpuEff
		totalMemEff += memEff
		totalPodSlot += podSlot
		schedulableCount++
		result.Summary.TotalNodes++
	}

	result.Summary.SchedulableNodes = schedulableCount
	if schedulableCount > 0 {
		result.Summary.AvgCPUEfficiency = roundTo2(totalCPUEff / float64(schedulableCount))
		result.Summary.AvgMemEfficiency = roundTo2(totalMemEff / float64(schedulableCount))
		result.Summary.AvgPodSlotUsage = roundTo2(totalPodSlot / float64(schedulableCount))
	}

	// Pod simulations
	for _, sim := range podSims {
		fitCount := 0
		blockedReason := ""
		for _, ns := range result.ByNode {
			if ns.AvailableCPU >= sim.cpuMilli && ns.AvailableMem >= sim.memMi {
				// Check pod slot availability (assume 1 slot needed)
				if int64(ns.CurrentPods) < ns.MaxPods {
					fitCount++
				}
			} else {
				if ns.AvailableCPU < sim.cpuMilli {
					blockedReason = "insufficient CPU on some nodes"
				} else if ns.AvailableMem < sim.memMi {
					blockedReason = "insufficient memory on some nodes"
				}
			}
		}
		fitPct := 0.0
		if len(result.ByNode) > 0 {
			fitPct = float64(fitCount) / float64(len(result.ByNode)) * 100
		}
		result.PodSimulations = append(result.PodSimulations, FragPodSim{
			PodSize:       sim.name,
			CPUMilli:      sim.cpuMilli,
			MemMi:         sim.memMi,
			NodesThatFit:  fitCount,
			TotalNodes:    len(result.ByNode),
			FitPercent:    roundTo2(fitPct),
			BlockedReason: blockedReason,
		})
	}

	// Sort nodes by fragmentation score (highest first)
	sort.Slice(result.FragmentedNodes, func(i, j int) bool {
		return result.FragmentedNodes[i].FragScore > result.FragmentedNodes[j].FragScore
	})
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].FragScore > result.ByNode[j].FragScore
	})
	if len(result.ByNode) > 30 {
		result.ByNode = result.ByNode[:30]
	}

	// Compute scores
	result.Summary.BinPackingScore = fragBinPackingScore(result.Summary)
	result.Summary.FragmentationScore = fragFragmentationScore(result.Summary)

	result.Recommendations = fragRecommendations(&result)

	writeJSON(w, result)
}

// nodeResourceTotals tracks aggregate resource usage per node.
type nodeResourceTotals struct {
	cpuMilli int64
	memMi    int64
	podCount int
}

// computeFragScore calculates a 0-100 fragmentation score for a node.
// Higher score = more fragmented (resources available but can't be used efficiently).
func computeFragScore(availCPU, availMem int64, availPods, allocCPU, allocMem, maxPods int64) int {
	if allocCPU == 0 || allocMem == 0 {
		return 0
	}

	cpuAvailRatio := float64(availCPU) / float64(allocCPU)
	memAvailRatio := float64(availMem) / float64(allocMem)

	// Pod slot pressure: no slots left but resources available
	podSlotPressure := 0
	if maxPods > 0 {
		podSlotUsedRatio := 1.0 - float64(availPods)/float64(maxPods)
		if availPods == 0 && (cpuAvailRatio > 0.2 || memAvailRatio > 0.2) {
			podSlotPressure = 50 // significant: resources wasted due to pod limit
		} else if podSlotUsedRatio > 0.9 && availPods <= 2 {
			podSlotPressure = 25
		}
	}

	// Resource imbalance: CPU available but not memory, or vice versa
	imbalance := 0
	if cpuAvailRatio > 0.3 && memAvailRatio < 0.05 {
		imbalance = 30
	} else if memAvailRatio > 0.3 && cpuAvailRatio < 0.05 {
		imbalance = 30
	} else if cpuAvailRatio > 0.2 && memAvailRatio < 0.1 {
		imbalance = 15
	} else if memAvailRatio > 0.2 && cpuAvailRatio < 0.1 {
		imbalance = 15
	}

	score := podSlotPressure + imbalance
	if score > 100 {
		score = 100
	}
	return score
}

// fragBinPackingScore computes overall bin-packing efficiency (0-100).
// 100 = resources are optimally packed (high utilization).
func fragBinPackingScore(s FragSummary) int {
	if s.SchedulableNodes == 0 {
		return 100
	}

	// Average of CPU and memory efficiency
	avgEff := (s.AvgCPUEfficiency + s.AvgMemEfficiency) / 2

	// Penalize fragmentation
	penalty := float64(s.FragmentedNodes) / float64(s.SchedulableNodes) * 20

	score := int(avgEff - penalty)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// fragFragmentationScore computes inverse fragmentation (0-100).
// 100 = no fragmentation at all.
func fragFragmentationScore(s FragSummary) int {
	if s.SchedulableNodes == 0 {
		return 100
	}

	fragRatio := float64(s.FragmentedNodes) / float64(s.SchedulableNodes)
	strandedCPUImpact := 0.0
	if s.AvgCPUEfficiency > 0 {
		strandedCPUImpact = float64(s.StrandedCPUMilli) / (float64(s.StrandedCPUMilli) + 1) * 10
	}

	score := 100 - int(fragRatio*80) - int(strandedCPUImpact)
	if score < 0 {
		score = 0
	}
	return score
}

// fragRecommendations generates actionable recommendations.
func fragRecommendations(r *FragResult) []string {
	var recs []string

	if r.Summary.FragmentedNodes > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) show resource fragmentation — consider draining and rescheduling to improve bin-packing efficiency",
			r.Summary.FragmentedNodes,
		))
	}

	if r.Summary.StrandedCPUMilli > 0 {
		recs = append(recs, fmt.Sprintf(
			"%.1f CPU cores and %d MiB memory are stranded across nodes (available but unusable due to pod limits or resource imbalance)",
			float64(r.Summary.StrandedCPUMilli)/1000.0,
			r.Summary.StrandedMemMi,
		))
	}

	// Check if pod slots are the bottleneck
	for _, sim := range r.PodSimulations {
		if sim.NodesThatFit == 0 && sim.PodSize != "xlarge" {
			recs = append(recs, fmt.Sprintf(
				"No nodes can fit a %s pod (%dm CPU, %dMi mem) — cluster needs scale-out or resource rebalancing",
				sim.PodSize, sim.CPUMilli, sim.MemMi,
			))
			break
		}
	}

	if r.Summary.BinPackingScore < 50 {
		recs = append(recs, "Low bin-packing efficiency — consider using Cluster Autoscaler with bin-packing optimization or a descheduler")
	}

	if r.Summary.AvgPodSlotUsage > 80 && r.Summary.AvgCPUEfficiency < 50 {
		recs = append(recs, fmt.Sprintf(
			"Pod slot usage at %.0f%% but CPU efficiency only %.0f%% — increase max-pods limit or reduce pod count per node",
			r.Summary.AvgPodSlotUsage, r.Summary.AvgCPUEfficiency,
		))
	}

	if r.Summary.FragmentationScore < 50 {
		recs = append(recs, "High fragmentation detected — run a descheduler (e.g., descheduler LowNodeUtilization strategy) to rebalance pods")
	}

	if len(recs) == 0 {
		recs = append(recs, "Resource fragmentation is low — pods are well distributed across nodes")
	}

	return recs
}

// Ensures resource import is used (for future extensions).
var _ = resource.MustParse
