package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResvResult is the node resource reservation & allocatable gap analysis.
type ResvResult struct {
	Timestamp       time.Time       `json:"timestamp"`
	Score           int             `json:"score"`
	Status          string          `json:"status"`
	Summary         ResvSummary     `json:"summary"`
	Nodes           []ResvNodeEntry `json:"nodes"`
	ByNodeType      []ResvTypeStat  `json:"byNodeType"`
	Issues          []ResvIssue     `json:"issues"`
	Recommendations []string        `json:"recommendations"`
}

// ResvSummary holds aggregate reservation metrics.
type ResvSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	TotalCapacityCPU float64 `json:"totalCapacityCPU"`
	TotalCapacityMem float64 `json:"totalCapacityMemGB"`
	TotalAllocCPU    float64 `json:"totalAllocatableCPU"`
	TotalAllocMem    float64 `json:"totalAllocatableMemGB"`
	AvgResvPctCPU    float64 `json:"avgReservationPctCPU"`
	AvgResvPctMem    float64 `json:"avgReservationPctMem"`
	OverResvNodes    int     `json:"overReservedNodes"`
	UnderResvNodes   int     `json:"underReservedNodes"`
}

// ResvNodeEntry shows reservation details per node.
type ResvNodeEntry struct {
	Node          string  `json:"node"`
	InstanceType  string  `json:"instanceType"`
	CapacityCPU   float64 `json:"capacityCPU"`
	AllocCPU      float64 `json:"allocatableCPU"`
	ResvCPU       float64 `json:"reservedCPU"`
	ResvPctCPU    float64 `json:"reservationPctCPU"`
	CapacityMemGB float64 `json:"capacityMemGB"`
	AllocMemGB    float64 `json:"allocatableMemGB"`
	ResvMemGB     float64 `json:"reservedMemGB"`
	ResvPctMem    float64 `json:"reservationPctMem"`
	PodCapacity   int64   `json:"podCapacity"`
	AllocPods     int64   `json:"allocPods"`
	Status        string  `json:"status"`
}

// ResvTypeStat groups by node instance type.
type ResvTypeStat struct {
	InstanceType string  `json:"instanceType"`
	NodeCount    int     `json:"nodeCount"`
	AvgResvCPU   float64 `json:"avgResvPctCPU"`
	AvgResvMem   float64 `json:"avgResvPctMem"`
}

// ResvIssue identifies a reservation problem.
type ResvIssue struct {
	Type     string  `json:"type"`
	Severity string  `json:"severity"`
	Node     string  `json:"node"`
	Value    float64 `json:"value"`
	Message  string  `json:"message"`
}

func (s *Server) handleResvAudit(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	result := analyzeReservations(nodes.Items)
	writeJSON(w, result)
}

func analyzeReservations(nodes []corev1.Node) ResvResult {
	now := time.Now()
	summary := ResvSummary{TotalNodes: len(nodes)}
	var entries []ResvNodeEntry
	var issues []ResvIssue
	typeStats := make(map[string]*ResvTypeStat)

	for _, node := range nodes {
		isReady := false
		for _, c := range node.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				isReady = true
			}
		}
		if !isReady {
			continue
		}

		capCPU := float64(node.Status.Capacity.Cpu().MilliValue()) / 1000
		allocCPU := float64(node.Status.Allocatable.Cpu().MilliValue()) / 1000
		resvCPU := capCPU - allocCPU
		resvPctCPU := 0.0
		if capCPU > 0 {
			resvPctCPU = resvCPU / capCPU * 100
		}

		capMem := float64(node.Status.Capacity.Memory().Value()) / (1024 * 1024 * 1024)
		allocMem := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		resvMem := capMem - allocMem
		resvPctMem := 0.0
		if capMem > 0 {
			resvPctMem = resvMem / capMem * 100
		}

		podCap := node.Status.Capacity.Pods().Value()
		podAlloc := node.Status.Allocatable.Pods().Value()

		instanceType := node.Labels["node.kubernetes.io/instance-type"]
		if instanceType == "" {
			instanceType = "unknown"
		}

		status := "normal"
		// Typical reservation: 5-15% for kube/system
		if resvPctCPU > 25 || resvPctMem > 25 {
			status = "over-reserved"
			summary.OverResvNodes++
			severity := "medium"
			if resvPctCPU > 40 || resvPctMem > 40 {
				severity = "high"
			}
			issues = append(issues, ResvIssue{
				Type:     "OverReserved",
				Severity: severity,
				Node:     node.Name,
				Value:    resvPctCPU,
				Message:  fmt.Sprintf("Node %s has %.1f%% CPU / %.1f%% memory reserved; check kube-reserved/system-reserved/eviction-threshold settings", node.Name, resvPctCPU, resvPctMem),
			})
		} else if resvPctCPU < 3 && resvPctMem < 3 {
			status = "under-reserved"
			summary.UnderResvNodes++
			issues = append(issues, ResvIssue{
				Type:     "UnderReserved",
				Severity: "medium",
				Node:     node.Name,
				Value:    resvPctCPU,
				Message:  fmt.Sprintf("Node %s has very low reservation (%.1f%% CPU); system workloads may compete with pods under pressure", node.Name, resvPctCPU),
			})
		}

		entry := ResvNodeEntry{
			Node:          node.Name,
			InstanceType:  instanceType,
			CapacityCPU:   capCPU,
			AllocCPU:      allocCPU,
			ResvCPU:       resvCPU,
			ResvPctCPU:    resvPctCPU,
			CapacityMemGB: capMem,
			AllocMemGB:    allocMem,
			ResvMemGB:     resvMem,
			ResvPctMem:    resvPctMem,
			PodCapacity:   podCap,
			AllocPods:     podAlloc,
			Status:        status,
		}
		entries = append(entries, entry)

		// Aggregate
		summary.TotalCapacityCPU += capCPU
		summary.TotalCapacityMem += capMem
		summary.TotalAllocCPU += allocCPU
		summary.TotalAllocMem += allocMem

		// Type stats
		ts, ok := typeStats[instanceType]
		if !ok {
			ts = &ResvTypeStat{InstanceType: instanceType}
			typeStats[instanceType] = ts
		}
		ts.NodeCount++
		ts.AvgResvCPU += resvPctCPU
		ts.AvgResvMem += resvPctMem
	}

	if summary.TotalNodes > 0 {
		summary.AvgResvPctCPU = (summary.TotalCapacityCPU - summary.TotalAllocCPU) / summary.TotalCapacityCPU * 100
		summary.AvgResvPctMem = (summary.TotalCapacityMem - summary.TotalAllocMem) / summary.TotalCapacityMem * 100
	}

	// Build type stats
	var byType []ResvTypeStat
	for _, ts := range typeStats {
		if ts.NodeCount > 0 {
			ts.AvgResvCPU /= float64(ts.NodeCount)
			ts.AvgResvMem /= float64(ts.NodeCount)
		}
		byType = append(byType, *ts)
	}
	sort.Slice(byType, func(i, j int) bool { return byType[i].NodeCount > byType[j].NodeCount })

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ResvPctCPU > entries[j].ResvPctCPU
	})

	// Score
	score := 100
	score -= summary.OverResvNodes * 5
	score -= summary.UnderResvNodes * 3
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if summary.OverResvNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) over-reserved (>25%%); tune kube-reserved/system-reserved to reclaim schedulable capacity", summary.OverResvNodes))
	}
	if summary.UnderResvNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) under-reserved (<3%%); add kube-reserved to protect system daemons under pressure", summary.UnderResvNodes))
	}
	if summary.AvgResvPctCPU > 20 {
		recs = append(recs, fmt.Sprintf("Average CPU reservation is %.1f%%; consider right-sizing reserved resources", summary.AvgResvPctCPU))
	}
	if len(recs) == 0 {
		recs = append(recs, "Node resource reservations look well-configured")
	}

	return ResvResult{
		Timestamp:       now,
		Score:           score,
		Status:          status,
		Summary:         summary,
		Nodes:           entries,
		ByNodeType:      byType,
		Issues:          issues,
		Recommendations: recs,
	}
}
