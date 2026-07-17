package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SchedulerFairnessResult analyzes pod scheduling fairness: whether workloads
// are evenly distributed across nodes, identify scheduling hotspots, and
// detect nodes that are over- or under-utilized relative to their capacity.
type SchedulerFairnessResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         FairnessSummary        `json:"summary"`
	ByNode          []FairnessNodeStat     `json:"byNode"`
	ByPriorityClass []FairnessPriorityStat `json:"byPriorityClass"`
	Imbalances      []FairnessImbalance    `json:"imbalances"`
	FairnessScore   int                    `json:"fairnessScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type FairnessSummary struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalPods      int     `json:"totalPods"`
	AvgPodsPerNode float64 `json:"avgPodsPerNode"`
	MaxPodsOnNode  int     `json:"maxPodsOnNode"`
	MinPodsOnNode  int     `json:"minPodsOnNode"`
	StdDev         float64 `json:"podDistributionStdDev"`
	ImbalanceRatio float64 `json:"imbalanceRatio"`
}

type FairnessNodeStat struct {
	Node      string  `json:"node"`
	PodCount  int     `json:"podCount"`
	CPUUsage  float64 `json:"cpuUsagePct"`
	MemUsage  float64 `json:"memUsagePct"`
	PodUsage  float64 `json:"podUsagePct"`
	Deviation float64 `json:"deviationFromAvg"`
	Status    string  `json:"status"`
}

type FairnessPriorityStat struct {
	PriorityClass string `json:"priorityClass"`
	PodCount      int    `json:"podCount"`
	NodeSpread    int    `json:"nodeSpread"`
}

type FairnessImbalance struct {
	Node     string `json:"node"`
	Type     string `json:"type"` // over-loaded, under-utilized, pod-hoarder
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

// handleSchedulerFairness handles GET /api/scalability/scheduler-fairness
func (s *Server) handleSchedulerFairness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := SchedulerFairnessResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build node stats
	type nodeData struct {
		pods       int
		cpuReq     float64
		memReq     float64
		cpuCap     float64
		memCap     float64
		podCap     int
		priorities map[string]int
	}
	nodeMap := make(map[string]*nodeData)
	var workerNodes []string

	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		nd := &nodeData{
			cpuCap:     n.Status.Allocatable.Cpu().AsApproximateFloat64(),
			memCap:     n.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9,
			priorities: make(map[string]int),
		}
		if v, ok := n.Status.Allocatable["pods"]; ok {
			nd.podCap = int(v.AsApproximateFloat64())
		}
		nodeMap[n.Name] = nd
		workerNodes = append(workerNodes, n.Name)
	}

	result.Summary.TotalNodes = len(workerNodes)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		nd, ok := nodeMap[pod.Spec.NodeName]
		if !ok {
			continue
		}
		nd.pods++
		result.Summary.TotalPods++

		pc := pod.Spec.PriorityClassName
		if pc == "" {
			pc = "default"
		}
		nd.priorities[pc]++

		for _, c := range pod.Spec.Containers {
			if v, ok := c.Resources.Requests["cpu"]; ok {
				nd.cpuReq += v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Requests["memory"]; ok {
				nd.memReq += v.AsApproximateFloat64() / 1e9
			}
		}
	}

	if len(workerNodes) == 0 {
		writeJSON(w, result)
		return
	}

	avg := float64(result.Summary.TotalPods) / float64(len(workerNodes))
	result.Summary.AvgPodsPerNode = avg

	// Calculate stats and deviations
	var nodeStats []FairnessNodeStat
	var imbalances []FairnessImbalance
	maxPods, minPods := 0, 999999
	var deviations []float64

	for _, nodeName := range workerNodes {
		nd := nodeMap[nodeName]
		maxPods = maxInt(maxPods, nd.pods)
		if nd.pods < minPods {
			minPods = nd.pods
		}
		dev := float64(nd.pods) - avg
		deviations = append(deviations, dev*dev)

		cpuPct := 0.0
		if nd.cpuCap > 0 {
			cpuPct = nd.cpuReq / nd.cpuCap * 100
		}
		memPct := 0.0
		if nd.memCap > 0 {
			memPct = nd.memReq / nd.memCap * 100
		}
		podPct := 0.0
		if nd.podCap > 0 {
			podPct = float64(nd.pods) / float64(nd.podCap) * 100
		}

		status := "balanced"
		if dev > avg*0.3 {
			status = "over-loaded"
			sev := "medium"
			if podPct > 80 {
				sev = "high"
			}
			imbalances = append(imbalances, FairnessImbalance{
				Node: nodeName, Type: "over-loaded",
				Detail:   fmt.Sprintf("%.0f%% pod capacity, %d pods (avg %.0f)", podPct, nd.pods, avg),
				Severity: sev,
			})
		} else if dev < -avg*0.3 {
			status = "under-utilized"
			imbalances = append(imbalances, FairnessImbalance{
				Node: nodeName, Type: "under-utilized",
				Detail:   fmt.Sprintf("%d pods (avg %.0f), %.0f%% utilization", nd.pods, avg, podPct),
				Severity: "low",
			})
		}

		nodeStats = append(nodeStats, FairnessNodeStat{
			Node: nodeName, PodCount: nd.pods,
			CPUUsage: cpuPct, MemUsage: memPct,
			PodUsage: podPct, Deviation: dev, Status: status,
		})
	}

	result.Summary.MaxPodsOnNode = maxPods
	result.Summary.MinPodsOnNode = minPods

	// Std deviation
	sumSq := 0.0
	for _, d := range deviations {
		sumSq += d
	}
	result.Summary.StdDev = sqrtFair(sumSq / float64(len(workerNodes)))
	if avg > 0 {
		result.Summary.ImbalanceRatio = result.Summary.StdDev / avg
	}

	// Priority class spread
	priorityMap := make(map[string]map[string]bool) // class -> set of nodes
	for _, nodeName := range workerNodes {
		for pc := range nodeMap[nodeName].priorities {
			if priorityMap[pc] == nil {
				priorityMap[pc] = make(map[string]bool)
			}
			priorityMap[pc][nodeName] = true
		}
	}
	for pc, nodeSet := range priorityMap {
		totalPC := 0
		for _, nd := range nodeMap {
			totalPC += nd.priorities[pc]
		}
		result.ByPriorityClass = append(result.ByPriorityClass, FairnessPriorityStat{
			PriorityClass: pc, PodCount: totalPC, NodeSpread: len(nodeSet),
		})
	}
	sort.Slice(result.ByPriorityClass, func(i, j int) bool {
		return result.ByPriorityClass[i].PodCount > result.ByPriorityClass[j].PodCount
	})

	sort.Slice(nodeStats, func(i, j int) bool {
		return nodeStats[i].PodCount > nodeStats[j].PodCount
	})
	result.ByNode = nodeStats
	result.Imbalances = imbalances

	// Score: lower imbalance = higher score
	score := 100
	if result.Summary.ImbalanceRatio > 0.5 {
		score -= 40
	} else if result.Summary.ImbalanceRatio > 0.3 {
		score -= 20
	} else if result.Summary.ImbalanceRatio > 0.15 {
		score -= 10
	}
	for _, imb := range imbalances {
		if imb.Severity == "high" {
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}
	result.FairnessScore = score

	switch {
	case score >= 80:
		result.Grade = "A"
	case score >= 60:
		result.Grade = "B"
	case score >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildFairnessRecs(&result)
	writeJSON(w, result)
}

func sqrtFair(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 20; i++ {
		if z == 0 {
			return 0
		}
		z = (z + x/z) / 2
	}
	return z
}

func buildFairnessRecs(r *SchedulerFairnessResult) []string {
	recs := []string{
		fmt.Sprintf("调度公平性: %.2f 不平衡率 (%s)", r.Summary.ImbalanceRatio, r.Grade),
	}
	if len(r.Imbalances) > 0 {
		over := 0
		for _, imb := range r.Imbalances {
			if imb.Type == "over-loaded" {
				over++
			}
		}
		if over > 0 {
			recs = append(recs, fmt.Sprintf("%d 个节点负载过高", over))
		}
	}
	if r.Summary.ImbalanceRatio > 0.3 {
		recs = append(recs, "建议添加 podAntiAffinity 或 topologySpreadConstraints 均衡负载")
	}
	return recs
}
