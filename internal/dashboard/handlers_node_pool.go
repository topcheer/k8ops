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

// NodePoolResult is the node pool & cluster autoscaler health audit.
type NodePoolResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         NodePoolSummary     `json:"summary"`
	ByPool          []NodePoolEntry     `json:"byPool"`
	UnhealthyNodes  []NodePoolNode      `json:"unhealthyNodes"`
	StaleNodes      []NodePoolNode      `json:"staleNodes"`
	ByZone          []NodePoolZoneEntry `json:"byZone"`
	Recommendations []string            `json:"recommendations"`
}

// NodePoolSummary aggregates node pool statistics.
type NodePoolSummary struct {
	TotalNodes      int  `json:"totalNodes"`
	ReadyNodes      int  `json:"readyNodes"`
	NotReadyNodes   int  `json:"notReadyNodes"`
	CordonedNodes   int  `json:"cordonedNodes"`
	StaleNodes      int  `json:"staleNodes"` // nodes not seen in >24h (kubelet heartbeat)
	TotalPools      int  `json:"totalPools"`
	UnbalancedPools int  `json:"unbalancedPools"` // pool with >30% not-ready
	HasAutoscaler   bool `json:"hasAutoscaler"`
	HealthScore     int  `json:"healthScore"`
}

// NodePoolEntry describes one node pool's health.
type NodePoolEntry struct {
	Name       string `json:"name"`
	Label      string `json:"label"` // the label key used to identify the pool
	TotalNodes int    `json:"totalNodes"`
	ReadyNodes int    `json:"readyNodes"`
	NotReady   int    `json:"notReadyNodes"`
	Cordoned   int    `json:"cordonedNodes"`
	Zone       string `json:"zone,omitempty"`
	RiskLevel  string `json:"riskLevel"`
}

// NodePoolNode describes a single problematic node.
type NodePoolNode struct {
	Name       string   `json:"name"`
	Pool       string   `json:"pool"`
	Zone       string   `json:"zone,omitempty"`
	Status     string   `json:"status"` // NotReady, Cordoned, Stale
	Conditions []string `json:"conditions"`
	Age        string   `json:"age"`
}

// NodePoolZoneEntry shows node distribution across zones.
type NodePoolZoneEntry struct {
	Zone       string `json:"zone"`
	TotalNodes int    `json:"totalNodes"`
	ReadyNodes int    `json:"readyNodes"`
}

// nodePoolAuditCore performs the audit on node list (testable).
func nodePoolAuditCore(nodes []corev1.Node, autoscalerPods []corev1.Pod) NodePoolResult {
	result := NodePoolResult{
		ScannedAt: time.Now(),
	}

	// Check if cluster autoscaler is running
	result.Summary.HasAutoscaler = len(autoscalerPods) > 0

	// Common node pool labels
	poolLabels := []string{
		"node-role.kubernetes.io/worker",
		"node-role.kubernetes.io/master",
		"node-role.kubernetes.io/control-plane",
		"kubernetes.io/role",
		"node.kubernetes.io/instance-type",
		"node-pool",
		"node-group",
		"pool",
	}

	poolStats := make(map[string]*NodePoolEntry)
	zoneStats := make(map[string]*NodePoolZoneEntry)
	now := time.Now()

	for i := range nodes {
		node := &nodes[i]
		result.Summary.TotalNodes++

		// Determine pool name
		poolName := "default"
		poolLabel := ""
		for _, label := range poolLabels {
			if val, exists := node.Labels[label]; exists {
				if val != "" {
					poolName = val
				} else {
					poolName = label
				}
				poolLabel = label
				break
			}
		}

		if _, ok := poolStats[poolName]; !ok {
			poolStats[poolName] = &NodePoolEntry{Name: poolName, Label: poolLabel}
		}
		poolStats[poolName].TotalNodes++

		// Determine zone
		zone := node.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = "unknown"
		}
		if _, ok := zoneStats[zone]; !ok {
			zoneStats[zone] = &NodePoolZoneEntry{Zone: zone}
		}
		zoneStats[zone].TotalNodes++

		// Check node readiness
		isReady := false
		var conditions []string
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				isReady = true
			}
			if cond.Status == corev1.ConditionTrue && cond.Type != corev1.NodeReady {
				conditions = append(conditions, string(cond.Type))
			}
		}

		// Check if cordoned
		isCordoned := node.Spec.Unschedulable

		age := formatDurationAge(node.CreationTimestamp.Time)

		if isReady && !isCordoned {
			result.Summary.ReadyNodes++
			poolStats[poolName].ReadyNodes++
			zoneStats[zone].ReadyNodes++
		} else if isCordoned {
			result.Summary.CordonedNodes++
			poolStats[poolName].Cordoned++
			result.UnhealthyNodes = append(result.UnhealthyNodes, NodePoolNode{
				Name: node.Name, Pool: poolName, Zone: zone,
				Status: "Cordoned", Conditions: conditions, Age: age,
			})
		} else {
			result.Summary.NotReadyNodes++
			poolStats[poolName].NotReady++
			result.UnhealthyNodes = append(result.UnhealthyNodes, NodePoolNode{
				Name: node.Name, Pool: poolName, Zone: zone,
				Status: "NotReady", Conditions: conditions, Age: age,
			})
		}

		// Check for stale nodes (last heartbeat > 5 min ago based on LastHeartbeatTime)
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				if !cond.LastHeartbeatTime.IsZero() && now.Sub(cond.LastHeartbeatTime.Time) > 5*time.Minute {
					result.Summary.StaleNodes++
					result.StaleNodes = append(result.StaleNodes, NodePoolNode{
						Name: node.Name, Pool: poolName, Zone: zone,
						Status: "Stale", Conditions: conditions, Age: age,
					})
				}
				break
			}
		}
	}

	// Build pool entries
	result.Summary.TotalPools = len(poolStats)
	for _, pool := range poolStats {
		if pool.TotalNodes > 0 && float64(pool.NotReady)/float64(pool.TotalNodes) > 0.3 {
			result.Summary.UnbalancedPools++
			pool.RiskLevel = "high"
		} else if pool.NotReady > 0 {
			pool.RiskLevel = "medium"
		} else if pool.Cordoned > 0 && pool.ReadyNodes == 0 {
			pool.RiskLevel = "critical"
		} else {
			pool.RiskLevel = "low"
		}
		// Find dominant zone
		pool.Zone = findDominantZone(nodes, pool.Name, pool.Label)
		result.ByPool = append(result.ByPool, *pool)
	}
	sort.Slice(result.ByPool, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return riskOrder[result.ByPool[i].RiskLevel] < riskOrder[result.ByPool[j].RiskLevel]
	})

	// Build zone entries
	for _, zone := range zoneStats {
		result.ByZone = append(result.ByZone, *zone)
	}
	sort.Slice(result.ByZone, func(i, j int) bool {
		return result.ByZone[i].TotalNodes > result.ByZone[j].TotalNodes
	})

	result.Summary.HealthScore = nodePoolScore(result.Summary)
	result.Recommendations = nodePoolRecommendations(result.Summary)

	return result
}

// findDominantZone finds the most common zone for a pool.
func findDominantZone(nodes []corev1.Node, poolName, poolLabel string) string {
	zoneCount := make(map[string]int)
	for i := range nodes {
		node := &nodes[i]
		if poolLabel != "" {
			if val, exists := node.Labels[poolLabel]; exists {
				if val != poolName && poolLabel != poolName {
					continue
				}
			}
		}
		zone := node.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = "unknown"
		}
		zoneCount[zone]++
	}
	bestZone := ""
	bestCount := 0
	for zone, count := range zoneCount {
		if count > bestCount {
			bestZone = zone
			bestCount = count
		}
	}
	return bestZone
}

// nodePoolScore calculates health score.
func nodePoolScore(s NodePoolSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}
	base := 100
	// NotReady nodes are critical
	base -= s.NotReadyNodes * 15
	// Stale nodes indicate kubelet issues
	base -= s.StaleNodes * 10
	// Cordoned nodes reduce capacity
	base -= s.CordonedNodes * 3
	// Unbalanced pools indicate systematic issues
	base -= s.UnbalancedPools * 8
	// No autoscaler means no auto-recovery
	if !s.HasAutoscaler {
		base -= 5
	}
	if base < 0 {
		base = 0
	}
	return base
}

// nodePoolRecommendations generates recommendations.
func nodePoolRecommendations(s NodePoolSummary) []string {
	var recs []string
	if s.NotReadyNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d nodes are NotReady — investigate node health, kubelet, and network connectivity", s.NotReadyNodes))
	}
	if s.StaleNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d nodes have stale heartbeats (>5min) — check kubelet status and node networking", s.StaleNodes))
	}
	if s.CordonedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d nodes are cordoned — drain and replace or uncordon if safe", s.CordonedNodes))
	}
	if s.UnbalancedPools > 0 {
		recs = append(recs, fmt.Sprintf("%d node pools have >30%% NotReady nodes — investigate systematic node pool issues", s.UnbalancedPools))
	}
	if !s.HasAutoscaler {
		recs = append(recs, "cluster autoscaler not detected — consider deploying for automatic node scaling")
	}
	if s.NotReadyNodes == 0 && s.StaleNodes == 0 && s.CordonedNodes == 0 {
		recs = append(recs, "all nodes healthy — no node pool issues detected")
	}
	return recs
}

// handleNodePool audits node pool & cluster autoscaler health.
// GET /api/scalability/node-pool-health
func (s *Server) handleNodePool(w http.ResponseWriter, r *http.Request) {
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

	// Check for cluster autoscaler pods
	autoscalerPods, _ := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=cluster-autoscaler",
	})

	result := nodePoolAuditCore(nodes.Items, autoscalerPods.Items)
	writeJSON(w, result)
}

// Suppress unused import
var _ = strings.Contains
