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

// ForecastHorizon defines how far ahead we project.
const forecastHorizonDays = 90

// ResourceForecast holds the exhaustion prediction for one resource type.
type ResourceForecast struct {
	Resource       string  `json:"resource"`       // cpu, memory, pods, storage
	TotalCapacity  int64   `json:"totalCapacity"`  // total cluster capacity (milli CPU, bytes, count)
	Allocated      int64   `json:"allocated"`      // sum of requests across all pods
	AllocatedPct   float64 `json:"allocatedPct"`   // allocated / capacity * 100
	Utilized       int64   `json:"utilized"`       // actual usage (if available)
	UtilizedPct    float64 `json:"utilizedPct"`    // utilized / capacity * 100
	DaysToExhaust  int     `json:"daysToExhaust"`  // estimated days until exhausted (0 = already exhausted, -1 = insufficient data)
	ExhaustionDate string  `json:"exhaustionDate"` // ISO date or empty
	RiskLevel      string  `json:"riskLevel"`      // safe, moderate, high, critical
	TrendPerDay    float64 `json:"trendPerDay"`    // estimated daily growth (allocated units/day)
	Recommendation string  `json:"recommendation"`
}

// CapacityForecast is the full forecasting result.
type CapacityForecast struct {
	ScannedAt      time.Time            `json:"scannedAt"`
	NodeCount      int                  `json:"nodeCount"`
	PodCount       int                  `json:"podCount"`
	Forecasts      []ResourceForecast   `json:"forecasts"`
	OverallRisk    string               `json:"overallRisk"`
	ClusterSummary ClusterCapacitySummary `json:"clusterSummary"`
}

// ClusterCapacitySummary is a high-level view of current capacity.
type ClusterCapacitySummary struct {
	TotalCPU     int64   `json:"totalCPU"`     // milli cores
	TotalMemory  int64   `json:"totalMemory"`  // bytes
	TotalPodsCap int64   `json:"totalPodsCap"`
	TotalStorage int64   `json:"totalStorage"` // bytes from PVCs
	UsedCPU      int64   `json:"usedCPU"`
	UsedMemory   int64   `json:"usedMemory"`
	UsedPods     int64   `json:"usedPods"`
}

// handleCapacityForecast predicts when cluster resources will be exhausted.
// GET /api/capacity/forecast
func (s *Server) handleCapacityForecast(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)

	// Get nodes for capacity
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get pods for allocation
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get PVCs for storage capacity
	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// Non-fatal — continue without storage data
		pvcs = nil
	}

	// Get node metrics if available (for actual utilization)
	nodeMetrics, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	forecast := computeForecast(nodes.Items, pods.Items, pvcs.Items, nodeMetrics.Items)
	writeJSON(w, forecast)
}

// computeForecast analyzes cluster capacity vs allocation and projects exhaustion.
func computeForecast(nodes []corev1.Node, pods []corev1.Pod, pvcs []corev1.PersistentVolumeClaim, _ []corev1.Node) CapacityForecast {
	result := CapacityForecast{
		ScannedAt: time.Now(),
		NodeCount: len(nodes),
		PodCount:  len(pods),
	}

	// Aggregate cluster capacity
	var totalCPU, totalMem, totalPodsCap int64
	for _, n := range nodes {
		totalCPU += n.Status.Allocatable.Cpu().MilliValue()
		totalMem += n.Status.Allocatable.Memory().Value()
		totalPodsCap += n.Status.Allocatable.Pods().Value()
	}

	// Aggregate allocation (requests)
	var allocCPU, allocMem int64
	var runningPods int64
	podCreationTimes := make([]time.Time, 0)

	for _, p := range pods {
		if p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodPending {
			runningPods++
		}
		podCreationTimes = append(podCreationTimes, p.CreationTimestamp.Time)

		for _, c := range p.Spec.Containers {
			if c.Resources.Requests.Cpu() != nil {
				allocCPU += c.Resources.Requests.Cpu().MilliValue()
			}
			if c.Resources.Requests.Memory() != nil {
				allocMem += c.Resources.Requests.Memory().Value()
			}
		}
	}

	// Aggregate PVC storage
	var pvcStorage int64
	for _, pvc := range pvcs {
		if pvc.Spec.Resources.Requests.Storage() != nil {
			pvcStorage += pvc.Spec.Resources.Requests.Storage().Value()
		}
	}

	result.ClusterSummary = ClusterCapacitySummary{
		TotalCPU:     totalCPU,
		TotalMemory:  totalMem,
		TotalPodsCap: totalPodsCap,
		TotalStorage: pvcStorage,
		UsedCPU:      allocCPU,
		UsedMemory:   allocMem,
		UsedPods:     runningPods,
	}

	// Estimate growth rate from pod creation timestamps
	growthRate := estimateGrowthRate(podCreationTimes)

	// CPU forecast
	cpuForecast := buildResourceForecast("cpu", allocCPU, totalCPU, totalCPU, growthRate.CPUPerDay)
	result.Forecasts = append(result.Forecasts, cpuForecast)

	// Memory forecast
	memForecast := buildResourceForecast("memory", allocMem, totalMem, totalMem, growthRate.MemPerDay)
	result.Forecasts = append(result.Forecasts, memForecast)

	// Pod capacity forecast
	podForecast := buildResourceForecast("pods", runningPods, totalPodsCap, totalPodsCap, growthRate.PodsPerDay)
	result.Forecasts = append(result.Forecasts, podForecast)

	// Storage forecast (if data available)
	if pvcStorage > 0 {
		// Estimate storage growth from PVC count * avg size growth
		storageGrowth := growthRate.StoragePerDay
		storageForecast := buildResourceForecast("storage", pvcStorage, pvcStorage*3, pvcStorage*3, storageGrowth)
		// Storage capacity is harder to determine — use PVC requested as "used" and estimate 3x as "capacity"
		storageForecast.Recommendation = "Storage capacity depends on underlying provisioner. Monitor PVC growth closely."
		result.Forecasts = append(result.Forecasts, storageForecast)
	}

	// Overall risk
	riskScores := map[string]int{"safe": 0, "moderate": 1, "high": 2, "critical": 3}
	worstRisk := 0
	for _, f := range result.Forecasts {
		if score, ok := riskScores[f.RiskLevel]; ok && score > worstRisk {
			worstRisk = score
		}
	}
	riskNames := []string{"safe", "moderate", "high", "critical"}
	if worstRisk < len(riskNames) {
		result.OverallRisk = riskNames[worstRisk]
	}

	return result
}

// growthRateEstimate holds estimated daily growth for each resource.
type growthRateEstimate struct {
	CPUPerDay      float64 // milli cores per day
	MemPerDay      float64 // bytes per day
	PodsPerDay     float64 // pods per day
	StoragePerDay  float64 // bytes per day
}

// estimateGrowthRate uses pod creation timestamps to estimate the rate of
// resource consumption growth. Falls back to a conservative linear model
// if there's insufficient historical data.
func estimateGrowthRate(creationTimes []time.Time) growthRateEstimate {
	if len(creationTimes) < 10 {
		// Not enough data — return conservative defaults based on small growth
		return growthRateEstimate{
			PodsPerDay:    0.5, // assume 0.5 new pods/day
			StoragePerDay: 100 * 1024 * 1024, // 100 MB/day
		}
	}

	now := time.Now()
	// Group pods by creation day
	dayBuckets := make(map[string]int)
	for _, t := range creationTimes {
		dayKey := t.Format("2006-01-02")
		dayBuckets[dayKey]++
	}

	// Calculate average pods per day over the cluster's lifetime
	earliest := creationTimes[0]
	for _, t := range creationTimes {
		if t.Before(earliest) {
			earliest = t
		}
	}

	daysActive := now.Sub(earliest).Hours() / 24
	if daysActive < 1 {
		daysActive = 1
	}

	podsPerDay := float64(len(creationTimes)) / daysActive

	// Estimate CPU and memory growth proportional to pod growth
	// Assume average pod requests: 500m CPU, 512Mi memory
	avgCPUPerPod := 500.0   // milli cores
	avgMemPerPod := 512.0 * 1024 * 1024 // bytes

	return growthRateEstimate{
		CPUPerDay:     podsPerDay * avgCPUPerPod,
		MemPerDay:     podsPerDay * avgMemPerPod,
		PodsPerDay:    podsPerDay,
		StoragePerDay: podsPerDay * 5 * 1024 * 1024 * 1024, // ~5GB per new pod (rough estimate)
	}
}

// buildResourceForecast creates a single resource's forecast.
func buildResourceForecast(resource string, used, capacity, totalCapacity int64, growthPerDay float64) ResourceForecast {
	pct := 0.0
	if capacity > 0 {
		pct = float64(used) / float64(capacity) * 100
	}

	f := ResourceForecast{
		Resource:       resource,
		TotalCapacity:  totalCapacity,
		Allocated:      used,
		AllocatedPct:   roundTo2(pct),
		TrendPerDay:    roundTo2(growthPerDay),
	}

	// Determine risk level based on current utilization
	switch {
	case pct >= 95:
		f.RiskLevel = "critical"
	case pct >= 80:
		f.RiskLevel = "high"
	case pct >= 60:
		f.RiskLevel = "moderate"
	default:
		f.RiskLevel = "safe"
	}

	// Calculate days to exhaustion
	if used >= capacity {
		f.DaysToExhaust = 0
		f.ExhaustionDate = time.Now().Format("2006-01-02")
		f.RiskLevel = "critical"
	} else if growthPerDay > 0 {
		remaining := float64(capacity - used)
		days := remaining / growthPerDay
		f.DaysToExhaust = int(days)

		if days <= 30 {
			f.ExhaustionDate = time.Now().AddDate(0, 0, int(days)).Format("2006-01-02")
			if f.RiskLevel == "safe" || f.RiskLevel == "moderate" {
				f.RiskLevel = "high"
			}
		}

		// Escalate risk based on days to exhaustion
		if days <= 7 {
			f.RiskLevel = "critical"
		} else if days <= 30 {
			if f.RiskLevel != "critical" {
				f.RiskLevel = "high"
			}
		}
	} else {
		// No growth detected
		f.DaysToExhaust = -1
	}

	f.Recommendation = generateRecommendation(resource, f.RiskLevel, pct, f.DaysToExhaust)

	return f
}

// generateRecommendation produces actionable advice for each resource and risk level.
func generateRecommendation(resource, riskLevel string, pct float64, daysToExhaust int) string {
	resourceLabel := map[string]string{
		"cpu":     "CPU",
		"memory":  "内存",
		"pods":    "Pod 容量",
		"storage": "存储",
	}
	label := resourceLabel[resource]
	if label == "" {
		label = resource
	}

	var sb strings.Builder

	switch riskLevel {
	case "critical":
		if daysToExhaust == 0 {
			sb.WriteString(fmt.Sprintf("⚠ %s 已达容量上限 (%.1f%%)，", label, pct))
			sb.WriteString("需要立即扩容。")
		} else if daysToExhaust > 0 && daysToExhaust <= 7 {
			sb.WriteString(fmt.Sprintf("⚠ %s 将在 %d 天内耗尽 (%.1f%%)，", label, daysToExhaust, pct))
		} else {
			sb.WriteString(fmt.Sprintf("⚠ %s 使用率极高 (%.1f%%)，", label, pct))
		}

		switch resource {
		case "cpu", "memory":
			sb.WriteString("建议：1) 添加工作节点 2) 优化高消耗 Pod 的资源请求 3) 检查是否有异常工作负载")
		case "pods":
			sb.WriteString("建议：1) 添加节点增加 Pod 上限 2) 清理已完成/失败的 Pod 3) 检查是否有 Pod 碎片化")
		case "storage":
			sb.WriteString("建议：1) 扩展存储卷 2) 清理未使用的 PVC 和镜像 3) 配置存储自动扩容")
		}

	case "high":
		sb.WriteString(fmt.Sprintf(" %s 使用率较高 (%.1f%%)", label, pct))
		if daysToExhaust > 0 {
			sb.WriteString(fmt.Sprintf("，预计 %d 天后耗尽", daysToExhaust))
		}
		sb.WriteString("。建议提前规划扩容。")

	case "moderate":
		sb.WriteString(fmt.Sprintf(" %s 使用率中等 (%.1f%%)，建议定期监控。", label, pct))

	case "safe":
		sb.WriteString(fmt.Sprintf(" %s 使用率健康 (%.1f%%)，无需操作。", label, pct))
	}

	return sb.String()
}

// roundTo2 rounds a float to 2 decimal places.
func roundTo2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// SortedForecasts returns forecasts sorted by risk priority (unused but available for callers).
func SortedForecasts(forecasts []ResourceForecast) []ResourceForecast {
	result := make([]ResourceForecast, len(forecasts))
	copy(result, forecasts)

	sort.Slice(result, func(i, j int) bool {
		return forecastRiskScore(result[i]) < forecastRiskScore(result[j])
	})

	return result
}

func forecastRiskScore(f ResourceForecast) int {
	switch f.RiskLevel {
	case "critical":
		return 0
	case "high":
		return 1
	case "moderate":
		return 2
	default:
		return 3
	}
}
