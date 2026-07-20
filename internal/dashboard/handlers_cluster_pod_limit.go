package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterPodLimitResult forecasts when cluster runs out of pod IP / allocation capacity.
type ClusterPodLimitResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         PodLimitSummary     `json:"summary"`
	ByNode          []PodLimitNodeEntry `json:"byNode"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type PodLimitSummary struct {
	TotalNodes        int     `json:"totalNodes"`
	TotalPodCapacity  int     `json:"totalPodCapacity"`
	CurrentPods       int     `json:"currentPods"`
	HeadroomPods      int     `json:"headroomPods"`
	UtilizationPct    float64 `json:"utilizationPct"`
	MaxPodsPerNode    int     `json:"maxPodsPerNode"`
	LimitReachedNodes int     `json:"nodesAtPodLimit"`
	CIDRRangeBits     int     `json:"podCIDRBits"`
	TotalIPs          int     `json:"totalPodIPs"`
}

type PodLimitNodeEntry struct {
	NodeName    string  `json:"nodeName"`
	PodCapacity int     `json:"podCapacity"`
	CurrentPods int     `json:"currentPods"`
	Headroom    int     `json:"headroom"`
	Utilization float64 `json:"utilizationPct"`
	RiskLevel   string  `json:"riskLevel"`
}

// handleClusterPodLimit handles GET /api/scalability/cluster-pod-limit
func (s *Server) handleClusterPodLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ClusterPodLimitResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node pod count map
	nodePodCount := make(map[string]int)
	result.Summary.CurrentPods = 0
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase != corev1.PodFailed {
			nodePodCount[pod.Spec.NodeName]++
			result.Summary.CurrentPods++
		}
	}

	maxPodsPerNode := 110 // default k8s

	for _, node := range nodes.Items {
		isMaster := false
		for k := range node.Labels {
			if k == "node-role.kubernetes.io/control-plane" || k == "node-role.kubernetes.io/master" {
				isMaster = true
			}
		}
		result.Summary.TotalNodes++

		// Get max pods from allocatable or system config
		capacity := 110
		if pc, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			capacity = int(pc.Value())
			if capacity > maxPodsPerNode {
				maxPodsPerNode = capacity
			}
		}
		if isMaster {
			// Master nodes typically have lower pod limits
			capacity = capacity / 2
		}

		current := nodePodCount[node.Name]
		headroom := capacity - current
		if headroom < 0 {
			headroom = 0
		}
		util := 0.0
		if capacity > 0 {
			util = float64(current) / float64(capacity) * 100
		}

		entry := PodLimitNodeEntry{
			NodeName:    node.Name,
			PodCapacity: capacity,
			CurrentPods: current,
			Headroom:    headroom,
			Utilization: util,
		}

		switch {
		case util >= 95:
			entry.RiskLevel = "critical"
			result.Summary.LimitReachedNodes++
		case util >= 80:
			entry.RiskLevel = "high"
		case util >= 60:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		result.Summary.TotalPodCapacity += capacity
		result.ByNode = append(result.ByNode, entry)
	}

	result.Summary.MaxPodsPerNode = maxPodsPerNode
	result.Summary.HeadroomPods = result.Summary.TotalPodCapacity - result.Summary.CurrentPods
	if result.Summary.HeadroomPods < 0 {
		result.Summary.HeadroomPods = 0
	}
	if result.Summary.TotalPodCapacity > 0 {
		result.Summary.UtilizationPct = float64(result.Summary.CurrentPods) / float64(result.Summary.TotalPodCapacity) * 100
	}

	// Estimate pod CIDR
	result.Summary.CIDRRangeBits = 24
	result.Summary.TotalIPs = 256 * result.Summary.TotalNodes // /24 per node approx

	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].Utilization > result.ByNode[j].Utilization
	})

	result.HealthScore = 100 - int(result.Summary.UtilizationPct)
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Pod 容量上限: %d/%d (%.1f%%), 余量 %d, 最大/Pod/节点=%d",
			result.Summary.CurrentPods, result.Summary.TotalPodCapacity,
			result.Summary.UtilizationPct, result.Summary.HeadroomPods,
			result.Summary.MaxPodsPerNode),
	}
	if result.Summary.LimitReachedNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个节点已达到 Pod 上限", result.Summary.LimitReachedNodes))
	}
	if result.Summary.UtilizationPct > 70 {
		result.Recommendations = append(result.Recommendations, "Pod 利用率超 70%, 考虑增加节点或提升 maxPods")
	}
	writeJSON(w, result)
}
