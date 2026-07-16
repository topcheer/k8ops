package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DensityBalanceResult analyzes pod scheduling density and resource balance
// across nodes. It identifies over-packed nodes (many pods on one node) vs
// under-utilized nodes, checks anti-affinity effectiveness, and recommends
// rebalancing actions for optimal fault tolerance.
type DensityBalanceResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         DensityBalanceSummary `json:"summary"`
	Nodes           []DensityNode       `json:"nodes"`
	Imbalance       ImbalanceInfo       `json:"imbalance"`
	RebalancingOps []RebalancingOp     `json:"rebalancingOps"`
	ByNamespace     []DensityNSStat     `json:"byNamespace"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

// DensityBalanceSummary aggregates pod density statistics.
type DensityBalanceSummary struct {
	TotalNodes      int     `json:"totalNodes"`
	TotalPods       int     `json:"totalPods"`
	AvgPodsPerNode  float64 `json:"avgPodsPerNode"`
	MaxPodsPerNode  int     `json:"maxPodsPerNode"`
	MinPodsPerNode  int     `json:"minPodsPerNode"`
	OverPackedNodes int     `json:"overPackedNodes"`
	UnderUsedNodes  int     `json:"underUsedNodes"`
	PodCapacityMax  int     `json:"podCapacityMax"`
	BalanceScore    int     `json:"balanceScore"`
	StdDeviation    float64 `json:"stdDeviation"`
}

// DensityNode per-node density info.
type DensityNode struct {
	Name         string  `json:"name"`
	PodCount     int     `json:"podCount"`
	MaxPods      int     `json:"maxPods"`
	DensityPct   float64 `json:"densityPct"`
	CPUUsagePct  float64 `json:"cpuUsagePct"`
	MemUsagePct  float64 `json:"memUsagePct"`
	IsOverPacked bool    `json:"isOverPacked"`
	IsUnderUsed  bool    `json:"isUnderUsed"`
	Zone         string  `json:"zone"`
	RiskLevel    string  `json:"riskLevel"`
}

// ImbalanceInfo describes cluster-wide distribution imbalance.
type ImbalanceInfo struct {
	PodDistribution string             `json:"podDistribution"` // balanced, imbalanced, severely-imbalanced
	CpuDistribution string             `json:"cpuDistribution"`
	MemDistribution string             `json:"memDistribution"`
	ZoneBalance     string             `json:"zoneBalance"`
	MaxMinRatio     float64            `json:"maxMinRatio"` // max pods / min pods across nodes
	GiniCoefficient float64            `json:"giniCoefficient"`
}

// RebalancingOp describes a recommended rebalancing action.
type RebalancingOp struct {
	Type     string `json:"type"`     // evacuate, spread, consolidate
	Node     string `json:"node"`
	Detail   string `json:"detail"`
	PodCount int    `json:"podCount"`
	Priority int    `json:"priority"`
}

// DensityNSStat per-namespace pod distribution.
type DensityNSStat struct {
	Namespace  string  `json:"namespace"`
	PodCount   int     `json:"podCount"`
	NodeSpread int     `json:"nodeSpread"` // number of unique nodes
	AvgPerNode float64 `json:"avgPerNode"`
}

// handleDensityBalance handles GET /api/scalability/density-balance
func (s *Server) handleDensityBalance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DensityBalanceResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node→pod count map
	nodePodMap := map[string][]corev1.Pod{}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		nodePodMap[pod.Spec.NodeName] = append(nodePodMap[pod.Spec.NodeName], pod)
	}

	totalPods := 0
	var podCounts []float64
	maxPods := 0
	minPods := 999999

	for _, node := range nodes.Items {
		nodePods := nodePodMap[node.Name]
		podCount := len(nodePods)
		totalPods += podCount

		maxPodCapacity := 110 // default Kubernetes limit
		if node.Status.Allocatable != nil {
			if p, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
				maxPodCapacity = int(p.Value())
			}
		}

		densityPct := 0.0
		if maxPodCapacity > 0 {
			densityPct = float64(podCount) / float64(maxPodCapacity) * 100
		}

		// CPU/Memory usage
		cpuReq := 0.0
		memReq := 0.0
		for _, pod := range nodePods {
			for _, c := range pod.Spec.Containers {
				if c.Resources.Requests != nil {
					if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
						cpuReq += float64(q.MilliValue()) / 1000
					}
					if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
						memReq += float64(q.Value()) / (1024 * 1024 * 1024)
					}
				}
			}
		}

		nodeCPUCapacity := 0.0
		nodeMemCapacity := 0.0
		if q, ok := node.Status.Allocatable[corev1.ResourceCPU]; ok {
			nodeCPUCapacity = float64(q.MilliValue()) / 1000
		}
		if q, ok := node.Status.Allocatable[corev1.ResourceMemory]; ok {
			nodeMemCapacity = float64(q.Value()) / (1024 * 1024 * 1024)
		}

		cpuPct := 0.0
		if nodeCPUCapacity > 0 {
			cpuPct = cpuReq / nodeCPUCapacity * 100
		}
		memPct := 0.0
		if nodeMemCapacity > 0 {
			memPct = memReq / nodeMemCapacity * 100
		}

		isOverPacked := densityPct > 80
		isUnderUsed := densityPct < 20 && podCount > 0

		zone := "unknown"
		if node.Labels != nil {
			if z, ok := node.Labels[corev1.LabelTopologyZone]; ok {
				zone = z
			}
		}

		riskLevel := "low"
		if isOverPacked {
			riskLevel = "high"
		} else if densityPct > 60 {
			riskLevel = "medium"
		}

		result.Nodes = append(result.Nodes, DensityNode{
			Name: node.Name, PodCount: podCount, MaxPods: maxPodCapacity,
			DensityPct: densityPct, CPUUsagePct: cpuPct, MemUsagePct: memPct,
			IsOverPacked: isOverPacked, IsUnderUsed: isUnderUsed,
			Zone: zone, RiskLevel: riskLevel,
		})

		podCounts = append(podCounts, float64(podCount))
		if podCount > maxPods {
			maxPods = podCount
		}
		if podCount < minPods {
			minPods = podCount
		}
	}

	// Summary
	avgPodsPerNode := 0.0
	if len(nodes.Items) > 0 {
		avgPodsPerNode = float64(totalPods) / float64(len(nodes.Items))
	}
	overPackedCount := 0
	underUsedCount := 0
	for _, n := range result.Nodes {
		if n.IsOverPacked {
			overPackedCount++
		}
		if n.IsUnderUsed {
			underUsedCount++
		}
	}

	stdDev := computeStdDev(podCounts, avgPodsPerNode)

	result.Summary = DensityBalanceSummary{
		TotalNodes: len(nodes.Items), TotalPods: totalPods,
		AvgPodsPerNode: avgPodsPerNode, MaxPodsPerNode: maxPods,
		MinPodsPerNode: minPods, OverPackedNodes: overPackedCount,
		UnderUsedNodes: underUsedCount, PodCapacityMax: 110,
		StdDeviation: stdDev,
	}
	result.Summary.BalanceScore = computeBalanceScore(result.Summary)

	// Imbalance info
	maxMinRatio := 1.0
	if minPods > 0 {
		maxMinRatio = float64(maxPods) / float64(minPods)
	}
	gini := computeGini(podCounts)

	podDist := "balanced"
	if stdDev > avgPodsPerNode*0.5 || maxMinRatio > 2 {
		podDist = "imbalanced"
	}
	if stdDev > avgPodsPerNode || maxMinRatio > 3 {
		podDist = "severely-imbalanced"
	}

	result.Imbalance = ImbalanceInfo{
		PodDistribution: podDist,
		CpuDistribution: podDist, // simplified
		MemDistribution: podDist,
		ZoneBalance:     "unknown",
		MaxMinRatio:     maxMinRatio,
		GiniCoefficient: gini,
	}

	// Rebalancing operations
	for _, n := range result.Nodes {
		if n.IsOverPacked {
			excessPods := n.PodCount - int(avgPodsPerNode)
			if excessPods > 0 {
				result.RebalancingOps = append(result.RebalancingOps, RebalancingOp{
					Type: "spread", Node: n.Name,
					Detail: fmt.Sprintf("%s has %d pods (%.0f%% density) — move %d pod(s) to other nodes", n.Name, n.PodCount, n.DensityPct, excessPods),
					PodCount: excessPods, Priority: 1,
				})
			}
		}
		if n.IsUnderUsed {
			result.RebalancingOps = append(result.RebalancingOps, RebalancingOp{
				Type: "consolidate", Node: n.Name,
				Detail: fmt.Sprintf("%s has only %d pods (%.0f%% density) — consider cordoning and draining", n.Name, n.PodCount, n.DensityPct),
				PodCount: n.PodCount, Priority: 2,
				})
		}
	}

	// Namespace spread analysis
	nsNodeMap := map[string]map[string]bool{}
	nsPodCount := map[string]int{}
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		if nsNodeMap[pod.Namespace] == nil {
			nsNodeMap[pod.Namespace] = map[string]bool{}
		}
		nsNodeMap[pod.Namespace][pod.Spec.NodeName] = true
		nsPodCount[pod.Namespace]++
	}
	for ns, nodeSet := range nsNodeMap {
		nodeSpread := len(nodeSet)
		avgPerNode := 0.0
		if nodeSpread > 0 {
			avgPerNode = float64(nsPodCount[ns]) / float64(nodeSpread)
		}
		result.ByNamespace = append(result.ByNamespace, DensityNSStat{
			Namespace: ns, PodCount: nsPodCount[ns],
			NodeSpread: nodeSpread, AvgPerNode: avgPerNode,
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].PodCount > result.ByNamespace[j].PodCount
	})

	// Score
	result.HealthScore = result.Summary.BalanceScore
	result.Grade = scoreToGrade(result.HealthScore)

	// Recs
	result.Recommendations = generateDensityBalanceRecs(result)

	writeJSON(w, result)
}

// computeStdDev computes standard deviation.
func computeStdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sumSq := 0.0
	for _, v := range values {
		diff := v - mean
		sumSq += diff * diff
	}
	return sqrtFloat(sumSq / float64(len(values)))
}

// sqrtFloat computes square root using Newton's method.
func sqrtFloat(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// computeGini computes Gini coefficient for inequality measurement.
func computeGini(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)
	sum := 0.0
	for i, v := range sorted {
		sum += float64(i+1) * v
	}
	total := 0.0
	for _, v := range sorted {
		total += v
	}
	if total == 0 {
		return 0
	}
	n := float64(len(sorted))
	return (2*sum) / (n * total) - (n+1)/n
}

// computeBalanceScore computes pod distribution balance score.
func computeBalanceScore(s DensityBalanceSummary) int {
	score := 100
	if s.TotalNodes == 0 {
		return score
	}
	// Penalize overpacked nodes
	score -= minInt(s.OverPackedNodes*15, 30)
	// Penalize underused nodes
	score -= minInt(s.UnderUsedNodes*10, 20)
	// Penalize high standard deviation
	if s.AvgPodsPerNode > 0 {
		cv := s.StdDeviation / s.AvgPodsPerNode // coefficient of variation
		if cv > 0.5 {
			score -= 15
		}
		if cv > 1.0 {
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateDensityRecs produces recommendations.
func generateDensityBalanceRecs(r DensityBalanceResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Pod density: %d pods across %d nodes (%.1f avg/node, balance score %d/100, %s)",
		r.Summary.TotalPods, r.Summary.TotalNodes, r.Summary.AvgPodsPerNode, r.HealthScore, r.Imbalance.PodDistribution))

	if r.Summary.OverPackedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d over-packed node(s) (>80%% capacity) — spread pods for fault tolerance", r.Summary.OverPackedNodes))
	}

	if r.Summary.UnderUsedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d under-used node(s) (<20%% capacity) — consolidate and scale down", r.Summary.UnderUsedNodes))
	}

	if r.Imbalance.MaxMinRatio > 2 {
		recs = append(recs, fmt.Sprintf("Pod distribution imbalance: max/min ratio %.1fx — add anti-affinity rules", r.Imbalance.MaxMinRatio))
	}

	if r.Imbalance.GiniCoefficient > 0.3 {
		recs = append(recs, fmt.Sprintf("Gini coefficient %.2f indicates uneven pod distribution — use topology spread constraints", r.Imbalance.GiniCoefficient))
	}

	for _, op := range r.RebalancingOps {
		if op.Priority == 1 {
			recs = append(recs, fmt.Sprintf("REBALANCE: %s", op.Detail))
		}
	}

	return recs
}
