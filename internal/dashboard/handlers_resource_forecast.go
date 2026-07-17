package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceForecastResult projects future resource needs based on historical
// trends. It forecasts when CPU, memory, and pod capacity will be exhausted,
// and recommends when to scale.
type ResourceForecastResult struct {
	ScannedAt            time.Time       `json:"scannedAt"`
	Summary              ForecastSummary `json:"summary"`
	CPUForecast          ForecastMetric  `json:"cpuForecast"`
	MemForecast          ForecastMetric  `json:"memForecast"`
	PodForecast          ForecastMetric  `json:"podForecast"`
	ByNamespace          []ForecastNS    `json:"byNamespace"`
	ScaleRecommendations []ForecastScale `json:"scaleRecommendations"`
	HealthScore          int             `json:"healthScore"`
	Grade                string          `json:"grade"`
	Recommendations      []string        `json:"recommendations"`
}

type ForecastSummary struct {
	CurrentNodes  int     `json:"currentNodes"`
	CPUUsed       float64 `json:"cpuUsed"`
	CPUCapacity   float64 `json:"cpuCapacity"`
	MemUsedGB     float64 `json:"memUsedGB"`
	MemCapacityGB float64 `json:"memCapacityGB"`
	PodsUsed      int     `json:"podsUsed"`
	PodsCapacity  int     `json:"podsCapacity"`
	GrowthRate    float64 `json:"growthRatePerMonth"`
}

type ForecastMetric struct {
	Resource        string  `json:"resource"`
	Used            float64 `json:"used"`
	Capacity        float64 `json:"capacity"`
	UsedPct         float64 `json:"usedPct"`
	MonthlyGrowth   float64 `json:"monthlyGrowthEst"`
	MonthsToExhaust int     `json:"monthsToExhaust"`
	Threshold       string  `json:"threshold"`
}

type ForecastNS struct {
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuReq"`
	MemReq    float64 `json:"memReqGB"`
	Pods      int     `json:"pods"`
	Share     float64 `json:"resourceSharePct"`
}

type ForecastScale struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	When     string `json:"when"`
	Urgency  string `json:"urgency"`
}

// handleResourceForecast handles GET /api/scalability/resource-forecast
func (s *Server) handleResourceForecast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ResourceForecastResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	cpuCap, memCap, podCap := 0.0, 0.0, 0
	workerCount := 0
	for _, n := range nodes.Items {
		if _, ok := n.Labels["node-role.kubernetes.io/control-plane"]; ok {
			continue
		}
		workerCount++
		cpuCap += n.Status.Allocatable.Cpu().AsApproximateFloat64()
		memCap += n.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
		if v, ok := n.Status.Allocatable["pods"]; ok {
			podCap += int(v.AsApproximateFloat64())
		}
	}

	cpuUsed, memUsed := 0.0, 0.0
	podCount := 0
	nsMap := make(map[string]*ForecastNS)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		podCount++
		if _, ok := nsMap[pod.Namespace]; !ok {
			nsMap[pod.Namespace] = &ForecastNS{Namespace: pod.Namespace}
		}
		for _, c := range pod.Spec.Containers {
			cpuReq := 0.0
			memReq := 0.0
			if v, ok := c.Resources.Requests["cpu"]; ok {
				cpuReq = v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Requests["memory"]; ok {
				memReq = v.AsApproximateFloat64() / 1e9
			}
			cpuUsed += cpuReq
			memUsed += memReq
			nsMap[pod.Namespace].CPUReq += cpuReq
			nsMap[pod.Namespace].MemReq += memReq
		}
		nsMap[pod.Namespace].Pods++
	}

	// Estimate growth rate (heuristic: 5% per month)
	growthRate := 0.05

	// CPU forecast
	cpuUsedPct := 0.0
	if cpuCap > 0 {
		cpuUsedPct = cpuUsed / cpuCap * 100
	}
	cpuMonthlyGrowth := cpuUsed * growthRate
	cpuMonths := calcMonthsToExhaust(cpuUsed, cpuCap, cpuMonthlyGrowth)

	// Memory forecast
	memUsedPct := 0.0
	if memCap > 0 {
		memUsedPct = memUsed / memCap * 100
	}
	memMonthlyGrowth := memUsed * growthRate
	memMonths := calcMonthsToExhaust(memUsed, memCap, memMonthlyGrowth)

	// Pod forecast
	podUsedPct := 0.0
	if podCap > 0 {
		podUsedPct = float64(podCount) / float64(podCap) * 100
	}
	podMonthlyGrowth := float64(podCount) * growthRate
	podMonths := calcMonthsToExhaust(float64(podCount), float64(podCap), podMonthlyGrowth)

	result.CPUForecast = ForecastMetric{
		Resource: "CPU", Used: cpuUsed, Capacity: cpuCap,
		UsedPct: cpuUsedPct, MonthlyGrowth: cpuMonthlyGrowth,
		MonthsToExhaust: cpuMonths,
		Threshold:       thresholdFromMonths(cpuMonths),
	}
	result.MemForecast = ForecastMetric{
		Resource: "Memory", Used: memUsed, Capacity: memCap,
		UsedPct: memUsedPct, MonthlyGrowth: memMonthlyGrowth,
		MonthsToExhaust: memMonths,
		Threshold:       thresholdFromMonths(memMonths),
	}
	result.PodForecast = ForecastMetric{
		Resource: "Pods", Used: float64(podCount), Capacity: float64(podCap),
		UsedPct: podUsedPct, MonthlyGrowth: podMonthlyGrowth,
		MonthsToExhaust: podMonths,
		Threshold:       thresholdFromMonths(podMonths),
	}

	// Scale recommendations
	if cpuMonths < 3 {
		result.ScaleRecommendations = append(result.ScaleRecommendations, ForecastScale{
			Resource: "CPU", Action: fmt.Sprintf("Add %.0f CPU cores", cpuMonthlyGrowth*3),
			When: fmt.Sprintf("%d 个月内", cpuMonths), Urgency: "high",
		})
	}
	if memMonths < 3 {
		result.ScaleRecommendations = append(result.ScaleRecommendations, ForecastScale{
			Resource: "Memory", Action: fmt.Sprintf("Add %.0f GB memory", memMonthlyGrowth*3),
			When: fmt.Sprintf("%d 个月内", memMonths), Urgency: "high",
		})
	}
	if podMonths < 3 {
		result.ScaleRecommendations = append(result.ScaleRecommendations, ForecastScale{
			Resource: "Pods", Action: fmt.Sprintf("Add %d pod capacity", int(podMonthlyGrowth)*3),
			When: fmt.Sprintf("%d 个月内", podMonths), Urgency: "high",
		})
	}

	// NS breakdown
	for _, ns := range nsMap {
		if cpuUsed > 0 {
			ns.Share = ns.CPUReq / cpuUsed * 100
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].CPUReq > result.ByNamespace[j].CPUReq
	})

	// Summary
	result.Summary.CurrentNodes = workerCount
	result.Summary.CPUUsed = cpuUsed
	result.Summary.CPUCapacity = cpuCap
	result.Summary.MemUsedGB = memUsed
	result.Summary.MemCapacityGB = memCap
	result.Summary.PodsUsed = podCount
	result.Summary.PodsCapacity = podCap
	result.Summary.GrowthRate = growthRate * 100

	// Score based on time to exhaustion
	minMonths := cpuMonths
	if memMonths < minMonths {
		minMonths = memMonths
	}
	if podMonths < minMonths {
		minMonths = podMonths
	}
	switch {
	case minMonths >= 12:
		result.HealthScore = 90
	case minMonths >= 6:
		result.HealthScore = 70
	case minMonths >= 3:
		result.HealthScore = 50
	default:
		result.HealthScore = 25
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildForecastRecs(&result)
	writeJSON(w, result)
}

func calcMonthsToExhaust(used, capacity, monthlyGrowth float64) int {
	if monthlyGrowth <= 0 || capacity <= 0 || used >= capacity {
		if used >= capacity {
			return 0
		}
		return 99
	}
	remaining := capacity - used
	months := 0
	projected := used
	for projected < capacity && months < 36 {
		projected += monthlyGrowth
		months++
	}
	_ = remaining
	if months >= 36 {
		return 36
	}
	return months
}

func thresholdFromMonths(months int) string {
	switch {
	case months <= 1:
		return "critical"
	case months <= 3:
		return "warning"
	case months <= 6:
		return "notice"
	default:
		return "safe"
	}
}

func buildForecastRecs(r *ResourceForecastResult) []string {
	recs := []string{
		fmt.Sprintf("资源预测: CPU %d个月, 内存 %d个月, Pod %d个月到容量上限",
			r.CPUForecast.MonthsToExhaust, r.MemForecast.MonthsToExhaust, r.PodForecast.MonthsToExhaust),
	}
	for _, sc := range r.ScaleRecommendations {
		recs = append(recs, fmt.Sprintf("[%s] %s (%s)", sc.Urgency, sc.Action, sc.When))
	}
	if len(recs) == 1 {
		recs = append(recs, "当前容量充足，无需紧急扩容")
	}
	return recs
}

var _ corev1.Pod
