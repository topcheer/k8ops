package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapacityPlanResult is the capacity planning & growth trend prediction.
type CapacityPlanResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         CapacityPlanSummary  `json:"summary"`
	ByNode          []CapacityNodeEntry  `json:"byNode"`
	GrowthTrends    []GrowthTrendEntry   `json:"growthTrends"`
	Forecast        CapacityPlanForecast `json:"forecast"`
	Issues          []CapacityPlanIssue  `json:"issues"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// CapacityPlanSummary aggregates capacity planning stats.
type CapacityPlanSummary struct {
	TotalNodes         int     `json:"totalNodes"`
	TotalCPUAlloc      float64 `json:"totalCPUAlloc"`      // cores
	TotalMemAlloc      float64 `json:"totalMemAllocGB"`    // GB
	TotalCPUCapacity   float64 `json:"totalCPUCapacity"`   // cores
	TotalMemCapacity   float64 `json:"totalMemCapacityGB"` // GB
	CPUUtilization     float64 `json:"cpuUtilization"`     // 0-1
	MemUtilization     float64 `json:"memUtilization"`     // 0-1
	PodUtilization     float64 `json:"podUtilization"`     // pods/capacity
	NodesNeedingScale  int     `json:"nodesNeedingScale"`
	HeadroomDays       int     `json:"headroomDays"` // days until capacity exhaustion
	ForecastConfidence string  `json:"forecastConfidence"`
}

// CapacityNodeEntry per-node capacity stats.
type CapacityNodeEntry struct {
	Name        string  `json:"name"`
	CPUAlloc    float64 `json:"cpuAlloc"`      // cores
	CPUCapacity float64 `json:"cpuCapacity"`   // cores
	CPUUtil     float64 `json:"cpuUtil"`       // 0-1
	MemAlloc    float64 `json:"memAllocGB"`    // GB
	MemCapacity float64 `json:"memCapacityGB"` // GB
	MemUtil     float64 `json:"memUtil"`       // 0-1
	PodCount    int     `json:"podCount"`
	PodCapacity int     `json:"podCapacity"`
	PodUtil     float64 `json:"podUtil"` // 0-1
	NeedsScale  bool    `json:"needsScale"`
	RiskLevel   string  `json:"riskLevel"`
}

// GrowthTrendEntry describes resource growth over time.
type GrowthTrendEntry struct {
	Metric        string  `json:"metric"`
	Current       float64 `json:"current"`
	Capacity      float64 `json:"capacity"`
	DailyGrowth   float64 `json:"dailyGrowth"`   // estimated daily growth rate
	DaysToExhaust int     `json:"daysToExhaust"` // days until capacity reached (-1 = no growth)
	Projection    string  `json:"projection"`    // human-readable projection
}

// CapacityPlanForecast predicts when scale-out is needed.
type CapacityPlanForecast struct {
	CPUExhaustDays    int    `json:"cpuExhaustDays"`
	MemExhaustDays    int    `json:"memExhaustDays"`
	PodExhaustDays    int    `json:"podExhaustDays"`
	FirstBottleneck   string `json:"firstBottleneck"`
	RecommendedAction string `json:"recommendedAction"`
}

// CapacityPlanIssue describes a capacity problem.
type CapacityPlanIssue struct {
	Severity   string `json:"severity"`
	Node       string `json:"node,omitempty"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion"`
}

// handleCapacityPlan handles GET /api/scalability/capacity-plan
// Predicts capacity exhaustion timelines and recommends scale-out actions.
func (s *Server) handleCapacityPlan(w http.ResponseWriter, r *http.Request) {
	result := s.auditCapacityPlan()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (s *Server) auditCapacityPlan() *CapacityPlanResult {
	result := &CapacityPlanResult{ScannedAt: time.Now()}

	if s.clientset == nil {
		result.HealthScore = 100
		return result
	}

	nodes, err := s.clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		result.HealthScore = 100
		return result
	}

	pods, err := s.clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		result.HealthScore = 100
		return result
	}

	// Build per-node pod allocation map
	nodePodStats := map[string]*struct {
		cpuAlloc resource.Quantity
		memAlloc resource.Quantity
		podCount int
	}{}
	for _, pod := range pods.Items {
		nodeName := pod.Spec.NodeName
		if nodeName == "" {
			continue // pending pod
		}
		stats, ok := nodePodStats[nodeName]
		if !ok {
			stats = &struct {
				cpuAlloc resource.Quantity
				memAlloc resource.Quantity
				podCount int
			}{}
			nodePodStats[nodeName] = stats
		}
		stats.podCount++
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests.Cpu() != nil {
				stats.cpuAlloc.Add(*c.Resources.Requests.Cpu())
			}
			if c.Resources.Requests.Memory() != nil {
				stats.memAlloc.Add(*c.Resources.Requests.Memory())
			}
		}
	}

	var totalCPUAlloc, totalMemAlloc, totalCPUCap, totalMemCap float64
	var totalPods, totalPodCap int

	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue // skip cordoned nodes
		}

		cpuCap := float64(node.Status.Allocatable.Cpu().MilliValue()) / 1000
		memCap := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		podCap := 110 // default Kubernetes max pods per node

		stats, ok := nodePodStats[node.Name]
		cpuAlloc := 0.0
		memAlloc := 0.0
		podCount := 0
		if ok {
			cpuAlloc = float64(stats.cpuAlloc.MilliValue()) / 1000
			memAlloc = float64(stats.memAlloc.Value()) / (1024 * 1024 * 1024)
			podCount = stats.podCount
		}

		cpuUtil := 0.0
		if cpuCap > 0 {
			cpuUtil = cpuAlloc / cpuCap
		}
		memUtil := 0.0
		if memCap > 0 {
			memUtil = memAlloc / memCap
		}
		podUtil := 0.0
		if podCap > 0 {
			podUtil = float64(podCount) / float64(podCap)
		}

		needsScale := cpuUtil > 0.8 || memUtil > 0.8 || podUtil > 0.85
		riskLevel := "healthy"
		if cpuUtil > 0.85 || memUtil > 0.85 {
			riskLevel = "critical"
		} else if cpuUtil > 0.7 || memUtil > 0.7 {
			riskLevel = "warning"
		}

		entry := CapacityNodeEntry{
			Name:        node.Name,
			CPUAlloc:    roundToCapacity(cpuAlloc, 2),
			CPUCapacity: roundToCapacity(cpuCap, 2),
			CPUUtil:     roundToCapacity(cpuUtil, 3),
			MemAlloc:    roundToCapacity(memAlloc, 2),
			MemCapacity: roundToCapacity(memCap, 2),
			MemUtil:     roundToCapacity(memUtil, 3),
			PodCount:    podCount,
			PodCapacity: podCap,
			PodUtil:     roundToCapacity(podUtil, 3),
			NeedsScale:  needsScale,
			RiskLevel:   riskLevel,
		}
		result.ByNode = append(result.ByNode, entry)

		totalCPUAlloc += cpuAlloc
		totalMemAlloc += memAlloc
		totalCPUCap += cpuCap
		totalMemCap += memCap
		totalPods += podCount
		totalPodCap += podCap

		if needsScale {
			result.Summary.NodesNeedingScale++
		}
	}

	sort.Slice(result.ByNode, func(i, j int) bool {
		return result.ByNode[i].CPUUtil > result.ByNode[j].CPUUtil
	})

	result.Summary.TotalNodes = len(result.ByNode)
	result.Summary.TotalCPUAlloc = roundToCapacity(totalCPUAlloc, 2)
	result.Summary.TotalMemAlloc = roundToCapacity(totalMemAlloc, 2)
	result.Summary.TotalCPUCapacity = roundToCapacity(totalCPUCap, 2)
	result.Summary.TotalMemCapacity = roundToCapacity(totalMemCap, 2)
	if totalCPUCap > 0 {
		result.Summary.CPUUtilization = roundToCapacity(totalCPUAlloc/totalCPUCap, 3)
	}
	if totalMemCap > 0 {
		result.Summary.MemUtilization = roundToCapacity(totalMemAlloc/totalMemCap, 3)
	}
	if totalPodCap > 0 {
		result.Summary.PodUtilization = roundToCapacity(float64(totalPods)/float64(totalPodCap), 3)
	}

	// Growth trends (estimated based on current utilization and cluster size)
	// Assume 2% daily growth rate as baseline (typical for growing clusters)
	dailyGrowthRate := 0.02

	cpuTrend := GrowthTrendEntry{
		Metric:      "CPU",
		Current:     totalCPUAlloc,
		Capacity:    totalCPUCap,
		DailyGrowth: totalCPUAlloc * dailyGrowthRate,
	}
	if cpuTrend.DailyGrowth > 0 {
		remaining := totalCPUCap - totalCPUAlloc
		cpuTrend.DaysToExhaust = int(remaining / cpuTrend.DailyGrowth)
		cpuTrend.Projection = fmt.Sprintf("%.1f cores remaining, ~%d days to exhaust at current growth", remaining, cpuTrend.DaysToExhaust)
	} else {
		cpuTrend.DaysToExhaust = -1
		cpuTrend.Projection = "No growth detected"
	}

	memTrend := GrowthTrendEntry{
		Metric:      "Memory",
		Current:     totalMemAlloc,
		Capacity:    totalMemCap,
		DailyGrowth: totalMemAlloc * dailyGrowthRate,
	}
	if memTrend.DailyGrowth > 0 {
		remaining := totalMemCap - totalMemAlloc
		memTrend.DaysToExhaust = int(remaining / memTrend.DailyGrowth)
		memTrend.Projection = fmt.Sprintf("%.1f GB remaining, ~%d days to exhaust at current growth", remaining, memTrend.DaysToExhaust)
	} else {
		memTrend.DaysToExhaust = -1
	}

	podTrend := GrowthTrendEntry{
		Metric:      "Pods",
		Current:     float64(totalPods),
		Capacity:    float64(totalPodCap),
		DailyGrowth: float64(totalPods) * dailyGrowthRate,
	}
	if podTrend.DailyGrowth > 0 {
		remaining := float64(totalPodCap) - float64(totalPods)
		podTrend.DaysToExhaust = int(remaining / podTrend.DailyGrowth)
		podTrend.Projection = fmt.Sprintf("%d pods remaining, ~%d days to exhaust", int(remaining), podTrend.DaysToExhaust)
	} else {
		podTrend.DaysToExhaust = -1
	}

	result.GrowthTrends = []GrowthTrendEntry{cpuTrend, memTrend, podTrend}

	// Forecast
	result.Forecast.CPUExhaustDays = cpuTrend.DaysToExhaust
	result.Forecast.MemExhaustDays = memTrend.DaysToExhaust
	result.Forecast.PodExhaustDays = podTrend.DaysToExhaust

	// Find first bottleneck
	minDays := -1
	bottleneck := "none"
	if cpuTrend.DaysToExhaust > 0 && (minDays < 0 || cpuTrend.DaysToExhaust < minDays) {
		minDays = cpuTrend.DaysToExhaust
		bottleneck = "CPU"
	}
	if memTrend.DaysToExhaust > 0 && (minDays < 0 || memTrend.DaysToExhaust < minDays) {
		minDays = memTrend.DaysToExhaust
		bottleneck = "Memory"
	}
	if podTrend.DaysToExhaust > 0 && (minDays < 0 || podTrend.DaysToExhaust < minDays) {
		minDays = podTrend.DaysToExhaust
		bottleneck = "Pod slots"
	}
	result.Forecast.FirstBottleneck = bottleneck
	result.Summary.HeadroomDays = minDays

	if minDays > 0 && minDays < 30 {
		result.Forecast.RecommendedAction = fmt.Sprintf("Scale-out needed within %d days — %s will be exhausted first", minDays, bottleneck)
	} else if minDays > 0 && minDays < 90 {
		result.Forecast.RecommendedAction = fmt.Sprintf("Plan scale-out within %d days — %s approaching capacity", minDays, bottleneck)
	} else if minDays > 0 {
		result.Forecast.RecommendedAction = fmt.Sprintf("Capacity sufficient for %d days, monitor growth trends", minDays)
	} else {
		result.Forecast.RecommendedAction = "No immediate capacity concerns"
	}

	// Confidence
	result.Summary.ForecastConfidence = "low" // static estimate, no historical data

	// Generate issues
	for _, node := range result.ByNode {
		if node.CPUUtil > 0.85 {
			result.Issues = append(result.Issues, CapacityPlanIssue{
				Severity:   "critical",
				Node:       node.Name,
				Issue:      fmt.Sprintf("Node %s CPU utilization %.0f%% — near capacity", node.Name, node.CPUUtil*100),
				Suggestion: "Add nodes or redistribute workloads",
			})
		}
		if node.MemUtil > 0.85 {
			result.Issues = append(result.Issues, CapacityPlanIssue{
				Severity:   "critical",
				Node:       node.Name,
				Issue:      fmt.Sprintf("Node %s memory utilization %.0f%% — near capacity", node.Name, node.MemUtil*100),
				Suggestion: "Add memory capacity or reduce pod density",
			})
		}
	}
	if result.Summary.CPUUtilization > 0.8 {
		result.Issues = append(result.Issues, CapacityPlanIssue{
			Severity:   "warning",
			Issue:      fmt.Sprintf("Cluster-wide CPU utilization %.0f%% — approaching capacity limit", result.Summary.CPUUtilization*100),
			Suggestion: "Consider adding nodes or right-sizing workloads",
		})
	}
	if result.Summary.MemUtilization > 0.8 {
		result.Issues = append(result.Issues, CapacityPlanIssue{
			Severity:   "warning",
			Issue:      fmt.Sprintf("Cluster-wide memory utilization %.0f%% — approaching capacity limit", result.Summary.MemUtilization*100),
			Suggestion: "Consider adding nodes or right-sizing workloads",
		})
	}

	// Recommendations
	if result.Summary.NodesNeedingScale > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d node(s) need scaling — CPU or memory utilization >80%%", result.Summary.NodesNeedingScale))
	}
	if minDays > 0 && minDays < 30 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Capacity exhaustion in ~%d days (%s) — initiate scale-out planning now", minDays, bottleneck))
	}
	if result.Summary.PodUtilization > 0.7 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Pod slot utilization %.0f%% — consider increasing maxPods per node or adding nodes", result.Summary.PodUtilization*100))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations, "Cluster capacity is healthy — no immediate action needed")
	}

	// Health score
	score := 100
	score -= result.Summary.NodesNeedingScale * 10
	if result.Summary.CPUUtilization > 0.8 {
		score -= 15
	} else if result.Summary.CPUUtilization > 0.7 {
		score -= 8
	}
	if result.Summary.MemUtilization > 0.8 {
		score -= 15
	} else if result.Summary.MemUtilization > 0.7 {
		score -= 8
	}
	if minDays > 0 && minDays < 30 {
		score -= 20
	} else if minDays > 0 && minDays < 90 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	return result
}

// roundTo rounds a float to n decimal places.
func roundToCapacity(val float64, n int) float64 {
	if n <= 0 {
		return float64(int(val))
	}
	multiplier := 1.0
	for i := 0; i < n; i++ {
		multiplier *= 10
	}
	return float64(int(val*multiplier)) / multiplier
}

// formatCapacityPlanSummary returns a human-readable summary.
func formatCapacityPlanSummary(r *CapacityPlanResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Capacity plan: %d nodes, CPU %.0f%%, Mem %.0f%%, Pods %.0f%%",
		r.Summary.TotalNodes,
		r.Summary.CPUUtilization*100,
		r.Summary.MemUtilization*100,
		r.Summary.PodUtilization*100)
	if r.Summary.HeadroomDays > 0 {
		fmt.Fprintf(&b, " | %d days headroom (%s bottleneck)", r.Summary.HeadroomDays, r.Forecast.FirstBottleneck)
	}
	return b.String()
}

// Ensure imports are used.
var _ corev1.Node
