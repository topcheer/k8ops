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

// TopologyResult is the full topology spread analysis output.
type TopologyResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         TopologySummary    `json:"summary"`
	Workloads       []TopologyWorkload `json:"workloads"`
	Nodes           []TopologyNodeInfo `json:"nodes"`
	Recommendations []string           `json:"recommendations"`
}

// TopologySummary aggregates cluster-wide topology metrics.
type TopologySummary struct {
	TotalDomains      int     `json:"totalDomains"`
	DomainKey         string  `json:"domainKey"` // e.g. "topology.kubernetes.io/zone"
	TotalWorkloads    int     `json:"totalWorkloads"`
	BalancedWorkloads int     `json:"balancedWorkloads"`
	SkewedWorkloads   int     `json:"skewedWorkloads"`
	NoConstraintWL    int     `json:"noConstraintWorkloads"`
	MaxSkew           int     `json:"maxSkew"`
	AvgSkew           float64 `json:"avgSkew"`
}

// TopologyWorkload describes spread status for a group of related pods.
type TopologyWorkload struct {
	Name              string               `json:"name"`
	Namespace         string               `json:"namespace"`
	Kind              string               `json:"kind"`
	Replicas          int                  `json:"replicas"`
	DomainKey         string               `json:"domainKey"`
	HasConstraint     bool                 `json:"hasConstraint"`
	WhenUnsatisfiable string               `json:"whenUnsatisfiable"`
	MaxSkew           int                  `json:"maxSkew"`
	Distribution      []DomainDistribution `json:"distribution"`
	ActualSkew        int                  `json:"actualSkew"`
	Status            string               `json:"status"` // balanced, skewed, no-constraint, single-replica
}

// DomainDistribution shows pod count per topology domain.
type DomainDistribution struct {
	Domain   string `json:"domain"`
	PodCount int    `json:"podCount"`
	Expected int    `json:"expected"`
}

// TopologyNodeInfo shows which domain each node belongs to.
type TopologyNodeInfo struct {
	Name     string `json:"name"`
	Ready    bool   `json:"ready"`
	Domain   string `json:"domain"`
	PodCount int    `json:"podCount"`
}

// handleTopologySpread analyzes pod distribution across topology domains.
// GET /api/topology/spread?namespace=xxx&domain=topology.kubernetes.io/zone
func (s *Server) handleTopologySpread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	domainKey := r.URL.Query().Get("domain")
	if domainKey == "" {
		domainKey = "kubernetes.io/hostname" // default to node-level spread
	}

	// Fetch nodes and pods
	nodeList, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	podList, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build node -> domain mapping
	nodeDomain := make(map[string]string)
	domainSet := make(map[string]bool)
	for _, node := range nodeList.Items {
		domain := getNodeDomain(&node, domainKey)
		nodeDomain[node.Name] = domain
		domainSet[domain] = true
	}

	result := TopologyResult{
		ScannedAt: time.Now(),
		Summary: TopologySummary{
			DomainKey:    domainKey,
			TotalDomains: len(domainSet),
		},
	}

	// Build node info
	for _, node := range nodeList.Items {
		podCount := 0
		for _, pod := range podList.Items {
			if pod.Spec.NodeName == node.Name && pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
				podCount++
			}
		}
		result.Nodes = append(result.Nodes, TopologyNodeInfo{
			Name:     node.Name,
			Ready:    isNodeReady(&node),
			Domain:   nodeDomain[node.Name],
			PodCount: podCount,
		})
	}
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].PodCount > result.Nodes[j].PodCount
	})

	// Group pods by workload (owner reference)
	workloadPods := groupPodsByWorkload(podList.Items)

	// Analyze each workload's topology spread
	totalSkew := 0
	skewCount := 0

	for wlKey, pods := range workloadPods {
		// Filter out completed/failed pods
		var activePods []corev1.Pod
		for _, pod := range pods {
			if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed && pod.Spec.NodeName != "" {
				activePods = append(activePods, pod)
			}
		}
		if len(activePods) == 0 {
			continue
		}

		tw := analyzeWorkloadTopology(wlKey, activePods, nodeDomain, domainKey, len(domainSet))
		result.Workloads = append(result.Workloads, tw)

		result.Summary.TotalWorkloads++
		switch tw.Status {
		case "balanced":
			result.Summary.BalancedWorkloads++
		case "skewed":
			result.Summary.SkewedWorkloads++
			totalSkew += tw.ActualSkew
			skewCount++
		case "no-constraint":
			result.Summary.NoConstraintWL++
		}

		if tw.ActualSkew > result.Summary.MaxSkew {
			result.Summary.MaxSkew = tw.ActualSkew
		}
	}

	if skewCount > 0 {
		result.Summary.AvgSkew = float64(totalSkew) / float64(skewCount)
	}

	// Sort workloads by skew descending
	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].ActualSkew > result.Workloads[j].ActualSkew
	})

	// Generate recommendations
	result.Recommendations = generateTopologyRecommendations(result)

	writeJSON(w, result)
}

// analyzeWorkloadTopology checks a single workload's pod distribution.
func analyzeWorkloadTopology(wlKey string, pods []corev1.Pod, nodeDomain map[string]string, domainKey string, totalDomains int) TopologyWorkload {
	parts := strings.SplitN(wlKey, "/", 3)
	ns, kind, name := "default", "Pod", "unknown"
	if len(parts) >= 3 {
		ns, kind, name = parts[0], parts[1], parts[2]
	}

	tw := TopologyWorkload{
		Name:      name,
		Namespace: ns,
		Kind:      kind,
		Replicas:  len(pods),
		DomainKey: domainKey,
	}

	// Extract topology spread constraints from first pod
	if len(pods) > 0 {
		constraints := pods[0].Spec.TopologySpreadConstraints
		if len(constraints) > 0 {
			for _, tsc := range constraints {
				if tsc.TopologyKey == domainKey || domainKey == "kubernetes.io/hostname" {
					tw.HasConstraint = true
					tw.WhenUnsatisfiable = string(tsc.WhenUnsatisfiable)
					if tsc.MaxSkew > 0 {
						tw.MaxSkew = int(tsc.MaxSkew)
					} else {
						tw.MaxSkew = 1 // default
					}
					break
				}
			}
		}
	}

	// Count pods per domain
	domainCount := make(map[string]int)
	for _, pod := range pods {
		domain := nodeDomain[pod.Spec.NodeName]
		if domain == "" {
			domain = "<unknown>"
		}
		domainCount[domain]++
	}

	// Build distribution
	expectedPerDomain := len(pods) / totalDomains
	if expectedPerDomain == 0 {
		expectedPerDomain = 1
	}

	for domain, count := range domainCount {
		tw.Distribution = append(tw.Distribution, DomainDistribution{
			Domain:   domain,
			PodCount: count,
			Expected: expectedPerDomain,
		})
	}
	sort.Slice(tw.Distribution, func(i, j int) bool {
		return tw.Distribution[i].PodCount > tw.Distribution[j].PodCount
	})

	// Calculate actual skew
	if len(tw.Distribution) > 1 {
		max := tw.Distribution[0].PodCount
		min := tw.Distribution[len(tw.Distribution)-1].PodCount
		tw.ActualSkew = max - min
	}

	// Determine status
	if len(pods) == 1 {
		tw.Status = "single-replica"
	} else if !tw.HasConstraint {
		if tw.ActualSkew > 1 && len(domainCount) > 1 {
			tw.Status = "no-constraint"
		} else {
			tw.Status = "balanced"
		}
	} else {
		if tw.ActualSkew <= tw.MaxSkew {
			tw.Status = "balanced"
		} else {
			tw.Status = "skewed"
		}
	}

	return tw
}

// groupPodsByWorkload groups pods by their owner reference (workload).
func groupPodsByWorkload(pods []corev1.Pod) map[string][]corev1.Pod {
	groups := make(map[string][]corev1.Pod)
	for _, pod := range pods {
		kind := "Pod"
		name := pod.Name
		if len(pod.OwnerReferences) > 0 {
			kind = pod.OwnerReferences[0].Kind
			name = pod.OwnerReferences[0].Name
		}
		key := fmt.Sprintf("%s/%s/%s", pod.Namespace, kind, name)
		groups[key] = append(groups[key], pod)
	}
	return groups
}

// getNodeDomain extracts the domain value for a given topology key.
func getNodeDomain(node *corev1.Node, topologyKey string) string {
	// Check node labels
	if val, ok := node.Labels[topologyKey]; ok {
		return val
	}
	// If topologyKey is hostname, return node name
	if topologyKey == "kubernetes.io/hostname" {
		return node.Name
	}
	return "<unlabeled>"
}

// generateTopologyRecommendations produces actionable recommendations.
func generateTopologyRecommendations(result TopologyResult) []string {
	var recs []string

	if result.Summary.SkewedWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have topology skew exceeding maxSkew — pods are not evenly distributed across %s domains", result.Summary.SkewedWorkloads, result.Summary.DomainKey))
	}

	if result.Summary.NoConstraintWL > 0 {
		recs = append(recs, fmt.Sprintf("%d multi-replica workload(s) have no topology spread constraints — add topologySpreadConstraints to improve availability", result.Summary.NoConstraintWL))
	}

	if result.Summary.MaxSkew > 2 {
		recs = append(recs, fmt.Sprintf("Maximum topology skew is %d — some workloads are heavily concentrated on a single %s", result.Summary.MaxSkew, result.Summary.DomainKey))
	}

	// Check for nodes in unlabeled domains
	for _, n := range result.Nodes {
		if n.Domain == "<unlabeled>" {
			recs = append(recs, fmt.Sprintf("Node %q has no %s label — topology spread cannot be evaluated for this node", n.Name, result.Summary.DomainKey))
			break
		}
	}

	if result.Summary.TotalDomains == 1 {
		recs = append(recs, fmt.Sprintf("Only 1 %s domain detected — topology spread provides no benefit without multiple domains. Consider labeling nodes with zone/region labels", result.Summary.DomainKey))
	}

	return recs
}
