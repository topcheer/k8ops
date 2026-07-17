package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CapForecastSummary is a lightweight capacity summary used by test helpers
// and legacy forecast representations.
type CapForecastSummary struct {
	NodeCount int     `json:"nodeCount"`
	CPUPct    float64 `json:"cpuPct"`
	MemPct    float64 `json:"memPct"`
}

// CapForecast represents a single resource forecast entry.
type CapForecast struct {
	Resource string  `json:"resource"`
	UsagePct float64 `json:"usagePct"`
	Severity string  `json:"severity"`
}

// CapacityForecastResult projects cluster resource consumption into the
// future using deployment creation timestamps as growth indicators.
// It calculates utilization rates, growth rates, and time-to-exhaustion
// for CPU, memory, and storage resources.
type CapacityForecastResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Current         CapacityCurrent    `json:"current"`
	Forecast        CapForecastData    `json:"forecast"`
	ByNamespace     []CapacityNS       `json:"byNamespace"`
	TopConsumers    []CapacityConsumer `json:"topConsumers"`
	GrowthTrend     []GrowthPoint      `json:"growthTrend"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type CapacityCurrent struct {
	NodeCount      int     `json:"nodeCount"`
	TotalCPU       float64 `json:"totalCPU"`          // cores
	TotalMemory    float64 `json:"totalMemory"`       // GB
	TotalStorage   float64 `json:"totalStorage"`      // GB (ephemeral)
	AllocatableCPU float64 `json:"allocatableCPU"`    // cores
	AllocatableMem float64 `json:"allocatableMemory"` // GB
	RequestedCPU   float64 `json:"requestedCPU"`      // cores
	RequestedMem   float64 `json:"requestedMemory"`   // GB
	LimitCPU       float64 `json:"limitCPU"`          // cores
	LimitMem       float64 `json:"limitMemory"`       // GB
	CPUUtilization float64 `json:"cpuUtilization"`    // 0-1
	MemUtilization float64 `json:"memUtilization"`    // 0-1
	PodCount       int     `json:"podCount"`
	PodCapacity    int     `json:"podCapacity"`
	PodUtilization float64 `json:"podUtilization"` // 0-1
}

type CapForecastData struct {
	GrowthRate30d    CapacityGrowth     `json:"growthRate30d"`
	Projection90d    CapacityProjection `json:"projection90d"`
	Projection180d   CapacityProjection `json:"projection180d"`
	TimeToExhaustion CapacityTTE        `json:"timeToExhaustion"`
}

type CapacityGrowth struct {
	CPUPercentPerMonth float64 `json:"cpuPercentPerMonth"`
	MemPercentPerMonth float64 `json:"memPercentPerMonth"`
	PodPercentPerMonth float64 `json:"podPercentPerMonth"`
	NewPodsPerWeek     float64 `json:"newPodsPerWeek"`
	DataPoints         int     `json:"dataPoints"`
}

type CapacityProjection struct {
	HorizonDays   int     `json:"horizonDays"`
	ProjectedCPU  float64 `json:"projectedCPU"`
	ProjectedMem  float64 `json:"projectedMemory"`
	ProjectedPods int     `json:"projectedPods"`
	CPUHeadroom   float64 `json:"cpuHeadroomPct"` // remaining capacity %
	MemHeadroom   float64 `json:"memHeadroomPct"`
	PodHeadroom   float64 `json:"podHeadroomPct"`
	Status        string  `json:"status"` // ok/warning/critical
}

type CapacityTTE struct {
	CPUExhaustionDays int    `json:"cpuExhaustionDays"` // 0 = already exhausted or no growth
	MemExhaustionDays int    `json:"memExhaustionDays"`
	PodExhaustionDays int    `json:"podExhaustionDays"`
	FirstBottleneck   string `json:"firstBottleneck"`
}

type CapacityNS struct {
	Namespace  string  `json:"namespace"`
	PodCount   int     `json:"podCount"`
	ReqCPU     float64 `json:"reqCPU"`
	ReqMem     float64 `json:"reqMem"`
	CPUPercent float64 `json:"cpuPercent"`
	MemPercent float64 `json:"memPercent"`
	GrowthRate float64 `json:"growthRate"` // pods per week
}

type CapacityConsumer struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Kind      string  `json:"kind"`
	ReqCPU    float64 `json:"reqCPU"`
	ReqMem    float64 `json:"reqMem"`
	Pods      int     `json:"pods"`
}

type GrowthPoint struct {
	Date     string `json:"date"`
	PodCount int    `json:"podCount"`
}

// handleCapacityForecast handles GET /api/scalability/capacity-forecast
func (s *Server) handleCapacityForecastDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CapacityForecastResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Calculate current capacity
	current := CapacityCurrent{NodeCount: len(nodes.Items)}

	for _, node := range nodes.Items {
		cpuAlloc := node.Status.Allocatable.Cpu()
		memAlloc := node.Status.Allocatable.Memory()
		current.AllocatableCPU += cpuAlloc.AsApproximateFloat64()
		current.AllocatableMem += memAlloc.AsApproximateFloat64() / 1e9 // bytes to GB

		totalCPU := node.Status.Capacity.Cpu()
		totalMem := node.Status.Capacity.Memory()
		current.TotalCPU += totalCPU.AsApproximateFloat64()
		current.TotalMemory += totalMem.AsApproximateFloat64() / 1e9

		// Ephemeral storage
		storage := node.Status.Allocatable.StorageEphemeral()
		current.TotalStorage += storage.AsApproximateFloat64() / 1e9

		// Pod capacity
		if podAlloc, ok := node.Status.Allocatable[corev1.ResourcePods]; ok {
			current.PodCapacity += int(podAlloc.Value())
		}
	}

	// Calculate requests and limits from pods
	nsMap := make(map[string]*CapacityNS)
	var consumers []CapacityConsumer

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		current.PodCount++

		ns := pod.Namespace
		if _, ok := nsMap[ns]; !ok {
			nsMap[ns] = &CapacityNS{Namespace: ns}
		}

		var podReqCPU, podReqMem float64
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				v := req.AsApproximateFloat64()
				current.RequestedCPU += v
				podReqCPU += v
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				v := req.AsApproximateFloat64() / 1e9
				current.RequestedMem += v
				podReqMem += v
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				current.LimitCPU += lim.AsApproximateFloat64()
			}
			if lim, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				current.LimitMem += lim.AsApproximateFloat64() / 1e9
			}
		}

		nsMap[ns].PodCount++
		nsMap[ns].ReqCPU += podReqCPU
		nsMap[ns].ReqMem += podReqMem
	}

	// Utilization rates
	if current.AllocatableCPU > 0 {
		current.CPUUtilization = current.RequestedCPU / current.AllocatableCPU
	}
	if current.AllocatableMem > 0 {
		current.MemUtilization = current.RequestedMem / current.AllocatableMem
	}
	if current.PodCapacity > 0 {
		current.PodUtilization = float64(current.PodCount) / float64(current.PodCapacity)
	}
	result.Current = current

	// Growth rate analysis from deployment creation timestamps
	now := time.Now()
	var pods30d, pods90d, pods180d int
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		age := now.Sub(d.CreationTimestamp.Time)
		replicas := int(ptrInt32(d.Spec.Replicas))
		if age < 30*24*time.Hour {
			pods30d += replicas
		}
		if age < 90*24*time.Hour {
			pods90d += replicas
		}
		if age < 180*24*time.Hour {
			pods180d += replicas
		}
	}

	// Estimate growth rate
	// If 30d pods represent X% of total, monthly growth rate is roughly X/30 per day
	totalDeployPods := 0
	for _, d := range deployments.Items {
		if !isSystemNamespace(d.Namespace) {
			totalDeployPods += int(ptrInt32(d.Spec.Replicas))
		}
	}

	growth := CapacityGrowth{}
	if totalDeployPods > 0 && pods30d > 0 {
		dailyPodRate := float64(pods30d) / 30.0
		growth.NewPodsPerWeek = dailyPodRate * 7
		growth.PodPercentPerMonth = (dailyPodRate * 30 / float64(totalDeployPods)) * 100
	}
	// Estimate CPU and memory growth proportional to pod growth
	if current.RequestedCPU > 0 && growth.PodPercentPerMonth > 0 {
		growth.CPUPercentPerMonth = growth.PodPercentPerMonth
	}
	if current.RequestedMem > 0 && growth.PodPercentPerMonth > 0 {
		growth.MemPercentPerMonth = growth.PodPercentPerMonth
	}
	growth.DataPoints = len(deployments.Items)
	result.Forecast.GrowthRate30d = growth

	// Projections
	project := func(currentVal, allocatableVal float64, growthPctPerMonth float64, horizonDays int) CapacityProjection {
		p := CapacityProjection{HorizonDays: horizonDays}
		months := float64(horizonDays) / 30.0
		growthFactor := 1.0 + (growthPctPerMonth/100.0)*months
		p.ProjectedCPU = current.RequestedCPU * growthFactor
		p.ProjectedMem = current.RequestedMem * growthFactor
		p.ProjectedPods = int(float64(current.PodCount) * growthFactor)

		if allocatableVal > 0 {
			headroom := (1.0 - (currentVal*growthFactor)/allocatableVal) * 100
			if headroom < 0 {
				headroom = 0
			}
			p.CPUHeadroom = headroom
		}

		if headroom := (1.0 - p.ProjectedCPU/current.AllocatableCPU) * 100; current.AllocatableCPU > 0 {
			p.CPUHeadroom = headroom
			if p.CPUHeadroom < 0 {
				p.CPUHeadroom = 0
			}
		}
		if headroom := (1.0 - p.ProjectedMem/current.AllocatableMem) * 100; current.AllocatableMem > 0 {
			p.MemHeadroom = headroom
			if p.MemHeadroom < 0 {
				p.MemHeadroom = 0
			}
		}
		if headroom := (1.0 - float64(p.ProjectedPods)/float64(current.PodCapacity)) * 100; current.PodCapacity > 0 {
			p.PodHeadroom = headroom
			if p.PodHeadroom < 0 {
				p.PodHeadroom = 0
			}
		}

		minHeadroom := p.CPUHeadroom
		if p.MemHeadroom < minHeadroom {
			minHeadroom = p.MemHeadroom
		}
		if p.PodHeadroom < minHeadroom {
			minHeadroom = p.PodHeadroom
		}
		if minHeadroom < 10 {
			p.Status = "critical"
		} else if minHeadroom < 25 {
			p.Status = "warning"
		} else {
			p.Status = "ok"
		}
		return p
	}

	result.Forecast.Projection90d = project(current.RequestedCPU, current.AllocatableCPU, growth.CPUPercentPerMonth, 90)
	result.Forecast.Projection180d = project(current.RequestedCPU, current.AllocatableCPU, growth.CPUPercentPerMonth, 180)

	// Time-to-exhaustion calculation
	tte := CapacityTTE{}
	if growth.CPUPercentPerMonth > 0 && current.AllocatableCPU > current.RequestedCPU {
		remaining := current.AllocatableCPU - current.RequestedCPU
		monthlyGrowth := current.RequestedCPU * (growth.CPUPercentPerMonth / 100)
		if monthlyGrowth > 0 {
			tte.CPUExhaustionDays = int((remaining / monthlyGrowth) * 30)
		}
	}
	if growth.MemPercentPerMonth > 0 && current.AllocatableMem > current.RequestedMem {
		remaining := current.AllocatableMem - current.RequestedMem
		monthlyGrowth := current.RequestedMem * (growth.MemPercentPerMonth / 100)
		if monthlyGrowth > 0 {
			tte.MemExhaustionDays = int((remaining / monthlyGrowth) * 30)
		}
	}
	if growth.PodPercentPerMonth > 0 && current.PodCapacity > current.PodCount {
		remaining := float64(current.PodCapacity - current.PodCount)
		monthlyGrowth := float64(current.PodCount) * (growth.PodPercentPerMonth / 100)
		if monthlyGrowth > 0 {
			tte.PodExhaustionDays = int((remaining / monthlyGrowth) * 30)
		}
	}

	// First bottleneck
	tte.FirstBottleneck = "none"
	minDays := -1
	for _, item := range []struct {
		name string
		days int
	}{{"cpu", tte.CPUExhaustionDays}, {"memory", tte.MemExhaustionDays}, {"pods", tte.PodExhaustionDays}} {
		if item.days > 0 && (minDays < 0 || item.days < minDays) {
			minDays = item.days
			tte.FirstBottleneck = item.name
		}
	}
	result.Forecast.TimeToExhaustion = tte

	// Namespace breakdown
	for _, ns := range nsMap {
		if current.RequestedCPU > 0 {
			ns.CPUPercent = (ns.ReqCPU / current.RequestedCPU) * 100
		}
		if current.RequestedMem > 0 {
			ns.MemPercent = (ns.ReqMem / current.RequestedMem) * 100
		}
		// Growth rate per namespace
		var nsPods30d int
		for _, d := range deployments.Items {
			if d.Namespace == ns.Namespace && now.Sub(d.CreationTimestamp.Time) < 30*24*time.Hour {
				nsPods30d += int(ptrInt32(d.Spec.Replicas))
			}
		}
		ns.GrowthRate = float64(nsPods30d) / 4.3 // per week
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ReqCPU > result.ByNamespace[j].ReqCPU
	})

	// Top consumers (from deployments)
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		replicas := int(ptrInt32(d.Spec.Replicas))
		var reqCPU, reqMem float64
		for _, c := range d.Spec.Template.Spec.Containers {
			if v, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				reqCPU += v.AsApproximateFloat64() * float64(replicas)
			}
			if v, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				reqMem += v.AsApproximateFloat64() / 1e9 * float64(replicas)
			}
		}
		if reqCPU > 0 || reqMem > 0 {
			consumers = append(consumers, CapacityConsumer{
				Name: d.Name, Namespace: d.Namespace, Kind: "Deployment",
				ReqCPU: reqCPU, ReqMem: reqMem, Pods: replicas,
			})
		}
	}
	sort.Slice(consumers, func(i, j int) bool {
		return consumers[i].ReqCPU > consumers[j].ReqCPU
	})
	if len(consumers) > 20 {
		consumers = consumers[:20]
	}
	result.TopConsumers = consumers

	// Growth trend (monthly buckets for past 6 months)
	for i := 5; i >= 0; i-- {
		date := now.AddDate(0, -i, 0)
		var count int
		for _, d := range deployments.Items {
			if isSystemNamespace(d.Namespace) {
				continue
			}
			if d.CreationTimestamp.Time.Before(date) || d.CreationTimestamp.Time.Equal(date) {
				count += int(ptrInt32(d.Spec.Replicas))
			}
		}
		result.GrowthTrend = append(result.GrowthTrend, GrowthPoint{
			Date:     date.Format("2006-01"),
			PodCount: count,
		})
	}

	// Health score based on current utilization and headroom
	result.HealthScore = calculateCapacityScore(&result)

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 65:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	case result.HealthScore >= 35:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildCapacityRecs(&result)

	writeJSON(w, result)
}

func calculateCapacityScore(r *CapacityForecastResult) int {
	score := 100

	// Penalize high utilization
	if r.Current.CPUUtilization > 0.8 {
		score -= 25
	} else if r.Current.CPUUtilization > 0.7 {
		score -= 15
	} else if r.Current.CPUUtilization > 0.6 {
		score -= 5
	}

	if r.Current.MemUtilization > 0.8 {
		score -= 25
	} else if r.Current.MemUtilization > 0.7 {
		score -= 15
	} else if r.Current.MemUtilization > 0.6 {
		score -= 5
	}

	if r.Current.PodUtilization > 0.8 {
		score -= 15
	} else if r.Current.PodUtilization > 0.7 {
		score -= 8
	}

	// Penalize projected exhaustion
	if r.Forecast.Projection90d.Status == "critical" {
		score -= 15
	} else if r.Forecast.Projection90d.Status == "warning" {
		score -= 8
	}

	if score < 0 {
		score = 0
	}
	return score
}

func buildCapacityRecs(r *CapacityForecastResult) []string {
	recs := []string{}

	if r.Current.CPUUtilization > 0.7 {
		recs = append(recs, fmt.Sprintf("CPU 利用率 %.0f%%，建议扩容节点或优化资源请求", r.Current.CPUUtilization*100))
	}
	if r.Current.MemUtilization > 0.7 {
		recs = append(recs, fmt.Sprintf("内存利用率 %.0f%%，建议扩容或排查内存泄漏", r.Current.MemUtilization*100))
	}
	if r.Forecast.TimeToExhaustion.FirstBottleneck != "none" {
		recs = append(recs, fmt.Sprintf("预计 %s 将在 %d 天内耗尽，建议提前规划扩容",
			r.Forecast.TimeToExhaustion.FirstBottleneck, r.Forecast.TimeToExhaustion.CPUExhaustionDays))
	}
	if r.Forecast.GrowthRate30d.NewPodsPerWeek > 5 {
		recs = append(recs, fmt.Sprintf("每周新增 %.1f 个 Pod，增长率较高，建议设置集群自动扩缩容", r.Forecast.GrowthRate30d.NewPodsPerWeek))
	}
	if r.Current.PodUtilization > 0.7 {
		recs = append(recs, fmt.Sprintf("Pod 密度 %.0f%%，接近节点 Pod 上限，考虑增加节点", r.Current.PodUtilization*100))
	}
	if len(recs) == 0 {
		recs = append(recs, "集群容量充足，建议定期审查资源请求以优化成本")
	}

	return recs
}

// resourceToCores converts a resource.Quantity to CPU cores as float64.
func resourceToCores(q resource.Quantity) float64 {
	return q.AsApproximateFloat64()
}

// suppress unused import warning for strings
var _ = strings.Contains
