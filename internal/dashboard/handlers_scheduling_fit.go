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

// SchedulingFitResult is the pod resource request density & scheduling fit audit.
type SchedulingFitResult struct {
	ScannedAt        time.Time          `json:"scannedAt"`
	Summary          SchedFitSummary    `json:"summary"`
	ByNode           []SchedFitNodeStat `json:"byNode"`
	ByNamespace      []SchedFitNSStat   `json:"byNamespace"`
	OverProvisioned  []SchedFitPodEntry `json:"overProvisioned"`
	UnderProvisioned []SchedFitPodEntry `json:"underProvisioned"`
	Risks            []SchedFitRisk     `json:"risks"`
	Recommendations  []string           `json:"recommendations"`
	HealthScore      int                `json:"healthScore"`
}

// SchedFitSummary aggregates scheduling fit metrics.
type SchedFitSummary struct {
	TotalPods        int     `json:"totalPods"`
	TotalNodes       int     `json:"totalNodes"`
	AvgNodePacking   float64 `json:"avgNodePacking"`   // avg % of node capacity used by requests
	OverProvisioned  int     `json:"overProvisioned"`  // requests > 2x typical usage (heuristic: requests > 4 CPU or 8Gi)
	UnderProvisioned int     `json:"underProvisioned"` // requests < 100m CPU or 128Mi
	NoRequest        int     `json:"noRequest"`        // containers with no resource requests
	BestFitNodes     int     `json:"bestFitNodes"`     // packing 60-85%
	OverpackedNodes  int     `json:"overpackedNodes"`  // packing >85%
	UnderpackedNodes int     `json:"underpackedNodes"` // packing <30%
}

// SchedFitNodeStat per-node scheduling fit.
type SchedFitNodeStat struct {
	NodeName       string  `json:"nodeName"`
	AllocatableCPU string  `json:"allocatableCPU"`
	AllocatableMem string  `json:"allocatableMem"`
	RequestedCPU   string  `json:"requestedCPU"`
	RequestedMem   string  `json:"requestedMem"`
	PackingPct     float64 `json:"packingPct"` // max of CPU/mem packing
	CPUPacking     float64 `json:"cpuPacking"`
	MemPacking     float64 `json:"memPacking"`
	PodCount       int     `json:"podCount"`
	FitCategory    string  `json:"fitCategory"` // underpacked, optimal, overpacked
}

// SchedFitNSStat per-namespace scheduling fit.
type SchedFitNSStat struct {
	Namespace        string `json:"namespace"`
	PodCount         int    `json:"podCount"`
	OverProvisioned  int    `json:"overProvisioned"`
	UnderProvisioned int    `json:"underProvisioned"`
	NoRequest        int    `json:"noRequest"`
}

// SchedFitPodEntry describes a pod with provisioning issues.
type SchedFitPodEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Node       string `json:"node,omitempty"`
	CPURequest string `json:"cpuRequest,omitempty"`
	MemRequest string `json:"memRequest,omitempty"`
	Issue      string `json:"issue"`
}

// SchedFitRisk describes a scheduling fit risk.
type SchedFitRisk struct {
	NodeName string `json:"nodeName,omitempty"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// handleSchedulingFit audits pod resource request density & scheduling fit.
// GET /api/scalability/scheduling-fit
func (s *Server) handleSchedulingFit(w http.ResponseWriter, r *http.Request) {
	result := SchedulingFitResult{
		ScannedAt: time.Now(),
	}

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
	result.Summary.TotalNodes = len(nodes.Items)

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// Build node map with allocatable resources
	nodeMap := make(map[string]*SchedFitNodeStat)
	for _, node := range nodes.Items {
		allocCPU := node.Status.Allocatable[corev1.ResourceCPU]
		allocMem := node.Status.Allocatable[corev1.ResourceMemory]
		entry := &SchedFitNodeStat{
			NodeName:       node.Name,
			AllocatableCPU: allocCPU.String(),
			AllocatableMem: allocMem.String(),
			FitCategory:    "underpacked",
		}
		nodeMap[node.Name] = entry
	}

	nsStats := map[string]*SchedFitNSStat{}

	// Aggregate pod requests per node
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Spec.NodeName == "" {
			continue
		}
		result.Summary.TotalPods++
		nodeEntry := nodeMap[pod.Spec.NodeName]
		if nodeEntry == nil {
			continue
		}
		nodeEntry.PodCount++

		ns := pod.Namespace
		if nsStats[ns] == nil {
			nsStats[ns] = &SchedFitNSStat{Namespace: ns}
		}

		var podCPU, podMem resource.Quantity
		hasRequest := false
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests != nil {
				if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
					podCPU.Add(cpu)
					hasRequest = true
				}
				if mem, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
					podMem.Add(mem)
					hasRequest = true
				}
			}
		}

		if !hasRequest {
			result.Summary.NoRequest++
			nsStats[ns].NoRequest++
			result.UnderProvisioned = append(result.UnderProvisioned, SchedFitPodEntry{
				Name: pod.Name, Namespace: ns, Node: pod.Spec.NodeName,
				Issue: "No resource requests specified — may cause scheduling failures",
			})
		} else {
			// Check over/under provisioned (heuristic)
			cpuMillicores := podCPU.MilliValue()
			memBytes := podMem.Value()

			if cpuMillicores > 4000 || memBytes > 8*1024*1024*1024 {
				result.Summary.OverProvisioned++
				nsStats[ns].OverProvisioned++
				result.OverProvisioned = append(result.OverProvisioned, SchedFitPodEntry{
					Name: pod.Name, Namespace: ns, Node: pod.Spec.NodeName,
					CPURequest: podCPU.String(), MemRequest: podMem.String(),
					Issue: fmt.Sprintf("High resource request (CPU: %s, Mem: %s) — verify actual usage to avoid waste", podCPU.String(), podMem.String()),
				})
			} else if cpuMillicores > 0 && cpuMillicores < 100 {
				result.Summary.UnderProvisioned++
				nsStats[ns].UnderProvisioned++
				result.UnderProvisioned = append(result.UnderProvisioned, SchedFitPodEntry{
					Name: pod.Name, Namespace: ns, Node: pod.Spec.NodeName,
					CPURequest: podCPU.String(),
					Issue:      fmt.Sprintf("Very low CPU request (%s) — may cause throttling under load", podCPU.String()),
				})
			}
		}

		// Add to node's requested total
		nodeEntryCpu := nodeEntry
		_ = nodeEntryCpu
		// Accumulate using a local map since we can't modify Quantity in map easily
		// We'll compute from raw milli values
		if nodeEntry.RequestedCPU == "" {
			nodeEntry.RequestedCPU = podCPU.String()
		} else {
			existing := resource.MustParse(nodeEntry.RequestedCPU)
			existing.Add(podCPU)
			nodeEntry.RequestedCPU = existing.String()
		}
		if nodeEntry.RequestedMem == "" {
			nodeEntry.RequestedMem = podMem.String()
		} else {
			existing := resource.MustParse(nodeEntry.RequestedMem)
			existing.Add(podMem)
			nodeEntry.RequestedMem = existing.String()
		}
	}

	// Calculate packing percentages
	totalPacking := 0.0
	for _, entry := range nodeMap {
		allocCPU := nodeMap[entry.NodeName].AllocatableCPU
		allocMem := nodeMap[entry.NodeName].AllocatableMem
		reqCPU := entry.RequestedCPU
		reqMem := entry.RequestedMem

		if allocCPU != "" && reqCPU != "" {
			allocQ := resource.MustParse(allocCPU)
			reqQ := resource.MustParse(reqCPU)
			if allocQ.MilliValue() > 0 {
				entry.CPUPacking = float64(reqQ.MilliValue()) / float64(allocQ.MilliValue()) * 100
			}
		}
		if allocMem != "" && reqMem != "" {
			allocQ := resource.MustParse(allocMem)
			reqQ := resource.MustParse(reqMem)
			if allocQ.Value() > 0 {
				entry.MemPacking = float64(reqQ.Value()) / float64(allocQ.Value()) * 100
			}
		}
		entry.PackingPct = entry.CPUPacking
		if entry.MemPacking > entry.PackingPct {
			entry.PackingPct = entry.MemPacking
		}

		// Categorize
		if entry.PackingPct > 85 {
			entry.FitCategory = "overpacked"
			result.Summary.OverpackedNodes++
			result.Risks = append(result.Risks, SchedFitRisk{
				NodeName: entry.NodeName,
				Issue:    fmt.Sprintf("Node %s is overpacked (%.1f%%) — risk of scheduling failures", entry.NodeName, entry.PackingPct),
				Severity: "high",
			})
		} else if entry.PackingPct >= 30 {
			entry.FitCategory = "optimal"
			result.Summary.BestFitNodes++
		} else {
			entry.FitCategory = "underpacked"
			result.Summary.UnderpackedNodes++
		}
		totalPacking += entry.PackingPct

		result.ByNode = append(result.ByNode, *entry)
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgNodePacking = totalPacking / float64(result.Summary.TotalNodes)
	}

	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].PackingPct > result.ByNode[j].PackingPct
	})

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].OverProvisioned+result.ByNamespace[i].UnderProvisioned >
			result.ByNamespace[j].OverProvisioned+result.ByNamespace[j].UnderProvisioned
	})

	// Health score
	score := 100
	if result.Summary.OverpackedNodes > 0 {
		score -= min(20, result.Summary.OverpackedNodes*5)
	}
	if result.Summary.NoRequest > 0 {
		score -= min(20, result.Summary.NoRequest*3)
	}
	if result.Summary.OverProvisioned > 0 {
		score -= min(15, result.Summary.OverProvisioned*3)
	}
	if result.Summary.UnderProvisioned > 0 {
		score -= min(10, result.Summary.UnderProvisioned*2)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.OverpackedNodes > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d node(s) are overpacked (>85%%) — add nodes or reduce resource requests", result.Summary.OverpackedNodes))
	}
	if result.Summary.NoRequest > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pod(s) have no resource requests — always specify requests for reliable scheduling", result.Summary.NoRequest))
	}
	if result.Summary.OverProvisioned > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pod(s) are likely over-provisioned — right-size requests based on actual usage", result.Summary.OverProvisioned))
	}
	if result.Summary.UnderpackedNodes > 0 && result.Summary.TotalNodes > 2 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d node(s) are underpacked (<30%%) — consider consolidating workloads and scaling down", result.Summary.UnderpackedNodes))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Resource request density is well-balanced — nodes are optimally packed")
	}

	writeJSON(w, result)
}
