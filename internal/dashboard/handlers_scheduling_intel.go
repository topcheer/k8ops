package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SchedulingIntelResult analyzes node bin-packing efficiency, resource fragmentation,
// and scheduling bottlenecks across the cluster.
type SchedulingIntelResult struct {
	ScannedAt         time.Time          `json:"scannedAt"`
	Summary           SchedSummary       `json:"summary"`
	ByNode            []NodePackAnalysis `json:"byNode"`
	FragileNodes      []FragileNode      `json:"fragileNodes"`
	StrandedResources []StrandedResource `json:"strandedResources"`
	SchedulingScore   int                `json:"schedulingScore"`
	Grade             string             `json:"grade"`
	Recommendations   []string           `json:"recommendations"`
}

// SchedSummary aggregates scheduling statistics.
type SchedSummary struct {
	TotalNodes         int     `json:"totalNodes"`
	UnderutilizedNodes int     `json:"underutilizedNodes"` // <30% utilized
	OverloadedNodes    int     `json:"overloadedNodes"`    // >85% utilized
	FragileNodes       int     `json:"fragileNodes"`       // can't fit any standard pod
	AvgCPUPacking      float64 `json:"avgCPUPacking"`      // % of allocatable CPU used
	AvgMemPacking      float64 `json:"avgMemPacking"`      // % of allocatable memory used
	BinPackScore       int     `json:"binPackScore"`       // 0-100 packing efficiency
	StrandedCPUMilli   int64   `json:"strandedCPUMilli"`   // total stranded CPU (fragmented)
	StrandedMemMi      int64   `json:"strandedMemMi"`      // total stranded memory
	FitAssessment      string  `json:"fitAssessment"`      // can a standard pod fit on any node?
}

// NodePackAnalysis shows bin-packing efficiency for one node.
type NodePackAnalysis struct {
	Name           string  `json:"name"`
	AllocatableCPU string  `json:"allocatableCPU"`
	AllocatableMem string  `json:"allocatableMem"`
	RequestedCPU   string  `json:"requestedCPU"`
	RequestedMem   string  `json:"requestedMem"`
	CPUUsagePct    float64 `json:"cpuUsagePct"`
	MemUsagePct    float64 `json:"memUsagePct"`
	PodCount       int     `json:"podCount"`
	Capacity       int     `json:"podCapacity"`
	PodDensity     float64 `json:"podDensity"` // pods/capacity %
	Status         string  `json:"status"`     // optimal, underutilized, overloaded, fragile
	LargestFitCPU  string  `json:"largestFitCPU"`
	LargestFitMem  string  `json:"largestFitMem"`
}

// FragileNode is a node with too much fragmentation to schedule new pods.
type FragileNode struct {
	Name        string  `json:"name"`
	CPUUsagePct float64 `json:"cpuUsagePct"`
	MemUsagePct float64 `json:"memUsagePct"`
	Reason      string  `json:"reason"`
	StrandedCPU string  `json:"strandedCPU"`
	StrandedMem string  `json:"strandedMem"`
}

// StrandedResource quantifies wasted allocatable resources.
type StrandedResource struct {
	Node   string `json:"node"`
	Type   string `json:"type"`   // cpu or memory
	Amount string `json:"amount"` // human-readable stranded amount
	Reason string `json:"reason"` // fragmentation, pod-limit, etc.
}

// Standard pod sizes for fit assessment
var stdPodSizes = []struct {
	Name string
	CPU  int64 // milli-cores
	Mem  int64 // MiB
}{
	{"nano", 25, 64},
	{"micro", 50, 128},
	{"small", 100, 256},
	{"medium", 250, 512},
	{"large", 500, 1024},
	{"xl", 1000, 2048},
}

// handleSchedulingIntel provides scheduling intelligence & bin-packing efficiency analysis.
// GET /api/scalability/scheduling-intel
func (s *Server) handleSchedulingIntel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SchedulingIntelResult{ScannedAt: time.Now()}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list nodes")
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build per-node resource usage map
	type nodeUsage struct {
		reqCPU   int64 // milli-cores
		reqMem   int64 // MiB
		podCount int
	}
	nodeUsageMap := make(map[string]*nodeUsage)

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			continue
		}
		nu, ok := nodeUsageMap[nodeName]
		if !ok {
			nu = &nodeUsage{}
			nodeUsageMap[nodeName] = nu
		}
		nu.podCount++
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					nu.reqCPU += cpu.MilliValue()
				}
				if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					nu.reqMem += mem.Value() / (1024 * 1024)
				}
			}
		}
	}

	totalCPUUsage := 0.0
	totalMemUsage := 0.0
	totalStrandedCPU := int64(0)
	totalStrandedMem := int64(0)

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++

		allocCPU := node.Status.Allocatable[corev1.ResourceCPU]
		allocMem := node.Status.Allocatable[corev1.ResourceMemory]
		allocCPUMilli := allocCPU.MilliValue()
		allocMemMi := allocMem.Value() / (1024 * 1024)

		// Pod capacity
		podCap := 110
		if cap, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			podCap = int(cap.Value())
		}

		nu := nodeUsageMap[node.Name]
		if nu == nil {
			nu = &nodeUsage{}
		}

		cpuPct := 0.0
		memPct := 0.0
		if allocCPUMilli > 0 {
			cpuPct = float64(nu.reqCPU) / float64(allocCPUMilli) * 100
		}
		if allocMemMi > 0 {
			memPct = float64(nu.reqMem) / float64(allocMemMi) * 100
		}

		podDensity := 0.0
		if podCap > 0 {
			podDensity = float64(nu.podCount) / float64(podCap) * 100
		}

		// Determine status
		status := "optimal"
		avgUsage := (cpuPct + memPct) / 2
		if avgUsage < 30 {
			status = "underutilized"
			result.Summary.UnderutilizedNodes++
		}
		if cpuPct > 85 || memPct > 85 {
			status = "overloaded"
			result.Summary.OverloadedNodes++
		}

		// Check fit: can any standard pod fit?
		availCPUMilli := allocCPUMilli - nu.reqCPU
		availMemMi := allocMemMi - nu.reqMem
		largestFitCPU := ""
		largestFitMem := ""
		fragile := false

		if availCPUMilli < 25 || availMemMi < 64 {
			// Can't fit even a nano pod
			if nu.podCount > 0 {
				status = "fragile"
				fragile = true
				result.Summary.FragileNodes++

				strandCPU := availCPUMilli
				strandMem := availMemMi
				if strandCPU < 0 {
					strandCPU = 0
				}
				if strandMem < 0 {
					strandMem = 0
				}
				totalStrandedCPU += strandCPU
				totalStrandedMem += strandMem

				result.FragileNodes = append(result.FragileNodes, FragileNode{
					Name:        node.Name,
					CPUUsagePct: cpuPct,
					MemUsagePct: memPct,
					Reason:      fmt.Sprintf("Cannot fit smallest pod (%dm CPU, %dMi mem free)", availCPUMilli, availMemMi),
					StrandedCPU: fmt.Sprintf("%dm", strandCPU),
					StrandedMem: fmt.Sprintf("%dMi", strandMem),
				})

				if strandCPU > 0 {
					result.StrandedResources = append(result.StrandedResources, StrandedResource{
						Node: node.Name, Type: "cpu", Amount: fmt.Sprintf("%dm", strandCPU),
						Reason: "fragmentation",
					})
				}
				if strandMem > 0 {
					result.StrandedResources = append(result.StrandedResources, StrandedResource{
						Node: node.Name, Type: "memory", Amount: fmt.Sprintf("%dMi", strandMem),
						Reason: "fragmentation",
					})
				}
			}
		}

		// Find largest standard pod that fits
		for _, sz := range stdPodSizes {
			if availCPUMilli >= sz.CPU && availMemMi >= sz.Mem {
				largestFitCPU = fmt.Sprintf("%dm", sz.CPU)
				largestFitMem = fmt.Sprintf("%dMi", sz.Mem)
			}
		}
		if largestFitCPU == "" {
			largestFitCPU = "none"
			largestFitMem = "none"
		}

		_ = fragile

		result.ByNode = append(result.ByNode, NodePackAnalysis{
			Name:           node.Name,
			AllocatableCPU: fmt.Sprintf("%dm", allocCPUMilli),
			AllocatableMem: fmt.Sprintf("%dMi", allocMemMi),
			RequestedCPU:   fmt.Sprintf("%dm", nu.reqCPU),
			RequestedMem:   fmt.Sprintf("%dMi", nu.reqMem),
			CPUUsagePct:    cpuPct,
			MemUsagePct:    memPct,
			PodCount:       nu.podCount,
			Capacity:       podCap,
			PodDensity:     podDensity,
			Status:         status,
			LargestFitCPU:  largestFitCPU,
			LargestFitMem:  largestFitMem,
		})

		totalCPUUsage += cpuPct
		totalMemUsage += memPct
	}

	// Averages
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCPUPacking = totalCPUUsage / float64(result.Summary.TotalNodes)
		result.Summary.AvgMemPacking = totalMemUsage / float64(result.Summary.TotalNodes)
	}

	// Stranded totals
	result.Summary.StrandedCPUMilli = totalStrandedCPU
	result.Summary.StrandedMemMi = totalStrandedMem

	// Fit assessment
	canFitSmall := false
	for _, n := range result.ByNode {
		if n.LargestFitCPU != "none" && (strings.Contains(n.LargestFitCPU, "100m") ||
			strings.Contains(n.LargestFitCPU, "250m") || strings.Contains(n.LargestFitCPU, "500m") ||
			strings.Contains(n.LargestFitCPU, "1000m")) {
			canFitSmall = true
			break
		}
	}
	if canFitSmall {
		result.Summary.FitAssessment = "standard pods can be scheduled"
	} else {
		result.Summary.FitAssessment = "critical: no node can fit a standard-sized pod"
	}

	// Bin pack score
	binScore := 50
	avgPack := (result.Summary.AvgCPUPacking + result.Summary.AvgMemPacking) / 2
	if avgPack > 60 {
		binScore += 30
	} else if avgPack > 40 {
		binScore += 15
	}
	if result.Summary.FragileNodes == 0 {
		binScore += 20
	}
	if result.Summary.UnderutilizedNodes > result.Summary.TotalNodes/3 {
		binScore -= 25
	}
	if binScore > 100 {
		binScore = 100
	}
	if binScore < 0 {
		binScore = 0
	}
	result.Summary.BinPackScore = binScore
	result.SchedulingScore = binScore
	result.Grade = goldenScoreToGrade(binScore)

	// Sort nodes by status priority (fragile > overloaded > underutilized > optimal)
	statusRank := map[string]int{"fragile": 4, "overloaded": 3, "underutilized": 2, "optimal": 1}
	sort.SliceStable(result.ByNode, func(i, j int) bool {
		return statusRank[result.ByNode[i].Status] > statusRank[result.ByNode[j].Status]
	})

	result.Recommendations = generateSchedIntelRecs(result)

	writeJSON(w, result)
}

// generateSchedIntelRecs produces actionable recommendations.
func generateSchedIntelRecs(result SchedulingIntelResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Bin-packing efficiency: %d/100 (grade %s) — avg CPU %.0f%%, avg mem %.0f%%",
		result.SchedulingScore, result.Grade, result.Summary.AvgCPUPacking, result.Summary.AvgMemPacking))

	if result.Summary.UnderutilizedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d underutilized nodes (<30%%) — consolidate workloads and scale down unused nodes",
			result.Summary.UnderutilizedNodes))
	}

	if result.Summary.FragileNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d fragile nodes with stranded resources — %dm CPU and %dMi memory wasted due to fragmentation",
			result.Summary.FragileNodes, result.Summary.StrandedCPUMilli, result.Summary.StrandedMemMi))
	}

	if result.Summary.OverloadedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d overloaded nodes (>85%%) — add capacity or rebalance pods with anti-affinity rules",
			result.Summary.OverloadedNodes))
	}

	if result.Summary.FitAssessment != "" && strings.Contains(result.Summary.FitAssessment, "critical") {
		recs = append(recs, "CRITICAL: no node can fit a standard-sized pod — cluster is at maximum density")
	}

	if result.Summary.BinPackScore > 70 {
		recs = append(recs, "Bin-packing is efficient — maintain current scheduling practices")
	}

	if len(recs) == 1 {
		recs = append(recs, "Scheduling and bin-packing are healthy")
	}

	return recs
}

// Suppress unused import
var _ resource.Quantity
