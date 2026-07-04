package dashboard

import (
	"fmt"
	"net/http"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OvercommitResult is the full resource over-commit analysis.
type OvercommitResult struct {
	Nodes           []OvercommitNode   `json:"nodes"`
	Summary         OvercommitSummary  `json:"summary"`
	NoLimitPods     []NoLimitEntry     `json:"noLimitPods"`
	ByNamespace     []OvercommitNsStat `json:"byNamespace"`
	Recommendations []string           `json:"recommendations"`
}

// OvercommitSummary aggregates cluster-wide over-commit metrics.
type OvercommitSummary struct {
	TotalNodes          int     `json:"totalNodes"`
	NodesAtRisk         int     `json:"nodesAtRisk"`    // CPU or mem overcommit > 2x
	NodesCritical       int     `json:"nodesCritical"`  // overcommit > 3x
	TotalCPUCommit      float64 `json:"totalCPUCommit"` // sum(requests) / capacity
	TotalMemCommit      float64 `json:"totalMemCommit"`
	TotalCPULimitCommit float64 `json:"totalCPULimitCommit"` // sum(limits) / capacity
	TotalMemLimitCommit float64 `json:"totalMemLimitCommit"`
	PodsWithoutLimits   int     `json:"podsWithoutLimits"`
	PodsWithoutRequests int     `json:"podsWithoutRequests"`
	ClusterScore        int     `json:"clusterScore"` // 0-100 (100 = safe)
}

// OvercommitNode describes over-commit status for one node.
type OvercommitNode struct {
	Name              string  `json:"name"`
	Ready             bool    `json:"ready"`
	PodCount          int     `json:"podCount"`
	CPUCapacity       int64   `json:"cpuCapacityM"` // millicores
	CPURequests       int64   `json:"cpuRequestsM"`
	CPULimits         int64   `json:"cpuLimitsM"`
	CPUCommitPct      float64 `json:"cpuCommitPct"`      // requests / capacity
	CPULimitCommitPct float64 `json:"cpuLimitCommitPct"` // limits / capacity
	MemCapacityGB     float64 `json:"memCapacityGB"`
	MemRequestsGB     float64 `json:"memRequestsGB"`
	MemLimitsGB       float64 `json:"memLimitsGB"`
	MemCommitPct      float64 `json:"memCommitPct"`
	MemLimitCommitPct float64 `json:"memLimitCommitPct"`
	RiskLevel         string  `json:"riskLevel"`     // safe / moderate / high / critical
	PressureScore     int     `json:"pressureScore"` // 0-100
}

// NoLimitEntry describes a pod without resource limits.
type NoLimitEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Node      string `json:"node"`
	Kind      string `json:"kind"`
	Issue     string `json:"issue"` // no-limits, no-requests, no-cpu-limit, no-mem-limit
}

// OvercommitNsStat aggregates per-namespace over-commit.
type OvercommitNsStat struct {
	Namespace   string  `json:"namespace"`
	PodCount    int     `json:"podCount"`
	CPURequestM int64   `json:"cpuRequestM"`
	CPULimitM   int64   `json:"cpuLimitM"`
	MemReqGB    float64 `json:"memRequestGB"`
	MemLimitGB  float64 `json:"memLimitGB"`
	NoLimitPods int     `json:"noLimitPods"`
}

// handleOvercommitAnalysis analyzes resource over-commit across nodes.
// GET /api/scalability/overcommit?namespace=xxx
func (s *Server) handleOvercommitAnalysis(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build per-node resource aggregation
	nodeRes := make(map[string]*nodeResourceAgg)
	for _, node := range nodes.Items {
		nodeRes[node.Name] = &nodeResourceAgg{name: node.Name}
	}

	nsMap := make(map[string]*OvercommitNsStat)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if pod.Spec.NodeName == "" {
			continue
		}

		nodeData, ok := nodeRes[pod.Spec.NodeName]
		if !ok {
			continue
		}
		nodeData.podCount++

		// Namespace stats
		nsStat := getOrCreateOvercommitNs(nsMap, pod.Namespace)
		nsStat.PodCount++

		podHasCPULimit := false
		podHasMemLimit := false
		podHasCPUReq := false
		podHasMemReq := false

		for _, c := range pod.Spec.Containers {
			// Requests
			if cpuReq := c.Resources.Requests.Cpu(); cpuReq != nil && !cpuReq.IsZero() {
				nodeData.cpuReqM += cpuReq.MilliValue()
				nsStat.CPURequestM += cpuReq.MilliValue()
				podHasCPUReq = true
			}
			if memReq := c.Resources.Requests.Memory(); memReq != nil && !memReq.IsZero() {
				gb := float64(memReq.Value()) / (1024 * 1024 * 1024)
				nodeData.memReqGB += gb
				nsStat.MemReqGB += gb
				podHasMemReq = true
			}

			// Limits
			if cpuLim := c.Resources.Limits.Cpu(); cpuLim != nil && !cpuLim.IsZero() {
				nodeData.cpuLimM += cpuLim.MilliValue()
				nsStat.CPULimitM += cpuLim.MilliValue()
				podHasCPULimit = true
			}
			if memLim := c.Resources.Limits.Memory(); memLim != nil && !memLim.IsZero() {
				gb := float64(memLim.Value()) / (1024 * 1024 * 1024)
				nodeData.memLimGB += gb
				nsStat.MemLimitGB += gb
				podHasMemLimit = true
			}
		}

		// Check for missing limits/requests
		kind := podKind(&pod)
		if !podHasCPULimit && !podHasMemLimit {
			result := NoLimitEntry{
				Name: pod.Name, Namespace: pod.Namespace, Node: pod.Spec.NodeName,
				Kind: kind, Issue: "no-limits",
			}
			// We'll collect these after building node data
			nodeData.noLimitPods = append(nodeData.noLimitPods, result)
			nsStat.NoLimitPods++
		} else if !podHasCPULimit {
			nodeData.noLimitPods = append(nodeData.noLimitPods, NoLimitEntry{
				Name: pod.Name, Namespace: pod.Namespace, Node: pod.Spec.NodeName,
				Kind: kind, Issue: "no-cpu-limit",
			})
		} else if !podHasMemLimit {
			nodeData.noLimitPods = append(nodeData.noLimitPods, NoLimitEntry{
				Name: pod.Name, Namespace: pod.Namespace, Node: pod.Spec.NodeName,
				Kind: kind, Issue: "no-mem-limit",
			})
		}

		if !podHasCPUReq && !podHasMemReq {
			nodeData.noLimitPods = append(nodeData.noLimitPods, NoLimitEntry{
				Name: pod.Name, Namespace: pod.Namespace, Node: pod.Spec.NodeName,
				Kind: kind, Issue: "no-requests",
			})
		}
	}

	result := OvercommitResult{}

	// Build node entries
	totalCPUReq, totalCPULim := int64(0), int64(0)
	totalMemReq, totalMemLim := 0.0, 0.0
	totalCPUCap, totalMemCap := int64(0), 0.0
	nodesAtRisk, nodesCritical := 0, 0
	totalNoLimits, totalNoReqs := 0, 0

	for _, node := range nodes.Items {
		data := nodeRes[node.Name]
		cpuCap := node.Status.Allocatable.Cpu().MilliValue()
		memCapGB := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)

		entry := OvercommitNode{
			Name:          node.Name,
			Ready:         isNodeReady(&node),
			PodCount:      data.podCount,
			CPUCapacity:   cpuCap,
			CPURequests:   data.cpuReqM,
			CPULimits:     data.cpuLimM,
			MemCapacityGB: memCapGB,
			MemRequestsGB: data.memReqGB,
			MemLimitsGB:   data.memLimGB,
		}

		if cpuCap > 0 {
			entry.CPUCommitPct = float64(data.cpuReqM) / float64(cpuCap) * 100
			entry.CPULimitCommitPct = float64(data.cpuLimM) / float64(cpuCap) * 100
		}
		if memCapGB > 0 {
			entry.MemCommitPct = data.memReqGB / memCapGB * 100
			entry.MemLimitCommitPct = data.memLimGB / memCapGB * 100
		}

		// Risk level based on limit over-commit
		maxCommit := entry.CPULimitCommitPct
		if entry.MemLimitCommitPct > maxCommit {
			maxCommit = entry.MemLimitCommitPct
		}
		switch {
		case maxCommit > 300:
			entry.RiskLevel = "critical"
			nodesCritical++
			nodesAtRisk++
		case maxCommit > 200:
			entry.RiskLevel = "high"
			nodesAtRisk++
		case maxCommit > 150:
			entry.RiskLevel = "moderate"
		default:
			entry.RiskLevel = "safe"
		}

		entry.PressureScore = calculatePressureScore(entry)

		result.Nodes = append(result.Nodes, entry)

		// Collect no-limit pods
		for _, nlp := range data.noLimitPods {
			if nlp.Issue == "no-limits" || nlp.Issue == "no-cpu-limit" || nlp.Issue == "no-mem-limit" {
				totalNoLimits++
			}
			if nlp.Issue == "no-requests" {
				totalNoReqs++
			}
			result.NoLimitPods = append(result.NoLimitPods, nlp)
		}

		totalCPUReq += data.cpuReqM
		totalCPULim += data.cpuLimM
		totalMemReq += data.memReqGB
		totalMemLim += data.memLimGB
		totalCPUCap += cpuCap
		totalMemCap += memCapGB
	}

	// Sort nodes by pressure score
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].PressureScore > result.Nodes[j].PressureScore
	})

	// Sort no-limit pods
	sort.Slice(result.NoLimitPods, func(i, j int) bool {
		return result.NoLimitPods[i].Namespace < result.NoLimitPods[j].Namespace
	})
	if len(result.NoLimitPods) > 50 {
		result.NoLimitPods = result.NoLimitPods[:50]
	}

	// Build summary
	result.Summary = OvercommitSummary{
		TotalNodes:          len(nodes.Items),
		NodesAtRisk:         nodesAtRisk,
		NodesCritical:       nodesCritical,
		PodsWithoutLimits:   totalNoLimits,
		PodsWithoutRequests: totalNoReqs,
	}
	if totalCPUCap > 0 {
		result.Summary.TotalCPUCommit = float64(totalCPUReq) / float64(totalCPUCap)
		result.Summary.TotalCPULimitCommit = float64(totalCPULim) / float64(totalCPUCap)
	}
	if totalMemCap > 0 {
		result.Summary.TotalMemCommit = totalMemReq / totalMemCap
		result.Summary.TotalMemLimitCommit = totalMemLim / totalMemCap
	}
	result.Summary.ClusterScore = calculateOvercommitScore(result.Summary)

	// Build namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		if result.ByNamespace[i].NoLimitPods != result.ByNamespace[j].NoLimitPods {
			return result.ByNamespace[i].NoLimitPods > result.ByNamespace[j].NoLimitPods
		}
		return result.ByNamespace[i].CPURequestM > result.ByNamespace[j].CPURequestM
	})

	// Recommendations
	result.Recommendations = generateOvercommitRecs(result)

	writeJSON(w, result)
}

// nodeResourceAgg accumulates per-node resource usage.
type nodeResourceAgg struct {
	name        string
	podCount    int
	cpuReqM     int64
	cpuLimM     int64
	memReqGB    float64
	memLimGB    float64
	noLimitPods []NoLimitEntry
}

// calculatePressureScore computes 0-100 pressure score for a node.
func calculatePressureScore(n OvercommitNode) int {
	score := 0

	// Request commit contribution
	if n.CPUCommitPct > 80 {
		score += 20
	} else if n.CPUCommitPct > 60 {
		score += 10
	}
	if n.MemCommitPct > 80 {
		score += 20
	} else if n.MemCommitPct > 60 {
		score += 10
	}

	// Limit over-commit contribution
	if n.CPULimitCommitPct > 300 {
		score += 30
	} else if n.CPULimitCommitPct > 200 {
		score += 15
	}
	if n.MemLimitCommitPct > 300 {
		score += 30
	} else if n.MemLimitCommitPct > 200 {
		score += 15
	}

	if score > 100 {
		score = 100
	}
	return score
}

// calculateOvercommitScore computes cluster-wide safety score.
func calculateOvercommitScore(s OvercommitSummary) int {
	score := 100
	score -= s.NodesCritical * 15
	score -= (s.NodesAtRisk - s.NodesCritical) * 5
	if s.TotalCPULimitCommit > 3 {
		score -= 10
	} else if s.TotalCPULimitCommit > 2 {
		score -= 5
	}
	if s.TotalMemLimitCommit > 3 {
		score -= 10
	} else if s.TotalMemLimitCommit > 2 {
		score -= 5
	}
	score -= s.PodsWithoutLimits * 1
	if score < 0 {
		score = 0
	}
	return score
}

// generateOvercommitRecs produces actionable recommendations.
func generateOvercommitRecs(result OvercommitResult) []string {
	var recs []string
	s := result.Summary

	if s.NodesCritical > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) have critical over-commit (>3x limits vs capacity) — OOMKill and CPU throttle risk is severe", s.NodesCritical))
	}
	if s.NodesAtRisk > s.NodesCritical {
		recs = append(recs, fmt.Sprintf("%d node(s) have high over-commit (2-3x) — workload interference likely under load", s.NodesAtRisk-s.NodesCritical))
	}
	if s.PodsWithoutLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have no resource limits — can consume unbounded CPU/memory and starve neighbors", s.PodsWithoutLimits))
	}
	if s.PodsWithoutRequests > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) have no resource requests — may not be scheduled properly and have no QoS guarantee", s.PodsWithoutRequests))
	}
	if s.TotalCPULimitCommit > 2 {
		recs = append(recs, fmt.Sprintf("Cluster CPU limit over-commit is %.1fx — CPU throttling will occur under contention", s.TotalCPULimitCommit))
	}
	if s.TotalMemLimitCommit > 2 {
		recs = append(recs, fmt.Sprintf("Cluster memory limit over-commit is %.1fx — OOMKill risk under memory pressure", s.TotalMemLimitCommit))
	}
	if s.ClusterScore < 50 {
		recs = append(recs, fmt.Sprintf("Over-commit safety score is %d/100 — add nodes or reduce limits to improve stability", s.ClusterScore))
	}

	return recs
}

func getOrCreateOvercommitNs(m map[string]*OvercommitNsStat, ns string) *OvercommitNsStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &OvercommitNsStat{Namespace: ns}
	m[ns] = e
	return e
}

// Ensure resource is used.
var _ = resource.Quantity{}
