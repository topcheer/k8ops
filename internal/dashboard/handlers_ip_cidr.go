package dashboard

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IPCIDRResult is the IP address & CIDR utilization analysis.
type IPCIDRResult struct {
	ScannedAt        time.Time     `json:"scannedAt"`
	Summary          IPCIDRSummary `json:"summary"`
	ByNode           []IPNodeStat  `json:"byNode"`
	LowCapacityNodes []IPNodeStat  `json:"lowCapacityNodes"`
	Recommendations  []string      `json:"recommendations"`
}

// IPCIDRSummary aggregates cluster-wide IP utilization.
type IPCIDRSummary struct {
	TotalNodes       int     `json:"totalNodes"`
	NodesWithPodCIDR int     `json:"nodesWithPodCIDR"`
	TotalPodIPsUsed  int     `json:"totalPodIPsUsed"`
	TotalPodCIDRCap  int64   `json:"totalPodCIDRCapacity"`
	OverallUtilPct   float64 `json:"overallUtilizationPct"`
	NodesNearFull    int     `json:"nodesNearFull"` // >80% CIDR used
	NodesFull        int     `json:"nodesFull"`     // 100% CIDR used
	DualStackNodes   int     `json:"dualStackNodes"`
	ServiceIPRange   string  `json:"serviceIPRange"`
	ClusterCIDRs     int     `json:"clusterCIDRs"`
	HealthScore      int     `json:"healthScore"`
}

// IPNodeStat describes IP utilization for one node.
type IPNodeStat struct {
	NodeName     string   `json:"nodeName"`
	PodCIDRs     []string `json:"podCIDRs"`
	CIDRCapacity int64    `json:"cidrCapacity"`
	PodsOnNode   int      `json:"podsOnNode"`
	Utilization  float64  `json:"utilizationPct"`
	Remaining    int64    `json:"remaining"`
	IsDualStack  bool     `json:"isDualStack"`
	RiskLevel    string   `json:"riskLevel"`
}

// handleIPCIDRAudit analyzes IP address and Pod CIDR utilization across nodes.
// GET /api/scalability/ip-cidr-utilization
func (s *Server) handleIPCIDRAudit(w http.ResponseWriter, r *http.Request) {
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

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Count pods per node
	podsPerNode := map[string]int{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	// Detect service IP range from services
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	serviceCIDR := detectServiceCIDR(svcs.Items)

	now := time.Now()
	result := IPCIDRResult{ScannedAt: now, Summary: IPCIDRSummary{ServiceIPRange: serviceCIDR}}
	result.Summary.TotalNodes = len(nodes.Items)

	var totalCapacity int64
	var totalUsed int

	for _, node := range nodes.Items {
		cidrs := node.Spec.PodCIDRs
		if len(cidrs) == 0 && node.Spec.PodCIDR != "" {
			cidrs = []string{node.Spec.PodCIDR}
		}

		if len(cidrs) == 0 {
			continue
		}

		result.Summary.NodesWithPodCIDR++

		// Calculate capacity from CIDR
		// Subtract 2 for network and broadcast (for /31 and smaller)
		// Also consider max-pods which is typically CIDR_size - 2 (kubenet) or node allocatable pods
		var capacity int64
		isDualStack := false
		for _, cidr := range cidrs {
			cap, err := cidrCapacity(cidr)
			if err != nil {
				continue
			}
			// Pod capacity per CIDR is typically cap - 2 (network + broadcast)
			// But k8s reserves more — use min(cap-2, maxPods)
			podCap := cap - 2
			if podCap < 0 {
				podCap = 0
			}
			// For dual-stack, use the IPv4 capacity as primary
			if strings.Contains(cidr, ":") {
				isDualStack = true
			} else {
				capacity += podCap
			}
		}

		// If only IPv6 CIDR (rare), use that
		if capacity == 0 && isDualStack {
			for _, cidr := range cidrs {
				if strings.Contains(cidr, ":") {
					cap, _ := cidrCapacity(cidr)
					capacity = cap - 2
				}
			}
		}

		// Cap at max-pods from allocatable
		maxPods := node.Status.Allocatable.Pods().Value()
		if maxPods > 0 && capacity > maxPods {
			capacity = maxPods
		}

		podCount := podsPerNode[node.Name]
		util := 0.0
		if capacity > 0 {
			util = float64(podCount) / float64(capacity) * 100
		}

		risk := "low"
		if util >= 100 {
			risk = "critical"
			result.Summary.NodesFull++
		} else if util >= 80 {
			risk = "high"
			result.Summary.NodesNearFull++
		} else if util >= 60 {
			risk = "medium"
		}

		stat := IPNodeStat{
			NodeName:     node.Name,
			PodCIDRs:     cidrs,
			CIDRCapacity: capacity,
			PodsOnNode:   podCount,
			Utilization:  float64(int(util*100)) / 100,
			Remaining:    capacity - int64(podCount),
			IsDualStack:  isDualStack,
			RiskLevel:    risk,
		}

		if isDualStack {
			result.Summary.DualStackNodes++
		}

		result.ByNode = append(result.ByNode, stat)
		if risk == "high" || risk == "critical" {
			result.LowCapacityNodes = append(result.LowCapacityNodes, stat)
		}

		totalCapacity += capacity
		totalUsed += podCount
	}

	result.Summary.TotalPodIPsUsed = totalUsed
	result.Summary.TotalPodCIDRCap = totalCapacity
	result.Summary.ClusterCIDRs = len(result.ByNode)

	if totalCapacity > 0 {
		result.Summary.OverallUtilPct = float64(int(float64(totalUsed)/float64(totalCapacity)*10000)) / 100
	}

	// Sort nodes by utilization (highest first)
	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].Utilization > result.ByNode[j].Utilization
	})
	sort.Slice(result.LowCapacityNodes, func(i, j int) bool {
		return result.LowCapacityNodes[i].Utilization > result.LowCapacityNodes[j].Utilization
	})

	result.Summary.HealthScore = ipCIDRScore(result.Summary)
	result.Recommendations = ipCIDRRecommendations(&result)

	writeJSON(w, result)
}

// cidrCapacity calculates the number of addresses in a CIDR range.
func cidrCapacity(cidr string) (int64, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, fmt.Errorf("invalid CIDR %s: %w", cidr, err)
	}
	ones, bits := ipNet.Mask.Size()
	if bits == 0 {
		return 0, fmt.Errorf("invalid mask")
	}
	// 2^(bits-ones) addresses
	hostBits := bits - ones
	if hostBits >= 63 {
		return int64(1) << 62, nil // cap to avoid overflow
	}
	return int64(1) << uint(hostBits), nil
}

// detectServiceCIDR tries to guess the service CIDR from existing service IPs.
func detectServiceCIDR(services []corev1.Service) string {
	if len(services) == 0 {
		return "unknown"
	}
	// Collect all cluster IPs and find the common /24 or /16 prefix
	var ips []net.IP
	for _, svc := range services {
		if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
			ip := net.ParseIP(svc.Spec.ClusterIP)
			if ip != nil && ip.To4() != nil {
				ips = append(ips, ip.To4())
			}
		}
	}
	if len(ips) == 0 {
		return "unknown"
	}
	// Use the first IP's /16 as a guess (most common for small clusters)
	// For more accuracy, would need to compute the common prefix
	if len(ips) > 0 {
		ipNet := &net.IPNet{
			IP:   ips[0].Mask(net.CIDRMask(16, 32)),
			Mask: net.CIDRMask(16, 32),
		}
		return ipNet.String()
	}
	return "unknown"
}

// ipCIDRScore computes a 0-100 health score.
func ipCIDRScore(s IPCIDRSummary) int {
	if s.TotalPodCIDRCap == 0 {
		return 100
	}

	score := 100

	// Penalize overall utilization
	if s.OverallUtilPct > 80 {
		score -= int((s.OverallUtilPct - 80) * 2)
	} else if s.OverallUtilPct > 60 {
		score -= int((s.OverallUtilPct - 60))
	}

	// Penalize full nodes
	if s.NodesFull > 0 {
		score -= min(30, s.NodesFull*10)
	}

	// Penalize near-full nodes
	if s.NodesNearFull > 0 {
		score -= min(20, s.NodesNearFull*5)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// ipCIDRRecommendations generates actionable recommendations.
func ipCIDRRecommendations(r *IPCIDRResult) []string {
	var recs []string

	if r.Summary.NodesFull > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) have exhausted their Pod CIDR — add nodes or expand Pod CIDR range",
			r.Summary.NodesFull,
		))
	}

	if r.Summary.NodesNearFull > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) are near Pod CIDR capacity (>80%%) — plan node expansion",
			r.Summary.NodesNearFull,
		))
	}

	if r.Summary.OverallUtilPct > 70 {
		recs = append(recs, fmt.Sprintf(
			"Cluster-wide Pod IP utilization is %.1f%% — consider expanding cluster CIDR or adding nodes",
			r.Summary.OverallUtilPct,
		))
	}

	if r.Summary.DualStackNodes == 0 && r.Summary.TotalNodes > 0 {
		recs = append(recs, "No dual-stack nodes detected — consider enabling IPv6 dual-stack for larger address space")
	}

	if r.Summary.NodesWithPodCIDR < r.Summary.TotalNodes {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) have no Pod CIDR assigned — check CNI configuration and node registration",
			r.Summary.TotalNodes-r.Summary.NodesWithPodCIDR,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "Pod CIDR utilization is healthy — adequate IP capacity across all nodes")
	}

	return recs
}
