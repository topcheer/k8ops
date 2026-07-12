package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeTopoResult is the node topology distribution & multi-AZ analysis.
type NodeTopoResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         NodeTopoSummary `json:"summary"`
	ByZone          []ZoneStat      `json:"byZone"`
	ByRegion        []RegionStat    `json:"byRegion"`
	Nodes           []NodeTopoEntry `json:"nodes"`
	Risks           []NodeTopoRisk  `json:"risks"`
	Recommendations []string        `json:"recommendations"`
}

// NodeTopoSummary aggregates topology distribution statistics.
type NodeTopoSummary struct {
	TotalNodes          int     `json:"totalNodes"`
	TotalZones          int     `json:"totalZones"`
	TotalRegions        int     `json:"totalRegions"`
	NodesWithoutZone    int     `json:"nodesWithoutZone"`
	SingleZoneCluster   bool    `json:"singleZoneCluster"`
	SingleRegionCluster bool    `json:"singleRegionCluster"`
	MaxZoneImbalance    float64 `json:"maxZoneImbalance"` // max node count diff between zones / total
	HealthScore         int     `json:"healthScore"`
}

// ZoneStat shows node and resource distribution per zone.
type ZoneStat struct {
	Zone       string  `json:"zone"`
	NodeCount  int     `json:"nodeCount"`
	CPUAllocMi int64   `json:"cpuAllocatableMilli"`
	MemAllocMi int64   `json:"memAllocatableMi"`
	PodCount   int     `json:"podCount"`
	CPUPercent float64 `json:"cpuPercent"` // share of total CPU
}

// RegionStat shows node distribution per region.
type RegionStat struct {
	Region    string `json:"region"`
	NodeCount int    `json:"nodeCount"`
}

// NodeTopoEntry describes one node's topology info.
type NodeTopoEntry struct {
	Name     string `json:"name"`
	Zone     string `json:"zone"`
	Region   string `json:"region"`
	IsMaster bool   `json:"isMaster"`
}

// NodeTopoRisk is a detected topology risk.
type NodeTopoRisk struct {
	Category string `json:"category"` // single_zone, imbalance, no_zone_label, etc.
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// handleNodeTopology analyzes node topology distribution and multi-AZ fault tolerance.
// GET /api/scalability/node-topology
func (s *Server) handleNodeTopology(w http.ResponseWriter, r *http.Request) {
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

	// Build pod count per node
	podsPerNode := map[string]int{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" && pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	now := time.Now()
	result := NodeTopoResult{ScannedAt: now}
	result.Summary.TotalNodes = len(nodes.Items)

	zoneMap := map[string]*ZoneStat{}
	regionMap := map[string]int{}

	for _, node := range nodes.Items {
		labels := node.Labels
		if labels == nil {
			labels = map[string]string{}
		}

		zone := labels["topology.kubernetes.io/zone"]
		region := labels["topology.kubernetes.io/region"]

		// Fallback to older labels
		if zone == "" {
			zone = labels["failure-domain.beta.kubernetes.io/zone"]
		}
		if region == "" {
			region = labels["failure-domain.beta.kubernetes.io/region"]
		}

		if zone == "" {
			zone = "unknown"
			result.Summary.NodesWithoutZone++
		}
		if region == "" {
			region = "unknown"
		}

		isMaster := false
		if _, ok := labels["node-role.kubernetes.io/control-plane"]; ok {
			isMaster = true
		}
		if _, ok := labels["node-role.kubernetes.io/master"]; ok {
			isMaster = true
		}

		entry := NodeTopoEntry{
			Name:     node.Name,
			Zone:     zone,
			Region:   region,
			IsMaster: isMaster,
		}
		result.Nodes = append(result.Nodes, entry)

		// Update zone stats
		zs, ok := zoneMap[zone]
		if !ok {
			zs = &ZoneStat{Zone: zone}
			zoneMap[zone] = zs
		}
		zs.NodeCount++
		zs.CPUAllocMi += node.Status.Allocatable.Cpu().MilliValue()
		zs.MemAllocMi += node.Status.Allocatable.Memory().Value() / (1024 * 1024)
		zs.PodCount += podsPerNode[node.Name]

		// Update region stats
		regionMap[region]++

		_ = now
	}

	// Build zone stats slice
	totalCPU := int64(0)
	for _, zs := range zoneMap {
		result.ByZone = append(result.ByZone, *zs)
		totalCPU += zs.CPUAllocMi
	}
	sort.Slice(result.ByZone, func(i, j int) bool {
		return result.ByZone[i].NodeCount > result.ByZone[j].NodeCount
	})

	// Compute CPU percentages
	for i := range result.ByZone {
		if totalCPU > 0 {
			result.ByZone[i].CPUPercent = float64(int(float64(result.ByZone[i].CPUAllocMi)/float64(totalCPU)*10000)) / 100
		}
	}

	// Build region stats
	for region, count := range regionMap {
		result.ByRegion = append(result.ByRegion, RegionStat{Region: region, NodeCount: count})
	}
	sort.Slice(result.ByRegion, func(i, j int) bool {
		return result.ByRegion[i].NodeCount > result.ByRegion[j].NodeCount
	})

	// Summary
	result.Summary.TotalZones = len(zoneMap)
	result.Summary.TotalRegions = len(regionMap)
	result.Summary.SingleZoneCluster = len(zoneMap) <= 1
	result.Summary.SingleRegionCluster = len(regionMap) <= 1

	// Compute max zone imbalance
	if len(zoneMap) > 1 {
		maxCount := 0
		minCount := len(nodes.Items)
		for _, zs := range zoneMap {
			if zs.NodeCount > maxCount {
				maxCount = zs.NodeCount
			}
			if zs.NodeCount < minCount {
				minCount = zs.NodeCount
			}
		}
		if len(nodes.Items) > 0 {
			result.Summary.MaxZoneImbalance = float64(int(float64(maxCount-minCount)/float64(len(nodes.Items))*10000)) / 100
		}
	}

	// Generate risks
	if result.Summary.SingleZoneCluster && len(nodes.Items) > 1 {
		result.Risks = append(result.Risks, NodeTopoRisk{
			Category: "single_zone",
			Severity: "critical",
			Message:  "All nodes are in a single zone — no fault tolerance against zone failure",
		})
	}

	if result.Summary.NodesWithoutZone > 0 {
		result.Risks = append(result.Risks, NodeTopoRisk{
			Category: "no_zone_label",
			Severity: "medium",
			Message:  fmt.Sprintf("%d node(s) have no zone label — topology spread constraints will not work correctly", result.Summary.NodesWithoutZone),
		})
	}

	if result.Summary.MaxZoneImbalance > 30 {
		result.Risks = append(result.Risks, NodeTopoRisk{
			Category: "imbalance",
			Severity: "high",
			Message:  fmt.Sprintf("Zone imbalance is %.1f%% — nodes are unevenly distributed across zones", result.Summary.MaxZoneImbalance),
		})
	}

	if result.Summary.SingleRegionCluster && len(nodes.Items) > 2 {
		result.Risks = append(result.Risks, NodeTopoRisk{
			Category: "single_region",
			Severity: "low",
			Message:  "All nodes are in a single region — consider multi-region deployment for DR",
		})
	}

	// Sort risks by severity
	sort.Slice(result.Risks, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Risks[i].Severity] < sevOrder[result.Risks[j].Severity]
	})

	result.Summary.HealthScore = nodeTopoScore(result.Summary)
	result.Recommendations = nodeTopoRecommendations(&result)

	writeJSON(w, result)
}

// nodeTopoScore computes a 0-100 health score.
func nodeTopoScore(s NodeTopoSummary) int {
	if s.TotalNodes == 0 {
		return 100
	}

	score := 100

	if s.SingleZoneCluster {
		score -= 40
	}

	if s.NodesWithoutZone > 0 {
		score -= min(15, s.NodesWithoutZone*5)
	}

	if s.MaxZoneImbalance > 30 {
		score -= min(20, int(s.MaxZoneImbalance/3))
	}

	if s.SingleRegionCluster {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	return score
}

// nodeTopoRecommendations generates actionable recommendations.
func nodeTopoRecommendations(r *NodeTopoResult) []string {
	var recs []string

	if r.Summary.SingleZoneCluster && r.Summary.TotalNodes > 1 {
		recs = append(recs, "Cluster is single-zone — add nodes in different availability zones for fault tolerance against zone failures")
	}

	if r.Summary.NodesWithoutZone > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d node(s) missing zone labels — add topology.kubernetes.io/zone label for topology spread constraints to work",
			r.Summary.NodesWithoutZone,
		))
	}

	if r.Summary.MaxZoneImbalance > 30 {
		recs = append(recs, fmt.Sprintf(
			"Zone imbalance is %.1f%% — rebalance nodes across zones for better fault tolerance",
			r.Summary.MaxZoneImbalance,
		))
	}

	if len(r.ByZone) >= 3 {
		// Check if any zone has < 2 nodes
		for _, zs := range r.ByZone {
			if zs.NodeCount < 2 && zs.Zone != "unknown" {
				recs = append(recs, fmt.Sprintf(
					"Zone %s has only %d node(s) — add at least 2 per zone for quorum-style fault tolerance",
					zs.Zone, zs.NodeCount,
				))
				break
			}
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "Node topology distribution is healthy — nodes are well distributed across multiple zones")
	}

	return recs
}
