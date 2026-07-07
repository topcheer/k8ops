package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// K8s recommended limits
const (
	k8sMaxNodes           = 5000
	k8sMaxPodsPerNode     = 110
	k8sMaxTotalPods       = 150000
	k8sMaxServices        = 5000
	k8sMaxServicesPerNode = 20 // kube-proxy iptables mode
	k8sMaxConfigMapsPerNS = 0  // no hard limit, but >1000 is problematic
	k8sMaxTotalNamespaces = 10000
	k8sMaxCRDs            = 500 // practical limit
)

// SBResult is the scalability bottleneck prediction.
type SBResult struct {
	ScannedAt       time.Time     `json:"scannedAt"`
	Summary         SBSummary     `json:"summary"`
	ByResource      []SBEntry     `json:"byResource"`  // each resource type with ratio
	Bottlenecks     []SBEntry     `json:"bottlenecks"` // resources >70% of limit
	ByNode          []SBNodeEntry `json:"byNode"`      // per-node bottleneck risk
	Recommendations []string      `json:"recommendations"`
}

// SBSummary aggregates scalability bottleneck stats.
type SBSummary struct {
	TotalNodes      int     `json:"totalNodes"`
	MaxPodsPerNode  float64 `json:"maxPodsPerNode"` // actual max
	AvgPodsPerNode  float64 `json:"avgPodsPerNode"`
	TotalPods       int     `json:"totalPods"`
	TotalServices   int     `json:"totalServices"`
	TotalNamespaces int     `json:"totalNamespaces"`
	BottleneckType  string  `json:"bottleneckType"`  // which resource is the bottleneck
	BottleneckRatio float64 `json:"bottleneckRatio"` // % of limit
	RiskScore       int     `json:"riskScore"`       // 0-100 (higher = better, less risk)
}

// SBEntry describes one resource type's scalability ratio.
type SBEntry struct {
	Resource     string  `json:"resource"` // e.g. "pods_per_node"
	Current      float64 `json:"current"`
	Limit        float64 `json:"limit"`
	Ratio        float64 `json:"ratio"`        // current/limit (0-1)
	RatioPercent float64 `json:"ratioPercent"` // 0-100
	Status       string  `json:"status"`       // healthy / warning / critical / bottleneck
	Description  string  `json:"description"`
}

// SBNodeEntry per-node scalability risk.
type SBNodeEntry struct {
	NodeName     string  `json:"nodeName"`
	PodCount     int     `json:"podCount"`
	PodRatio     float64 `json:"podRatio"` // pods / 110
	ServiceCount int     `json:"serviceCount,omitempty"`
	RiskLevel    string  `json:"riskLevel"`
}

// handleScalabilityBottleneck predicts which K8s resource will become the bottleneck first.
// GET /api/scalability/bottleneck-predictor
func (s *Server) handleScalabilityBottleneck(w http.ResponseWriter, r *http.Request) {
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

	svcs, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	nss, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	totalNodes := len(nodes.Items)
	totalPods := 0
	totalSvcs := len(svcs.Items)
	totalNS := len(nss.Items)

	// Per-node pod counts
	nodePods := make(map[string]int)
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed" {
			continue
		}
		if pod.Spec.NodeName != "" {
			nodePods[pod.Spec.NodeName]++
			totalPods++
		}
	}

	// Per-node service endpoints (approximate by counting services in namespaces on that node)
	result := SBResult{ScannedAt: time.Now()}
	result.Summary.TotalNodes = totalNodes
	result.Summary.TotalPods = totalPods
	result.Summary.TotalServices = totalSvcs
	result.Summary.TotalNamespaces = totalNS

	// Calculate resource ratios
	var maxPodsPerNode float64
	var totalPodsOnNodes int
	for _, count := range nodePods {
		if float64(count) > maxPodsPerNode {
			maxPodsPerNode = float64(count)
		}
		totalPodsOnNodes += count
	}
	if totalNodes > 0 {
		result.Summary.AvgPodsPerNode = float64(totalPodsOnNodes) / float64(totalNodes)
	}
	result.Summary.MaxPodsPerNode = maxPodsPerNode

	// Build resource entries
	var entries []SBEntry

	// Pods per node (max)
	entries = append(entries, sbMakeEntry("max_pods_per_node", maxPodsPerNode, k8sMaxPodsPerNode,
		"Maximum pods on any single node vs K8s limit of 110"))

	// Average pods per node
	if totalNodes > 0 {
		entries = append(entries, sbMakeEntry("avg_pods_per_node", result.Summary.AvgPodsPerNode, k8sMaxPodsPerNode,
			"Average pods per node vs K8s limit of 110"))
	}

	// Total pods
	entries = append(entries, sbMakeEntry("total_pods", float64(totalPods), k8sMaxTotalPods,
		"Total cluster pods vs K8s recommended max of 150,000"))

	// Total services
	entries = append(entries, sbMakeEntry("total_services", float64(totalSvcs), k8sMaxServices,
		"Total cluster services vs K8s recommended max of 5,000"))

	// Services per node (approximate)
	if totalNodes > 0 {
		svcsPerNode := float64(totalSvcs) / float64(totalNodes)
		entries = append(entries, sbMakeEntry("services_per_node", svcsPerNode, k8sMaxServicesPerNode,
			"Average services per node vs kube-proxy practical limit of 20"))
	}

	// Total nodes
	entries = append(entries, sbMakeEntry("total_nodes", float64(totalNodes), k8sMaxNodes,
		"Cluster node count vs K8s recommended max of 5,000"))

	// Namespaces
	entries = append(entries, sbMakeEntry("total_namespaces", float64(totalNS), k8sMaxTotalNamespaces,
		"Namespace count vs K8s practical max of 10,000"))

	// Find bottleneck
	for _, e := range entries {
		if e.Ratio > 0.7 {
			result.Bottlenecks = append(result.Bottlenecks, e)
		}
	}

	// Sort by ratio descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Ratio > entries[j].Ratio
	})
	result.ByResource = entries

	// Identify primary bottleneck
	if len(entries) > 0 {
		top := entries[0]
		result.Summary.BottleneckType = top.Resource
		result.Summary.BottleneckRatio = top.RatioPercent
	}

	// Per-node bottleneck entries
	for _, node := range nodes.Items {
		podCount := nodePods[node.Name]
		entry := SBNodeEntry{
			NodeName: node.Name,
			PodCount: podCount,
			PodRatio: float64(podCount) / float64(k8sMaxPodsPerNode),
		}
		if entry.PodRatio > 0.8 {
			entry.RiskLevel = "critical"
		} else if entry.PodRatio > 0.6 {
			entry.RiskLevel = "high"
		} else if entry.PodRatio > 0.4 {
			entry.RiskLevel = "medium"
		} else {
			entry.RiskLevel = "low"
		}
		result.ByNode = append(result.ByNode, entry)
	}
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].PodCount > result.ByNode[j].PodCount
	})
	if len(result.ByNode) > 20 {
		result.ByNode = result.ByNode[:20]
	}

	result.Summary.RiskScore = sbScore(entries)
	result.Recommendations = sbGenRecs(result.Summary, result.Bottlenecks, result.ByNode)

	writeJSON(w, result)
}

// sbMakeEntry creates a resource scalability entry.
func sbMakeEntry(resource string, current, limit float64, desc string) SBEntry {
	ratio := 0.0
	if limit > 0 {
		ratio = current / limit
	}
	status := "healthy"
	if ratio > 0.9 {
		status = "bottleneck"
	} else if ratio > 0.7 {
		status = "critical"
	} else if ratio > 0.5 {
		status = "warning"
	}
	return SBEntry{
		Resource:     resource,
		Current:      current,
		Limit:        limit,
		Ratio:        ratio,
		RatioPercent: ratio * 100,
		Status:       status,
		Description:  desc,
	}
}

// sbScore computes risk score (100 = safe, 0 = imminent bottleneck).
func sbScore(entries []SBEntry) int {
	minSafe := 100
	for _, e := range entries {
		safe := int((1 - e.Ratio) * 100)
		if safe < minSafe {
			minSafe = safe
		}
	}
	if minSafe < 0 {
		minSafe = 0
	}
	return minSafe
}

// sbGenRecs produces actionable advice.
func sbGenRecs(summary SBSummary, bottlenecks []SBEntry, byNode []SBNodeEntry) []string {
	var recs []string

	if len(bottlenecks) > 0 {
		top := bottlenecks[0]
		recs = append(recs, fmt.Sprintf("Primary bottleneck: %s at %.1f%% of K8s limit (%.0f/%.0f) — plan capacity expansion", top.Resource, top.RatioPercent, top.Current, top.Limit))
	}
	if summary.MaxPodsPerNode > 90 {
		recs = append(recs, fmt.Sprintf("Max pods on a single node is %.0f (limit: 110) — add more nodes or redistribute workloads", summary.MaxPodsPerNode))
	}
	if summary.AvgPodsPerNode > 80 {
		recs = append(recs, fmt.Sprintf("Average pods per node is %.1f — approaching per-node limit, add nodes", summary.AvgPodsPerNode))
	}
	// Check for nodes near limit
	nearLimitCount := 0
	for _, n := range byNode {
		if n.PodRatio > 0.8 {
			nearLimitCount++
		}
	}
	if nearLimitCount > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are >80%% full (>88 pods) — redistribute pods or add nodes", nearLimitCount))
	}
	if summary.TotalServices > 3000 {
		recs = append(recs, fmt.Sprintf("%d services — approaching K8s limit of 5000, kube-proxy overhead increases", summary.TotalServices))
	}
	if summary.TotalNamespaces > 5000 {
		recs = append(recs, fmt.Sprintf("%d namespaces — high count impacts API server etcd performance", summary.TotalNamespaces))
	}
	if summary.RiskScore < 30 {
		recs = append(recs, fmt.Sprintf("Cluster is at scalability risk (score: %d/100) — immediate capacity planning needed", summary.RiskScore))
	}
	if len(bottlenecks) == 0 {
		recs = append(recs, fmt.Sprintf("No scalability bottlenecks detected — cluster has ample headroom (score: %d/100)", summary.RiskScore))
	}

	return recs
}
