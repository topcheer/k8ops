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

// PodDensityResult is the pod density & scheduling capacity analysis.
type PodDensityResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         DensitySummary     `json:"summary"`
	NodeAnalysis    []DensityNodeEntry `json:"nodeAnalysis"`
	BinPacking      BinPackingAnalysis `json:"binPacking"`
	AtCapacityNodes []string           `json:"atCapacityNodes"`
	Fragments       []FragmentEntry    `json:"fragments"`
	Recommendations []string           `json:"recommendations"`
}

// DensitySummary aggregates cluster scheduling capacity.
type DensitySummary struct {
	TotalNodes        int     `json:"totalNodes"`
	SchedulableNodes  int     `json:"schedulableNodes"`
	TotalPods         int     `json:"totalPods"`
	ScheduledPods     int     `json:"scheduledPods"`
	AvgPodsPerNode    float64 `json:"avgPodsPerNode"`
	MaxPodsPerNode    int     `json:"maxPodsPerNode"`    // configured limit
	TotalHeadroomPods int     `json:"totalHeadroomPods"` // how many more pods can be scheduled
	CPUHeadroomCores  float64 `json:"cpuHeadroomCores"`
	MemHeadroomGB     float64 `json:"memHeadroomGB"`
	NodesNearFull     int     `json:"nodesNearFull"` // >85% pod capacity
	NodesFull         int     `json:"nodesFull"`     // at max pods
	NodesCordoned     int     `json:"nodesCordoned"`
	BinPackingScore   int     `json:"binPackingScore"` // 0-100
	HealthScore       int     `json:"healthScore"`     // 0-100
}

// DensityNodeEntry describes one node's pod density.
type DensityNodeEntry struct {
	Name           string  `json:"name"`
	IsSchedulable  bool    `json:"isSchedulable"`
	PodCount       int     `json:"podCount"`
	MaxPods        int     `json:"maxPods"`
	PodCapacityPct float64 `json:"podCapacityPct"`
	CPUReqCores    float64 `json:"cpuReqCores"`
	CPUCapCores    float64 `json:"cpuCapCores"`
	CPUUsagePct    float64 `json:"cpuUsagePct"`
	MemReqGB       float64 `json:"memReqGB"`
	MemCapGB       float64 `json:"memCapGB"`
	MemUsagePct    float64 `json:"memUsagePct"`
	PodHeadroom    int     `json:"podHeadroom"`
	RiskLevel      string  `json:"riskLevel"`
}

// BinPackingAnalysis evaluates workload distribution efficiency.
type BinPackingAnalysis struct {
	Score          int     `json:"score"`          // 0-100
	Strategy       string  `json:"strategy"`       // spread / packed / random
	CPUStdDev      float64 `json:"cpuStdDev"`      // standard deviation of CPU usage across nodes
	MemStdDev      float64 `json:"memStdDev"`      // standard deviation of memory usage
	PodStdDev      float64 `json:"podStdDev"`      // standard deviation of pod count
	ImbalanceScore float64 `json:"imbalanceScore"` // 0-100 (higher = worse)
	Description    string  `json:"description"`
}

// FragmentEntry describes a resource fragmentation instance.
type FragmentEntry struct {
	Node     string  `json:"node"`
	CPUFree  float64 `json:"cpuFreeCores"`
	MemFree  float64 `json:"memFreeGB"`
	PodsFree int     `json:"podsFree"`
	Cause    string  `json:"cause"`
}

// handlePodDensity analyzes pod density and scheduling capacity.
// GET /api/scalability/pod-density
func (s *Server) handlePodDensity(w http.ResponseWriter, r *http.Request) {
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

	// Build per-node pod counts and resource requests
	type nodeUsage struct {
		podCount int
		cpuReq   float64 // millicores
		memReq   float64 // bytes
	}
	nodeUsageMap := make(map[string]*nodeUsage)

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" || pod.Status.Phase == corev1.PodSucceeded {
			continue
		}
		u := nodeUsageMap[pod.Spec.NodeName]
		if u == nil {
			u = &nodeUsage{}
			nodeUsageMap[pod.Spec.NodeName] = u
		}
		u.podCount++

		for _, c := range pod.Spec.Containers {
			if r := c.Resources.Requests.Cpu(); r != nil {
				u.cpuReq += float64(r.MilliValue())
			}
			if r := c.Resources.Requests.Memory(); r != nil {
				u.memReq += float64(r.Value())
			}
		}
	}

	result := PodDensityResult{ScannedAt: time.Now()}
	result.Summary.TotalNodes = len(nodes.Items)
	result.Summary.TotalPods = len(pods.Items)

	var allCPUUsage, allMemUsage, allPodCounts []float64

	for _, node := range nodes.Items {
		entry := DensityNodeEntry{
			Name:          node.Name,
			IsSchedulable: !node.Spec.Unschedulable,
		}

		if !entry.IsSchedulable {
			result.Summary.NodesCordoned++
		} else {
			result.Summary.SchedulableNodes++
		}

		// Max pods: default 110, but can be configured
		maxPods := 110
		if pi, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			maxPods = int(pi.Value())
		}
		entry.MaxPods = maxPods

		// CPU capacity
		if cpu := node.Status.Allocatable[corev1.ResourceCPU]; !cpu.IsZero() {
			entry.CPUCapCores = float64(cpu.MilliValue()) / 1000
		}

		// Memory capacity
		if mem := node.Status.Allocatable[corev1.ResourceMemory]; !mem.IsZero() {
			entry.MemCapGB = float64(mem.Value()) / (1024 * 1024 * 1024)
		}

		// Usage from pods
		u := nodeUsageMap[node.Name]
		if u != nil {
			entry.PodCount = u.podCount
			entry.CPUReqCores = u.cpuReq / 1000
			entry.MemReqGB = u.memReq / (1024 * 1024 * 1024)
		}

		// Percentages
		if maxPods > 0 {
			entry.PodCapacityPct = float64(entry.PodCount) / float64(maxPods) * 100
		}
		if entry.CPUCapCores > 0 {
			entry.CPUUsagePct = entry.CPUReqCores / entry.CPUCapCores * 100
		}
		if entry.MemCapGB > 0 {
			entry.MemUsagePct = entry.MemReqGB / entry.MemCapGB * 100
		}

		// Headroom
		entry.PodHeadroom = maxPods - entry.PodCount
		result.Summary.TotalHeadroomPods += entry.PodHeadroom
		result.Summary.CPUHeadroomCores += entry.CPUCapCores - entry.CPUReqCores
		result.Summary.MemHeadroomGB += entry.MemCapGB - entry.MemReqGB

		// Risk
		entry.RiskLevel = assessDensityRisk(entry)

		// Near full / full
		if entry.PodCapacityPct >= 100 {
			result.Summary.NodesFull++
			result.AtCapacityNodes = append(result.AtCapacityNodes, node.Name)
		} else if entry.PodCapacityPct >= 85 {
			result.Summary.NodesNearFull++
		}

		// Fragmentation: enough pods free but low CPU/mem
		if entry.PodHeadroom > 10 && entry.CPUUsagePct > 80 {
			result.Fragments = append(result.Fragments, FragmentEntry{
				Node:     node.Name,
				CPUFree:  entry.CPUCapCores - entry.CPUReqCores,
				MemFree:  entry.MemCapGB - entry.MemReqGB,
				PodsFree: entry.PodHeadroom,
				Cause:    "CPU exhausted despite available pod slots — cannot schedule CPU-heavy pods",
			})
		}
		if entry.PodHeadroom > 10 && entry.MemUsagePct > 80 {
			result.Fragments = append(result.Fragments, FragmentEntry{
				Node:     node.Name,
				CPUFree:  entry.CPUCapCores - entry.CPUReqCores,
				MemFree:  entry.MemCapGB - entry.MemReqGB,
				PodsFree: entry.PodHeadroom,
				Cause:    "Memory exhausted despite available pod slots — cannot schedule memory-heavy pods",
			})
		}

		// Collect for bin-packing analysis
		allCPUUsage = append(allCPUUsage, entry.CPUUsagePct)
		allMemUsage = append(allMemUsage, entry.MemUsagePct)
		allPodCounts = append(allPodCounts, float64(entry.PodCount))

		result.NodeAnalysis = append(result.NodeAnalysis, entry)
	}

	// Sort nodes by risk
	sort.Slice(result.NodeAnalysis, func(i, j int) bool {
		return densityRiskRank(result.NodeAnalysis[i].RiskLevel) < densityRiskRank(result.NodeAnalysis[j].RiskLevel)
	})

	// Average pods per node
	if result.Summary.SchedulableNodes > 0 {
		totalScheduled := 0
		for _, u := range nodeUsageMap {
			totalScheduled += u.podCount
		}
		result.Summary.AvgPodsPerNode = float64(totalScheduled) / float64(result.Summary.SchedulableNodes)
		result.Summary.MaxPodsPerNode = int(result.Summary.AvgPodsPerNode) // rough estimate
	}

	// Bin-packing analysis
	result.BinPacking = analyzeBinPacking(allCPUUsage, allMemUsage, allPodCounts)
	result.Summary.BinPackingScore = result.BinPacking.Score

	// Health score
	result.Summary.HealthScore = calculateDensityScore(result.Summary)
	result.Recommendations = generateDensityRecs(result.Summary, result.BinPacking, result.AtCapacityNodes, result.Fragments)

	writeJSON(w, result)
}

// assessDensityRisk determines risk level for a node.
func assessDensityRisk(entry DensityNodeEntry) string {
	if !entry.IsSchedulable {
		return "low" // cordoned nodes are not a risk
	}
	if entry.PodCapacityPct >= 100 {
		return "critical"
	}
	if entry.PodCapacityPct >= 85 || entry.CPUUsagePct >= 90 || entry.MemUsagePct >= 90 {
		return "high"
	}
	if entry.PodCapacityPct >= 70 || entry.CPUUsagePct >= 75 || entry.MemUsagePct >= 75 {
		return "medium"
	}
	return "low"
}

// analyzeBinPacking evaluates workload distribution.
func analyzeBinPacking(cpuUsage, memUsage, podCounts []float64) BinPackingAnalysis {
	if len(cpuUsage) == 0 {
		return BinPackingAnalysis{Score: 100, Strategy: "unknown", Description: "No nodes to analyze"}
	}

	cpuSD := stdDev(cpuUsage)
	memSD := stdDev(memUsage)
	podSD := stdDev(podCounts)

	avgSD := (cpuSD + memSD) / 2
	imbalance := avgSD // 0 = perfect balance, higher = worse

	// Score: lower imbalance = higher score
	score := 100 - int(imbalance)
	if score < 0 {
		score = 0
	}

	strategy := "spread"
	if imbalance > 25 {
		strategy = "uneven"
	} else if imbalance > 15 {
		strategy = "moderate"
	}

	desc := fmt.Sprintf("CPU std dev: %.1f%%, Memory std dev: %.1f%%, Pod count std dev: %.1f", cpuSD, memSD, podSD)
	if imbalance > 25 {
		desc += " — workloads are unevenly distributed, consider podAntiAffinity or topologySpreadConstraints"
	}

	return BinPackingAnalysis{
		Score:          score,
		Strategy:       strategy,
		CPUStdDev:      cpuSD,
		MemStdDev:      memSD,
		PodStdDev:      podSD,
		ImbalanceScore: imbalance,
		Description:    desc,
	}
}

// stdDev computes standard deviation.
func stdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	var sqDiff float64
	for _, v := range values {
		diff := v - mean
		sqDiff += diff * diff
	}
	return sqrt(sqDiff / float64(len(values)))
}

// sqrt computes square root using Newton's method.
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}

// calculateDensityScore computes 0-100.
func calculateDensityScore(s DensitySummary) int {
	if s.SchedulableNodes == 0 {
		return 0
	}
	score := 100
	score -= s.NodesFull * 15
	score -= s.NodesNearFull * 5
	if s.TotalHeadroomPods < 50 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateDensityRecs produces actionable advice.
func generateDensityRecs(s DensitySummary, bp BinPackingAnalysis, atCapacity []string, fragments []FragmentEntry) []string {
	var recs []string

	if s.NodesFull > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are at max pod capacity — add nodes or increase --max-pods", s.NodesFull))
	}
	if s.NodesNearFull > 0 {
		recs = append(recs, fmt.Sprintf("%d node(s) are near full (>85%% pod capacity) — plan node expansion", s.NodesNearFull))
	}
	if s.TotalHeadroomPods < 100 {
		recs = append(recs, fmt.Sprintf("Only %d pod slots remaining cluster-wide — consider adding nodes soon", s.TotalHeadroomPods))
	}
	if s.CPUHeadroomCores < 4 {
		recs = append(recs, fmt.Sprintf("CPU headroom is %.1f cores — dangerously low for failover scenarios", s.CPUHeadroomCores))
	}
	if s.MemHeadroomGB < 8 {
		recs = append(recs, fmt.Sprintf("Memory headroom is %.1f GB — dangerously low for failover scenarios", s.MemHeadroomGB))
	}
	if len(fragments) > 0 {
		recs = append(recs, fmt.Sprintf("%d resource fragmentation instance(s) detected — pods slots available but blocked by CPU/memory limits", len(fragments)))
	}
	if bp.ImbalanceScore > 25 {
		recs = append(recs, fmt.Sprintf("Bin-packing imbalance score %.0f — add podAntiAffinity or topologySpreadConstraints for better distribution", bp.ImbalanceScore))
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Scheduling health score is %d/100 — cluster approaching capacity limits", s.HealthScore))
	}

	return recs
}

func densityRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

// Ensure strings import is used
var _ = strings.Contains
