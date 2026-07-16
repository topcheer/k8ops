package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DrainImpactResult simulates the effect of draining a specific node.
// It identifies which pods would be evicted, whether they can be
// rescheduled on remaining nodes, and what the service disruption
// impact would be. This helps operators plan safe maintenance windows.
type DrainImpactResult struct {
	ScannedAt        time.Time          `json:"scannedAt"`
	TargetNode       string             `json:"targetNode"`
	NodeInfo         DrainNodeInfo      `json:"nodeInfo"`
	ImpactSummary    DrainImpactSummary `json:"impactSummary"`
	AffectedPods     []DrainPodImpact   `json:"affectedPods"`
	RescheduleFeas   DrainReschedule    `json:"rescheduleFeasibility"`
	ServiceImpact    []DrainServiceBlip `json:"serviceImpact"`
	SafeToDrain      bool               `json:"safeToDrain"`
	RiskLevel        string             `json:"riskLevel"`
	Recommendations  []string           `json:"recommendations"`
}

type DrainNodeInfo struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	CPUCapacity float64 `json:"cpuCapacity"`
	MemCapacity float64 `json:"memCapacityGB"`
	PodCapacity int    `json:"podCapacity"`
	Taints      []string `json:"taints"`
	Zone       string `json:"zone"`
}

type DrainImpactSummary struct {
	TotalPods        int `json:"totalPodsOnNode"`
	EvictablePods    int `json:"evictablePods"`
	DaemonSetPods    int `json:"daemonSetPods"` // not evicted
	StaticPods      int `json:"staticPods"`
	WithPDB          int `json:"withPDB"`
	ProtectedPods    int `json:"protectedPods"` // PDB blocks eviction
	Reschedulable    int `json:"reschedulable"`
	NotReschedulable int `json:"notReschedulable"`
	CriticalWorkloads int `json:"criticalWorkloads"`
}

type DrainPodImpact struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Kind           string `json:"kind"`
	OwnerName      string `json:"ownerName"`
	Replicas       int    `json:"replicas"`
	HasPDB         bool   `json:"hasPDB"`
	PDBMinAvail    int    `json:"pdbMinAvailable"`
	CanEvict       bool   `json:"canEvict"`
	CanReschedule  bool   `json:"canReschedule"`
	RescheduleReason string `json:"rescheduleReason"`
	DisruptionRisk string `json:"disruptionRisk"`
}

type DrainReschedule struct {
	RemainingNodes    int     `json:"remainingNodes"`
	AvailableCPU      float64 `json:"availableCPU"`
	AvailableMem      float64 `json:"availableMemoryGB"`
	AvailablePodSlots int     `json:"availablePodSlots"`
	NeededCPU         float64 `json:"neededCPU"`
	NeededMem         float64 `json:"neededMemoryGB"`
	NeededPodSlots    int     `json:"neededPodSlots"`
	FitsCPU           bool    `json:"fitsCPU"`
	FitsMem           bool    `json:"fitsMemory"`
	FitsPods          bool    `json:"fitsPods"`
	OverallFit        bool    `json:"overallFit"`
}

type DrainServiceBlip struct {
	Service     string `json:"service"`
	Namespace   string `json:"namespace"`
	ImpactType  string `json:"impactType"` // degraded, unavailable, transient
	Duration    string `json:"estimatedDuration"`
	Description string `json:"description"`
}

// handleDrainImpact handles GET /api/operations/drain-impact?node=<name>
func (s *Server) handleDrainImpact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	targetNode := r.URL.Query().Get("node")
	if targetNode == "" {
		writeError(w, http.StatusBadRequest, "missing 'node' query parameter")
		return
	}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	result := DrainImpactResult{
		ScannedAt:  time.Now(),
		TargetNode: targetNode,
	}

	// Find target node info
	var targetNodeObj *corev1.Node
	for i := range nodes.Items {
		if nodes.Items[i].Name == targetNode {
			targetNodeObj = &nodes.Items[i]
			break
		}
	}
	if targetNodeObj == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("node %q not found", targetNode))
		return
	}

	// Node info
	result.NodeInfo = extractNodeInfo(targetNodeObj)

	// Build PDB coverage map
	pdbNSMap := make(map[string]bool)
	for _, pdb := range pdbs.Items {
		if pdb.Spec.Selector != nil && len(pdb.Spec.Selector.MatchLabels) > 0 {
			pdbNSMap[pdb.Namespace] = true
		}
	}

	// Calculate remaining cluster capacity
	otherNodesCapacity := DrainReschedule{}
	for _, n := range nodes.Items {
		if n.Name == targetNode {
			continue
		}
		otherNodesCapacity.RemainingNodes++
		otherNodesCapacity.AvailableCPU += n.Status.Allocatable.Cpu().AsApproximateFloat64()
		otherNodesCapacity.AvailableMem += n.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
		if podAlloc, ok := n.Status.Allocatable[corev1.ResourcePods]; ok {
			otherNodesCapacity.AvailablePodSlots += int(podAlloc.Value())
		}
	}

	// Subtract existing pod usage on remaining nodes
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Spec.NodeName == targetNode {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		otherNodesCapacity.AvailablePodSlots--
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				otherNodesCapacity.AvailableCPU -= req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				otherNodesCapacity.AvailableMem -= req.AsApproximateFloat64() / 1e9
			}
		}
	}

	// Analyze pods on target node
	var allImpacts []DrainPodImpact
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != targetNode {
			continue
		}

		dpi := DrainPodImpact{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		}

		// Determine owner kind
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				dpi.Kind = ref.Kind
				dpi.OwnerName = ref.Name
				break
			}
		}

		// DaemonSet pods are not evicted
		if dpi.Kind == "DaemonSet" {
			result.ImpactSummary.DaemonSetPods++
			dpi.CanEvict = false
			dpi.CanReschedule = false
			dpi.RescheduleReason = "DaemonSet pod stays on node"
			continue
		}

		// Mirror pods (static pods)
		if dpi.Kind == "" && pod.Annotations["kubernetes.io/config.mirror"] != "" {
			result.ImpactSummary.StaticPods++
			continue
		}

		result.ImpactSummary.TotalPods++
		result.ImpactSummary.EvictablePods++

		// Check PDB
		if pdbNSMap[pod.Namespace] {
			dpi.HasPDB = true
			result.ImpactSummary.WithPDB++
		}

		// Calculate resource needs for rescheduling
		var podCPU, podMem float64
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				podCPU += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				podMem += req.AsApproximateFloat64() / 1e9
			}
		}

		otherNodesCapacity.NeededCPU += podCPU
		otherNodesCapacity.NeededMem += podMem
		otherNodesCapacity.NeededPodSlots++

		// Reschedule feasibility check
		dpi.CanReschedule = true
		dpi.CanEvict = true
		if dpi.HasPDB {
			dpi.DisruptionRisk = "PDB may block eviction"
			result.ImpactSummary.ProtectedPods++
		}

		// Check node affinity/selectors that might prevent rescheduling
		if pod.Spec.NodeSelector != nil && len(pod.Spec.NodeSelector) > 0 {
			hasMatching := false
			for _, n := range nodes.Items {
				if n.Name == targetNode {
					continue
				}
				matches := true
				for k, v := range pod.Spec.NodeSelector {
					if n.Labels[k] != v {
						matches = false
						break
					}
				}
				if matches {
					hasMatching = true
					break
				}
			}
			if !hasMatching {
				dpi.CanReschedule = false
				dpi.RescheduleReason = "NodeSelector does not match any remaining node"
			}
		}

		// Check tolerations vs taints on other nodes
		if dpi.CanReschedule {
			for _, n := range nodes.Items {
				if n.Name == targetNode {
					continue
				}
				for _, taint := range n.Spec.Taints {
					tolerated := false
					for _, tol := range pod.Spec.Tolerations {
						if tol.Key == taint.Key && (tol.Effect == "" || string(tol.Effect) == string(taint.Effect)) {
							tolerated = true
							break
						}
					}
					if !tolerated && taint.Effect == corev1.TaintEffectNoSchedule {
						// Check if this is the only remaining node
						if otherNodesCapacity.RemainingNodes <= 1 {
							dpi.CanReschedule = false
							dpi.RescheduleReason = fmt.Sprintf("Taint %s on remaining node not tolerated", taint.Key)
						}
					}
				}
			}
		}

		if dpi.CanReschedule {
			result.ImpactSummary.Reschedulable++
		} else {
			result.ImpactSummary.NotReschedulable++
		}

		allImpacts = append(allImpacts, dpi)
	}

	// Check capacity fit
	otherNodesCapacity.FitsCPU = otherNodesCapacity.AvailableCPU >= otherNodesCapacity.NeededCPU
	otherNodesCapacity.FitsMem = otherNodesCapacity.AvailableMem >= otherNodesCapacity.NeededMem
	otherNodesCapacity.FitsPods = otherNodesCapacity.AvailablePodSlots >= otherNodesCapacity.NeededPodSlots
	otherNodesCapacity.OverallFit = otherNodesCapacity.FitsCPU && otherNodesCapacity.FitsMem && otherNodesCapacity.FitsPods
	result.RescheduleFeas = otherNodesCapacity

	// Build service impact list
	for _, dpi := range allImpacts {
		if !dpi.CanReschedule || dpi.HasPDB {
			result.ServiceImpact = append(result.ServiceImpact, DrainServiceBlip{
				Service:     dpi.OwnerName,
				Namespace:   dpi.Namespace,
				ImpactType:  impactTypeForPod(dpi),
				Duration:    "transient",
				Description: impactDescForPod(dpi),
			})
		}
	}

	// Determine safety
	result.ImpactSummary.CriticalWorkloads = result.ImpactSummary.NotReschedulable + result.ImpactSummary.ProtectedPods
	sort.Slice(allImpacts, func(i, j int) bool {
		// Can't reschedule first
		if allImpacts[i].CanReschedule != allImpacts[j].CanReschedule {
			return !allImpacts[i].CanReschedule
		}
		return allImpacts[i].HasPDB && !allImpacts[j].HasPDB
	})
	result.AffectedPods = allImpacts

	// Safety assessment
	if result.RescheduleFeas.RemainingNodes == 0 {
		result.SafeToDrain = false
		result.RiskLevel = "critical"
	} else if !result.RescheduleFeas.OverallFit {
		result.SafeToDrain = false
		result.RiskLevel = "high"
	} else if result.ImpactSummary.CriticalWorkloads > 0 {
		result.SafeToDrain = result.ImpactSummary.CriticalWorkloads <= 2
		if result.ImpactSummary.CriticalWorkloads > 5 {
			result.RiskLevel = "high"
		} else {
			result.RiskLevel = "medium"
		}
	} else {
		result.SafeToDrain = true
		result.RiskLevel = "low"
	}

	result.Recommendations = buildDrainRecs(&result)

	writeJSON(w, result)
}

func extractNodeInfo(n *corev1.Node) DrainNodeInfo {
	info := DrainNodeInfo{
		Name:        n.Name,
		CPUCapacity: n.Status.Allocatable.Cpu().AsApproximateFloat64(),
		MemCapacity: n.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9,
	}
	if podAlloc, ok := n.Status.Allocatable[corev1.ResourcePods]; ok {
		info.PodCapacity = int(podAlloc.Value())
	}
	for _, t := range n.Spec.Taints {
		info.Taints = append(info.Taints, fmt.Sprintf("%s=%s:%s", t.Key, t.Value, t.Effect))
	}
	info.Zone = n.Labels["topology.kubernetes.io/zone"]
	if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
		info.Role = "control-plane"
	} else if _, ok := n.Labels["node-role.kubernetes.io/master"]; ok {
		info.Role = "master"
	} else {
		info.Role = "worker"
	}
	return info
}

func impactTypeForPod(dpi DrainPodImpact) string {
	if !dpi.CanReschedule {
		return "unavailable"
	}
	if dpi.HasPDB {
		return "degraded"
	}
	return "transient"
}

func impactDescForPod(dpi DrainPodImpact) string {
	if !dpi.CanReschedule {
		if dpi.RescheduleReason != "" {
			return dpi.RescheduleReason
		}
		return "Pod cannot be rescheduled on remaining nodes"
	}
	if dpi.HasPDB {
		return "PDB may delay or block eviction"
	}
	return "Brief disruption during rescheduling"
}

func buildDrainRecs(r *DrainImpactResult) []string {
	recs := []string{}
	if r.RescheduleFeas.RemainingNodes == 0 {
		recs = append(recs, "CRITICAL: This is the only node — draining will cause total service outage")
		return recs
	}
	if !r.RescheduleFeas.FitsCPU {
		recs = append(recs, fmt.Sprintf("CPU 不足: 剩余 %.1f 核，需要 %.1f 核", r.RescheduleFeas.AvailableCPU, r.RescheduleFeas.NeededCPU))
	}
	if !r.RescheduleFeas.FitsMem {
		recs = append(recs, fmt.Sprintf("内存不足: 剩余 %.1f GB，需要 %.1f GB", r.RescheduleFeas.AvailableMem, r.RescheduleFeas.NeededMem))
	}
	if !r.RescheduleFeas.FitsPods {
		recs = append(recs, fmt.Sprintf("Pod 槽位不足: 剩余 %d，需要 %d", r.RescheduleFeas.AvailablePodSlots, r.RescheduleFeas.NeededPodSlots))
	}
	if r.ImpactSummary.ProtectedPods > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 受 PDB 保护，驱逐可能被拒绝", r.ImpactSummary.ProtectedPods))
	}
	if r.ImpactSummary.NotReschedulable > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 Pod 无法重新调度，将导致服务中断", r.ImpactSummary.NotReschedulable))
	}
	if r.SafeToDrain {
		recs = append(recs, "节点可以安全排空，建议在低峰期执行")
	} else {
		recs = append(recs, "建议先扩容节点或迁移关键工作负载后再排空")
	}
	return recs
}
