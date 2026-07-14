package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SpotReadinessResult is the spot/preemptible instance readiness audit.
type SpotReadinessResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         SpotReadinessSummary `json:"summary"`
	SpotNodes       []SpotNodeEntry      `json:"spotNodes"`
	OnDemandNodes   []SpotNodeEntry      `json:"onDemandNodes"`
	AtRiskWorkloads []SpotWorkloadEntry  `json:"atRiskWorkloads"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// SpotReadinessSummary aggregates spot instance statistics.
type SpotReadinessSummary struct {
	TotalNodes        int     `json:"totalNodes"`
	SpotNodes         int     `json:"spotNodes"`
	OnDemandNodes     int     `json:"onDemandNodes"`
	SpotPercentage    float64 `json:"spotPercentage"`
	SpotCapacity      int     `json:"spotCapacity"`      // total CPU cores on spot
	OnDemandCapacity  int     `json:"onDemandCapacity"`  // total CPU cores on on-demand
	EstimatedSavings  float64 `json:"estimatedSavings"`  // estimated monthly savings in USD
	WorkloadsOnSpot   int     `json:"workloadsOnSpot"`   // pods running on spot nodes
	CriticalOnSpot    int     `json:"criticalOnSpot"`    // critical pods on spot without protection
	HasSpotToleration int     `json:"hasSpotToleration"` // pods with spot toleration
	HasSpotAntiAffin  int     `json:"hasSpotAntiAffin"`  // pods with spot anti-affinity
	SpotInterruption  bool    `json:"spotInterruption"`  // interruption handler detected
}

// SpotNodeEntry describes a node with spot/on-demand classification.
type SpotNodeEntry struct {
	Name         string `json:"name"`
	InstanceType string `json:"instanceType"`
	CapacityType string `json:"capacityType"` // spot, on-demand, unknown
	Zone         string `json:"zone"`
	CPUCores     int    `json:"cpuCores"`
	PodCount     int    `json:"podCount"`
	Age          string `json:"age"`
}

// SpotWorkloadEntry describes a workload at risk on spot nodes.
type SpotWorkloadEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	NodeName  string `json:"nodeName"`
	Severity  string `json:"severity"`
	Reason    string `json:"reason"`
}

// spotReadinessAuditCore performs the audit on nodes and pods (testable).
func spotReadinessAuditCore(
	nodes []corev1.Node,
	pods []corev1.Pod,
	now time.Time,
) SpotReadinessResult {
	result := SpotReadinessResult{
		ScannedAt: now,
	}

	// Detect spot interruption handler
	for i := range pods {
		pod := &pods[i]
		name := pod.Name
		if strings.Contains(name, "node-termination-handler") ||
			strings.Contains(name, "spot-instance-interrupter") ||
			strings.Contains(name, "karpenter") {
			result.Summary.SpotInterruption = true
			break
		}
	}

	// Classify nodes
	nodeTypeMap := make(map[string]string) // node name -> capacity type

	for i := range nodes {
		node := &nodes[i]
		result.Summary.TotalNodes++

		capacityType := classifyNodeCapacityType(node)
		nodeTypeMap[node.Name] = capacityType

		// Get CPU cores
		cpuCores := 0
		if cpu := node.Status.Allocatable.Cpu(); cpu != nil {
			cpuCores = int(cpu.MilliValue() / 1000)
		}

		// Get zone
		zone := node.Labels[corev1.LabelTopologyZone]
		if zone == "" {
			zone = node.Labels["topology.kubernetes.io/zone"]
		}

		// Get instance type
		instanceType := node.Labels[corev1.LabelInstanceType]
		if instanceType == "" {
			instanceType = node.Labels["beta.kubernetes.io/instance-type"]
		}

		// Age
		age := now.Sub(node.CreationTimestamp.Time)

		entry := SpotNodeEntry{
			Name:         node.Name,
			InstanceType: instanceType,
			CapacityType: capacityType,
			Zone:         zone,
			CPUCores:     cpuCores,
			Age:          formatDuration(age),
		}

		// Count pods on this node
		for j := range pods {
			if pods[j].Spec.NodeName == node.Name {
				entry.PodCount++
			}
		}

		if capacityType == "spot" {
			result.Summary.SpotNodes++
			result.Summary.SpotCapacity += cpuCores
			result.SpotNodes = append(result.SpotNodes, entry)
		} else {
			result.Summary.OnDemandNodes++
			result.Summary.OnDemandCapacity += cpuCores
			result.OnDemandNodes = append(result.OnDemandNodes, entry)
		}
	}

	// Calculate spot percentage
	if result.Summary.TotalNodes > 0 {
		result.Summary.SpotPercentage = float64(result.Summary.SpotNodes) / float64(result.Summary.TotalNodes) * 100
	}

	// Estimate savings: spot is ~70% cheaper than on-demand
	if result.Summary.SpotCapacity > 0 {
		// Rough estimate: $0.10/core/hour on-demand, $0.03/core/hour spot
		hourlySavings := float64(result.Summary.SpotCapacity) * 0.07
		result.Summary.EstimatedSavings = hourlySavings * 24 * 30 // monthly
	}

	// Analyze workloads on spot nodes
	for i := range pods {
		pod := &pods[i]
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			continue
		}

		nodeType, ok := nodeTypeMap[nodeName]
		if !ok || nodeType != "spot" {
			continue
		}

		result.Summary.WorkloadsOnSpot++

		// Check if pod has spot toleration
		hasSpotToleration := false
		for _, tol := range pod.Spec.Tolerations {
			if strings.Contains(tol.Key, "spot") || strings.Contains(tol.Key, "preemptible") ||
				strings.Contains(tol.Key, "karpenter") {
				hasSpotToleration = true
				break
			}
		}
		if hasSpotToleration {
			result.Summary.HasSpotToleration++
		}

		// Check for anti-affinity (spread across node types)
		hasAntiAffinity := false
		if pod.Spec.Affinity != nil && pod.Spec.Affinity.PodAntiAffinity != nil {
			hasAntiAffinity = true
		}
		if hasAntiAffinity {
			result.Summary.HasSpotAntiAffin++
		}

		// Determine criticality: no spot toleration + on spot = at risk
		if !hasSpotToleration {
			// Check if it's a critical workload (Deployment/StatefulSet with replicas > 0)
			ownerKind := "Pod"
			for _, ref := range pod.OwnerReferences {
				if ref.Kind == "Deployment" || ref.Kind == "StatefulSet" || ref.Kind == "ReplicaSet" {
					ownerKind = ref.Kind
					break
				}
			}

			severity := "medium"
			reason := "running on spot node without spot toleration — may be evicted without graceful handling"
			if ownerKind == "StatefulSet" {
				severity = "high"
				reason = "StatefulSet on spot node without spot toleration — data loss risk on eviction"
			}

			result.Summary.CriticalOnSpot++
			result.AtRiskWorkloads = append(result.AtRiskWorkloads, SpotWorkloadEntry{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Kind:      ownerKind,
				NodeName:  nodeName,
				Severity:  severity,
				Reason:    reason,
			})
		}
	}

	// Sort
	sort.Slice(result.SpotNodes, func(i, j int) bool {
		return result.SpotNodes[i].CPUCores > result.SpotNodes[j].CPUCores
	})
	sort.Slice(result.AtRiskWorkloads, func(i, j int) bool {
		return result.AtRiskWorkloads[i].Severity > result.AtRiskWorkloads[j].Severity
	})

	result.HealthScore = spotReadinessScore(result.Summary)
	result.Recommendations = spotReadinessRecommendations(result.Summary)

	return result
}

// classifyNodeCapacityType determines if a node is spot/preemptible or on-demand.
func classifyNodeCapacityType(node *corev1.Node) string {
	labels := node.Labels

	// Karpenter capacity type
	if ct, ok := labels["karpenter.sh/capacity-type"]; ok {
		if ct == "spot" || ct == "preemptible" {
			return "spot"
		}
		return "on-demand"
	}

	// AWS spot instance
	if it, ok := labels[corev1.LabelInstanceType]; ok {
		if strings.HasPrefix(it, "spot") || strings.Contains(it, "-spot") {
			return "spot"
		}
	}

	// GCE preemptible
	if _, ok := labels["cloud.google.com/gke-preemptible"]; ok {
		return "spot"
	}

	// Azure spot
	if _, ok := labels["kubernetes.azure.com/scalesetpriority"]; ok {
		if labels["kubernetes.azure.com/scalesetpriority"] == "spot" {
			return "spot"
		}
	}

	// Generic spot label
	if v, ok := labels["node-role.kubernetes.io/spot"]; ok || v == "" {
		if _, ok2 := labels["node-role.kubernetes.io/spot"]; ok2 {
			return "spot"
		}
	}

	// Check for spot-related annotations in labels
	for k, v := range labels {
		if strings.Contains(k, "spot") || strings.Contains(v, "spot") ||
			strings.Contains(k, "preemptible") || strings.Contains(v, "preemptible") {
			return "spot"
		}
	}

	return "on-demand"
}

// spotReadinessScore calculates the health score.
func spotReadinessScore(s SpotReadinessSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}
	base := 100

	// Having spot instances is good (cost savings), but unprotected workloads are bad
	if s.SpotNodes > 0 {
		// Reward for using spot
		if s.SpotPercentage > 0 {
			base += 5 // bonus for cost optimization
		}
		// Penalize for critical workloads without protection
		base -= s.CriticalOnSpot * 10
		// Penalize for missing interruption handler
		if !s.SpotInterruption {
			base -= 15
		}
	} else {
		// No spot instances — no penalty, but no bonus either
		base -= 5 // small penalty for not optimizing costs
	}

	if base > 100 {
		base = 100
	}
	if base < 0 {
		base = 0
	}
	return base
}

// spotReadinessRecommendations generates actionable recommendations.
func spotReadinessRecommendations(s SpotReadinessSummary) []string {
	var recs []string
	if s.SpotNodes == 0 {
		recs = append(recs, "no spot/preemptible nodes detected — consider using spot instances for non-critical workloads to reduce costs by up to 70%")
		return recs
	}

	recs = append(recs, fmt.Sprintf("%.1f%% of nodes are spot/preemptible (%d/%d) — estimated monthly savings: $%.2f",
		s.SpotPercentage, s.SpotNodes, s.TotalNodes, s.EstimatedSavings))

	if !s.SpotInterruption {
		recs = append(recs, "no spot interruption handler detected — install AWS Node Termination Handler or use Karpenter for graceful spot eviction handling")
	}

	if s.CriticalOnSpot > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) running on spot nodes without spot toleration — add spot tolerations and node anti-affinity to prevent unexpected evictions", s.CriticalOnSpot))
	}

	if s.WorkloadsOnSpot > 0 && s.HasSpotToleration == 0 {
		recs = append(recs, "no workloads have spot tolerations — add tolerations for spot/preemptible taints to enable proper scheduling")
	}

	if s.SpotNodes > 0 && s.HasSpotAntiAffin == 0 {
		recs = append(recs, "no workloads have spot anti-affinity — add pod anti-affinity to spread critical workloads across spot and on-demand nodes")
	}

	if s.CriticalOnSpot == 0 && s.SpotInterruption {
		recs = append(recs, fmt.Sprintf("spot instance configuration is healthy — %d workloads on spot with interruption handling and proper tolerations", s.WorkloadsOnSpot))
	}

	return recs
}

// handleSpotReadiness audits spot/preemptible instance readiness and cost optimization.
// GET /api/scalability/spot-readiness
func (s *Server) handleSpotReadiness(w http.ResponseWriter, r *http.Request) {
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
		pods = &corev1.PodList{}
	}

	result := spotReadinessAuditCore(nodes.Items, pods.Items, time.Now())
	writeJSON(w, result)
}
