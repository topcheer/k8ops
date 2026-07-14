package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeDrainReadinessResult is the node drain & rotation readiness audit.
type NodeDrainReadinessResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         DrainReadinessSummary `json:"summary"`
	ByNode          []NodeDrainEntry      `json:"byNode"`
	Blockers        []DrainBlocker        `json:"blockers"`
	RotationRisks   []RotationRisk        `json:"rotationRisks"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// DrainReadinessSummary aggregates drain readiness metrics.
type DrainReadinessSummary struct {
	TotalNodes         int `json:"totalNodes"`
	SafeToDrain        int `json:"safeToDrain"`      // all pods can be evicted
	RiskyToDrain       int `json:"riskyToDrain"`     // has stateful pods or PDB blockers
	DangerousToDrain   int `json:"dangerousToDrain"` // single replica on this node
	Cordoned           int `json:"cordoned"`         // already unschedulable
	TotalPods          int `json:"totalPods"`
	PodsWithPDB        int `json:"podsWithPDB"`        // pods protected by PDB
	PodsWithoutPDB     int `json:"podsWithoutPDB"`     // pods without PDB protection
	StatefulPodsOnNode int `json:"statefulPodsOnNode"` // StatefulSet pods (sticky)
	DaemonSetPods      int `json:"daemonSetPods"`      // DaemonSet pods (won't move)
	BarePods           int `json:"barePods"`           // pods without a controller
	StandalonePods     int `json:"standalonePods"`     // pods without owner reference
	NodesWithLocalData int `json:"nodesWithLocalData"` // nodes hosting pods with local storage
}

// NodeDrainEntry per-node drain readiness assessment.
type NodeDrainEntry struct {
	NodeName        string `json:"nodeName"`
	Status          string `json:"status"` // safe, risky, dangerous, cordoned
	Drainable       bool   `json:"drainable"`
	PodCount        int    `json:"podCount"`
	MovablePods     int    `json:"movablePods"`     // pods that can reschedule
	StickyPods      int    `json:"stickyPods"`      // pods that won't move (DaemonSet, local storage)
	StatefulPods    int    `json:"statefulPods"`    // StatefulSet pods
	PodsWithPDB     int    `json:"podsWithPDB"`     // pods protected by PDB
	BarePods        int    `json:"barePods"`        // pods without controller
	HasLocalStorage bool   `json:"hasLocalStorage"` // any pod with local storage
	Cordoned        bool   `json:"cordoned"`
	Ready           bool   `json:"ready"`
	RiskLevel       string `json:"riskLevel"` // low, medium, high, critical
	BlockReason     string `json:"blockReason,omitempty"`
}

// DrainBlocker describes something that blocks draining a node.
type DrainBlocker struct {
	NodeName string `json:"nodeName,omitempty"`
	PodName  string `json:"podName,omitempty"`
	Kind     string `json:"kind"` // PDB, StatefulSet, LocalStorage, BarePod, NodePressure
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

// RotationRisk describes a node rotation risk.
type RotationRisk struct {
	NodeName string `json:"nodeName,omitempty"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
	Impact   string `json:"impact"`
}

// handleNodeDrainReadiness audits node drain & rotation readiness.
// GET /api/scalability/node-drain-readiness
func (s *Server) handleNodeDrainReadiness(w http.ResponseWriter, r *http.Request) {
	result := NodeDrainReadinessResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Get nodes and pods
	nodes, err := rc.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list nodes: %v", err))
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	// 2. Get PDBs to identify protected pods
	pdbs, pdbErr := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	pdbProtectedNS := map[string]map[string]bool{} // namespace → set of matched pod labels
	if pdbErr == nil {
		for _, pdb := range pdbs.Items {
			if pdb.Spec.Selector == nil {
				continue
			}
			// We'll match later by checking if pod labels match PDB selector
			_ = pdbProtectedNS
		}
	}

	// 3. Build per-node drain assessment
	nodeMap := make(map[string]*NodeDrainEntry)
	for _, node := range nodes.Items {
		entry := &NodeDrainEntry{
			NodeName:  node.Name,
			Status:    "safe",
			Drainable: true,
			Ready:     isNodeReadyDrain(&node),
			Cordoned:  node.Spec.Unschedulable,
		}
		if node.Spec.Unschedulable {
			entry.Status = "cordoned"
			entry.Drainable = false
			result.Summary.Cordoned++
		}
		nodeMap[node.Name] = entry
	}

	// 4. Analyze pods per node
	for i := range pods.Items {
		pod := &pods.Items[i]
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			continue // pending pod, not on any node
		}
		entry := nodeMap[nodeName]
		if entry == nil {
			continue
		}

		result.Summary.TotalPods++
		entry.PodCount++

		// Classify pod type
		isDaemonSet := false
		isStatefulSet := false
		isBarePod := true
		hasLocalStorage := false

		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "DaemonSet" {
				isDaemonSet = true
				isBarePod = false
			}
			if ref.Kind == "StatefulSet" {
				isStatefulSet = true
				isBarePod = false
			}
			if ref.Kind == "ReplicaSet" || ref.Kind == "Deployment" || ref.Kind == "Job" || ref.Kind == "CronJob" {
				isBarePod = false
			}
		}

		// Check for local storage (emptyDir, hostPath)
		for _, vol := range pod.Spec.Volumes {
			if vol.EmptyDir != nil || vol.HostPath != nil {
				hasLocalStorage = true
			}
		}

		if isDaemonSet {
			entry.StickyPods++
			result.Summary.DaemonSetPods++
			// DaemonSet pods don't move, but they'll be replaced on new node
		} else if isStatefulSet {
			entry.StatefulPods++
			result.Summary.StatefulPodsOnNode++
			entry.StickyPods++
			// StatefulSet pods are sticky (PVC binding)
			entry.Status = upgradeStatus(entry.Status, "risky")
			entry.Drainable = false
			result.Blockers = append(result.Blockers, DrainBlocker{
				NodeName: nodeName,
				PodName:  pod.Name,
				Kind:     "StatefulSet",
				Detail:   fmt.Sprintf("StatefulSet pod %s/%s has PVC binding — will not reschedule", pod.Namespace, pod.Name),
				Severity: "warning",
			})
		} else if isBarePod {
			entry.BarePods++
			result.Summary.BarePods++
			result.Summary.StandalonePods++
			entry.Status = upgradeStatus(entry.Status, "dangerous")
			entry.Drainable = false
			result.Blockers = append(result.Blockers, DrainBlocker{
				NodeName: nodeName,
				PodName:  pod.Name,
				Kind:     "BarePod",
				Detail:   fmt.Sprintf("Standalone pod %s/%s without controller — will be lost on drain", pod.Namespace, pod.Name),
				Severity: "critical",
			})
		} else {
			entry.MovablePods++
		}

		if hasLocalStorage {
			entry.HasLocalStorage = true
			result.Summary.NodesWithLocalData++
			result.Blockers = append(result.Blockers, DrainBlocker{
				NodeName: nodeName,
				PodName:  pod.Name,
				Kind:     "LocalStorage",
				Detail:   fmt.Sprintf("Pod %s/%s uses local storage — data will be lost on drain", pod.Namespace, pod.Name),
				Severity: "high",
			})
		}

		// Check PDB protection
		if pdbErr == nil && pdbs != nil {
			for _, pdb := range pdbs.Items {
				if pdb.Spec.Selector == nil {
					continue
				}
				// Simple label match
				if matchLabels(pod.Labels, pdb.Spec.Selector.MatchLabels) {
					entry.PodsWithPDB++
					result.Summary.PodsWithPDB++
					// Check if PDB blocks eviction
					if pdb.Spec.MinAvailable != nil {
						// PDB with minAvailable may block drain
						entry.Status = upgradeStatus(entry.Status, "risky")
						if entry.Drainable {
							entry.Drainable = false
						}
					}
					break
				}
			}
		}

		if entry.PodsWithPDB == 0 && !isDaemonSet {
			result.Summary.PodsWithoutPDB++
		}
	}

	// 5. Finalize node statuses and build result
	for _, entry := range nodeMap {
		// Determine risk level
		if entry.BarePods > 0 {
			entry.RiskLevel = "critical"
			result.Summary.DangerousToDrain++
		} else if entry.StatefulPods > 0 || entry.HasLocalStorage {
			entry.RiskLevel = "high"
			result.Summary.RiskyToDrain++
		} else if entry.PodsWithPDB > 0 {
			entry.RiskLevel = "medium"
			result.Summary.RiskyToDrain++
		} else if !entry.Ready {
			entry.RiskLevel = "high"
		} else {
			entry.RiskLevel = "low"
			if entry.Status != "cordoned" {
				result.Summary.SafeToDrain++
			}
		}

		// Set block reason
		if entry.BarePods > 0 {
			entry.BlockReason = fmt.Sprintf("%d standalone pod(s) without controller", entry.BarePods)
		} else if entry.StatefulPods > 0 {
			entry.BlockReason = fmt.Sprintf("%d StatefulSet pod(s) with PVC binding", entry.StatefulPods)
		} else if entry.HasLocalStorage {
			entry.BlockReason = "pod(s) with local storage"
		} else if entry.PodsWithPDB > 0 {
			entry.BlockReason = fmt.Sprintf("%d pod(s) protected by PDB", entry.PodsWithPDB)
		}

		result.ByNode = append(result.ByNode, *entry)
	}

	// Sort by risk level (critical first)
	sort.Slice(result.ByNode, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return riskOrder[result.ByNode[i].RiskLevel] < riskOrder[result.ByNode[j].RiskLevel]
	})

	// 6. Identify rotation risks
	for _, node := range nodes.Items {
		entry := nodeMap[node.Name]
		if entry == nil {
			continue
		}
		// Check for single-replica workloads on this node
		singleReplicaCount := 0
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Spec.NodeName != node.Name {
				continue
			}
			for _, ref := range pod.OwnerReferences {
				if ref.Kind == "ReplicaSet" || ref.Kind == "Deployment" {
					// Check if this is the only replica
					sameRSCount := 0
					for j := range pods.Items {
						p := &pods.Items[j]
						if p.Namespace == pod.Namespace && len(p.OwnerReferences) > 0 {
							for _, r := range p.OwnerReferences {
								if r.Kind == ref.Kind && r.Name == ref.Name && p.Spec.NodeName != "" {
									sameRSCount++
								}
							}
						}
					}
					if sameRSCount == 1 {
						singleReplicaCount++
					}
				}
			}
		}
		if singleReplicaCount > 0 {
			result.RotationRisks = append(result.RotationRisks, RotationRisk{
				NodeName: node.Name,
				Issue:    fmt.Sprintf("%d single-replica workload(s) on this node — draining will cause downtime", singleReplicaCount),
				Severity: "high",
				Impact:   "Service downtime during node rotation",
			})
		}
	}

	// 7. Calculate health score
	score := 100
	if result.Summary.DangerousToDrain > 0 {
		score -= 30
	}
	if result.Summary.RiskyToDrain > 0 {
		score -= 15
	}
	if result.Summary.PodsWithoutPDB > 0 {
		score -= min(20, result.Summary.PodsWithoutPDB)
	}
	if result.Summary.BarePods > 0 {
		score -= min(20, result.Summary.BarePods*5)
	}
	if result.Summary.NodesWithLocalData > 0 {
		score -= min(10, result.Summary.NodesWithLocalData*3)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 8. Recommendations
	if result.Summary.DangerousToDrain > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d node(s) are dangerous to drain — standalone pods without controllers will be lost", result.Summary.DangerousToDrain))
	}
	if result.Summary.RiskyToDrain > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d node(s) are risky to drain — StatefulSet pods or PDB-protected pods may block eviction", result.Summary.RiskyToDrain))
	}
	if result.Summary.PodsWithoutPDB > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pod(s) have no PDB — add PodDisruptionBudget for safe node rotation", result.Summary.PodsWithoutPDB))
	}
	if result.Summary.NodesWithLocalData > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d node(s) host pods with local storage — data will be lost during rotation", result.Summary.NodesWithLocalData))
	}
	if result.Summary.SafeToDrain == result.Summary.TotalNodes-result.Summary.Cordoned && result.Summary.TotalNodes > 0 {
		result.Recommendations = append(result.Recommendations,
			"All nodes are safe to drain — cluster is ready for rotation")
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Node drain readiness is healthy — all workloads can be safely evicted")
	}

	writeJSON(w, result)
}

// isNodeReadyDrain checks if a node is in Ready state.
func isNodeReadyDrain(node *corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// upgradeStatus upgrades node status to higher risk level.
func upgradeStatus(current, newStatus string) string {
	order := map[string]int{"safe": 0, "risky": 1, "dangerous": 2, "cordoned": 3}
	if order[newStatus] > order[current] {
		return newStatus
	}
	return current
}

// matchLabels checks if pod labels match a PDB selector.
func matchLabels(podLabels map[string]string, selector map[string]string) bool {
	for k, v := range selector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}
