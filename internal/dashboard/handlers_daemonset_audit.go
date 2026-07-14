package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DSResult is the DaemonSet rollout & node coverage audit.
type DSResult struct {
	ScannedAt       time.Time   `json:"scannedAt"`
	Summary         DSSummary   `json:"summary"`
	ByDaemonSet     []DSEntry   `json:"byDaemonSet"`
	NodeGaps        []DSNodeGap `json:"nodeGaps"`
	Recommendations []string    `json:"recommendations"`
	HealthScore     int         `json:"healthScore"`
}

// DSSummary aggregates DaemonSet statistics.
type DSSummary struct {
	TotalDaemonSets  int `json:"totalDaemonSets"`
	TotalNodes       int `json:"totalNodes"`
	DesiredScheduled int `json:"desiredScheduled"` // total pods that should be scheduled
	CurrentScheduled int `json:"currentScheduled"` // total pods currently scheduled
	UpdatedScheduled int `json:"updatedScheduled"` // pods at latest revision
	Ready            int `json:"ready"`            // ready pods
	Available        int `json:"available"`        // available pods
	MissingNodes     int `json:"missingNodes"`     // nodes missing DS pods (total across all DS)
	OnDeleteStrategy int `json:"onDeleteStrategy"` // DS with OnDelete (manual update needed)
	RollingUpdate    int `json:"rollingUpdate"`    // DS with RollingUpdate
	StaleRevisions   int `json:"staleRevisions"`   // pods running old revision
	NoTolerations    int `json:"noTolerations"`    // DS without tolerations (may miss tainted nodes)
}

// DSEntry describes a single DaemonSet.
type DSEntry struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	Image            string `json:"image"`
	UpdateStrategy   string `json:"updateStrategy"`
	DesiredScheduled int    `json:"desiredScheduled"`
	CurrentScheduled int    `json:"currentScheduled"`
	UpdatedScheduled int    `json:"updatedScheduled"`
	Ready            int    `json:"ready"`
	Available        int    `json:"available"`
	MissingNodes     int    `json:"missingNodes"`
	StalePods        int    `json:"stalePods"`
	HasTolerations   bool   `json:"hasTolerations"`
	NodeSelector     string `json:"nodeSelector"`
	Status           string `json:"status"` // healthy, updating, degraded, critical
}

// DSNodeGap describes a node that's missing a DaemonSet pod.
type DSNodeGap struct {
	DaemonSet string `json:"daemonSet"`
	Namespace string `json:"namespace"`
	NodeName  string `json:"nodeName"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// dsAuditCore performs the audit on DaemonSets and nodes (testable).
func dsAuditCore(
	daemonSets []appsv1.DaemonSet,
	nodes []corev1.Node,
	pods []corev1.Pod,
) DSResult {
	result := DSResult{
		ScannedAt: time.Now(),
	}

	result.Summary.TotalNodes = len(nodes)

	// Build pod index by (namespace, daemonSet owner)
	// DaemonSet pods have ownerReference to ReplicaSet which is owned by DaemonSet
	// But pods also have labels: pod-template-hash, controlled-by: daemonset
	// We'll match by namespace + pod labels matching DS selector

	for i := range daemonSets {
		ds := &daemonSets[i]
		result.Summary.TotalDaemonSets++

		entry := DSEntry{
			Name:             ds.Name,
			Namespace:        ds.Namespace,
			DesiredScheduled: int(ds.Status.DesiredNumberScheduled),
			CurrentScheduled: int(ds.Status.CurrentNumberScheduled),
			UpdatedScheduled: int(ds.Status.UpdatedNumberScheduled),
			Ready:            int(ds.Status.NumberReady),
			Available:        int(ds.Status.NumberAvailable),
		}

		// Update strategy
		if ds.Spec.UpdateStrategy.Type == appsv1.OnDeleteDaemonSetStrategyType {
			entry.UpdateStrategy = "OnDelete"
			result.Summary.OnDeleteStrategy++
		} else {
			entry.UpdateStrategy = "RollingUpdate"
			result.Summary.RollingUpdate++
		}

		// Get image from first container
		if len(ds.Spec.Template.Spec.Containers) > 0 {
			entry.Image = ds.Spec.Template.Spec.Containers[0].Image
		}

		// Tolerations
		entry.HasTolerations = len(ds.Spec.Template.Spec.Tolerations) > 0
		if !entry.HasTolerations {
			result.Summary.NoTolerations++
		}

		// Node selector
		if len(ds.Spec.Template.Spec.NodeSelector) > 0 {
			parts := make([]string, 0, len(ds.Spec.Template.Spec.NodeSelector))
			for k, v := range ds.Spec.Template.Spec.NodeSelector {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
			sort.Strings(parts)
			entry.NodeSelector = strings.Join(parts, ",")
		}

		// Calculate aggregate stats
		result.Summary.DesiredScheduled += entry.DesiredScheduled
		result.Summary.CurrentScheduled += entry.CurrentScheduled
		result.Summary.UpdatedScheduled += entry.UpdatedScheduled
		result.Summary.Ready += entry.Ready
		result.Summary.Available += entry.Available

		// Calculate missing nodes
		missingCount := entry.DesiredScheduled - entry.CurrentScheduled
		if missingCount > 0 {
			entry.MissingNodes = missingCount
			result.Summary.MissingNodes += missingCount
		}

		// Calculate stale pods
		staleCount := entry.CurrentScheduled - entry.UpdatedScheduled
		if staleCount > 0 {
			entry.StalePods = staleCount
			result.Summary.StaleRevisions += staleCount
		}

		// Determine status
		entry.Status = "healthy"
		if entry.MissingNodes > 0 {
			entry.Status = "degraded"
		}
		if entry.Ready < entry.CurrentScheduled {
			entry.Status = "updating"
		}
		if entry.MissingNodes > 0 && entry.Ready == 0 {
			entry.Status = "critical"
		}

		// Find missing nodes by checking which nodes don't have a pod for this DS
		// Build set of nodes that have pods for this DS
		dsPodNodes := make(map[string]bool)
		for j := range pods {
			pod := &pods[j]
			if pod.Namespace != ds.Namespace {
				continue
			}
			// Check if this pod belongs to the DaemonSet
			for _, ref := range pod.OwnerReferences {
				if ref.Kind == "DaemonSet" && ref.Name == ds.Name {
					if pod.Spec.NodeName != "" {
						dsPodNodes[pod.Spec.NodeName] = true
					}
					break
				}
			}
		}

		// Check which nodes are missing pods
		for j := range nodes {
			node := &nodes[j]
			if isNodeSchedulable(node) && !dsPodNodes[node.Name] {
				// This node should have a DS pod but doesn't
				result.NodeGaps = append(result.NodeGaps, DSNodeGap{
					DaemonSet: ds.Name,
					Namespace: ds.Namespace,
					NodeName:  node.Name,
					Reason:    "schedulable node missing DaemonSet pod — check taints, tolerations, and node selector",
					Severity:  "medium",
				})
			}
		}

		result.ByDaemonSet = append(result.ByDaemonSet, entry)
	}

	// Sort by status (critical first)
	statusOrder := map[string]int{"critical": 0, "degraded": 1, "updating": 2, "healthy": 3}
	sort.Slice(result.ByDaemonSet, func(i, j int) bool {
		return statusOrder[result.ByDaemonSet[i].Status] < statusOrder[result.ByDaemonSet[j].Status]
	})

	sort.Slice(result.NodeGaps, func(i, j int) bool {
		return result.NodeGaps[i].Severity > result.NodeGaps[j].Severity
	})

	result.HealthScore = dsScore(result.Summary)
	result.Recommendations = dsRecommendations(result.Summary)

	return result
}

// isNodeSchedulable checks if a node is schedulable.
func isNodeSchedulable(node *corev1.Node) bool {
	if node.Spec.Unschedulable {
		return false
	}
	for _, taint := range node.Spec.Taints {
		if taint.Key == "node.kubernetes.io/unschedulable" {
			return false
		}
	}
	return true
}

// dsScore calculates the health score.
func dsScore(s DSSummary) int {
	if s.TotalDaemonSets == 0 {
		return 100
	}
	base := 100
	// Missing nodes is most critical
	base -= s.MissingNodes * 5
	// Stale revisions indicate incomplete rollout
	base -= s.StaleRevisions * 2
	// OnDelete strategy means updates require manual intervention
	base -= s.OnDeleteStrategy * 3
	// No tolerations means DS may miss tainted nodes
	base -= s.NoTolerations * 1
	if base < 0 {
		base = 0
	}
	return base
}

// dsRecommendations generates actionable recommendations.
func dsRecommendations(s DSSummary) []string {
	var recs []string
	if s.MissingNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) missing DaemonSet pods — check taints, tolerations, and node selectors", s.MissingNodes))
	}
	if s.OnDeleteStrategy > 0 {
		recs = append(recs, fmt.Sprintf("%d DaemonSet(s) use OnDelete strategy — switch to RollingUpdate for automatic updates", s.OnDeleteStrategy))
	}
	if s.StaleRevisions > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) running old DaemonSet revision — rollout may be stuck or incomplete", s.StaleRevisions))
	}
	if s.NoTolerations > 0 {
		recs = append(recs, fmt.Sprintf("%d DaemonSet(s) have no tolerations — pods won't schedule on tainted nodes (control plane, GPU, etc.)", s.NoTolerations))
	}
	if s.MissingNodes == 0 && s.StaleRevisions == 0 && s.OnDeleteStrategy == 0 {
		recs = append(recs, fmt.Sprintf("all %d DaemonSet(s) are healthy — pods cover all %d nodes at latest revision", s.TotalDaemonSets, s.TotalNodes))
	}
	return recs
}

// handleDaemonSetAudit audits DaemonSet rollout status and node coverage.
// GET /api/deployment/daemonset-audit
func (s *Server) handleDaemonSetAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	daemonSets, err := rc.clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		pods = &corev1.PodList{}
	}

	result := dsAuditCore(daemonSets.Items, nodes.Items, pods.Items)
	writeJSON(w, result)
}
