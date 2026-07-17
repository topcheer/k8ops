package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostAnomalyResult detects unusual resource allocation patterns that
// may indicate cost overruns: oversized requests, underutilized allocations,
// sudden spikes in resource consumption, and namespace-level anomalies.
type CostAnomalyResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         CostAnomalySummary `json:"summary"`
	Anomalies       []CostAnomalyItem  `json:"anomalies"`
	ByNamespace     []CostAnomalyNS    `json:"byNamespace"`
	TopSpenders     []CostSpender      `json:"topSpenders"`
	EstimatedWaste  CostWasteEstimate  `json:"estimatedWaste"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type CostAnomalySummary struct {
	TotalWorkloads   int     `json:"totalWorkloads"`
	AnomalyCount     int     `json:"anomalyCount"`
	OversizedCount   int     `json:"oversizedCount"`
	IdleCount        int     `json:"idleCount"`
	SpikeCount       int     `json:"spikeCount"`
	TotalMonthlyCost float64 `json:"totalMonthlyCostUSD"`
	AnomalyCost      float64 `json:"anomalyCostUSD"`
}

type CostAnomalyItem struct {
	Workload      string  `json:"workload"`
	Namespace     string  `json:"namespace"`
	Kind          string  `json:"kind"`
	Type          string  `json:"type"` // oversized, idle, spike, misconfigured
	Severity      string  `json:"severity"`
	Detail        string  `json:"detail"`
	CurrentCost   float64 `json:"currentCostUSD"`
	ExpectedCost  float64 `json:"expectedCostUSD"`
	PotentialSave float64 `json:"potentialSaveUSD"`
}

type CostAnomalyNS struct {
	Namespace   string  `json:"namespace"`
	Workloads   int     `json:"workloadCount"`
	MonthlyCost float64 `json:"monthlyCostUSD"`
	Anomalies   int     `json:"anomalyCount"`
	WastePct    float64 `json:"wastePct"`
}

type CostSpender struct {
	Name        string  `json:"name"`
	Namespace   string  `json:"namespace"`
	Kind        string  `json:"kind"`
	MonthlyCost float64 `json:"monthlyCostUSD"`
	CPUCost     float64 `json:"cpuCostUSD"`
	MemCost     float64 `json:"memCostUSD"`
}

type CostWasteEstimate struct {
	TotalWasteUSD  float64 `json:"totalWasteUSD"`
	WastePct       float64 `json:"wastePct"`
	OversizedWaste float64 `json:"oversizedWasteUSD"`
	IdleWaste      float64 `json:"idleWasteUSD"`
}

// handleCostAnomaly handles GET /api/scalability/cost-anomaly
func (s *Server) handleCostAnomaly(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CostAnomalyResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	// Calculate node unit cost
	nodeCount := len(nodes.Items)
	nodeCPUCapacity := 0.0
	nodeMemCapacity := 0.0
	for _, n := range nodes.Items {
		nodeCPUCapacity += n.Status.Allocatable.Cpu().AsApproximateFloat64()
		nodeMemCapacity += n.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	// Assume $100/node/month for cost estimation
	nodeMonthlyCost := float64(nodeCount) * 100.0
	cpuUnitCost := 0.0 // cost per core/month
	memUnitCost := 0.0 // cost per GB/month
	if nodeCPUCapacity > 0 {
		cpuUnitCost = nodeMonthlyCost * 0.6 / nodeCPUCapacity // 60% CPU-weighted
	}
	if nodeMemCapacity > 0 {
		memUnitCost = nodeMonthlyCost * 0.4 / nodeMemCapacity // 40% mem-weighted
	}

	// Pod resource usage by namespace
	nsStats := make(map[string]*CostAnomalyNS)
	var anomalies []CostAnomalyItem
	var spenders []CostSpender

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		replicas := int(ptrInt32(d.Spec.Replicas))
		result.Summary.TotalWorkloads++

		if _, ok := nsStats[d.Namespace]; !ok {
			nsStats[d.Namespace] = &CostAnomalyNS{Namespace: d.Namespace}
		}
		ns := nsStats[d.Namespace]
		ns.Workloads++

		// Calculate per-workload cost
		var reqCPU, reqMem float64
		for _, c := range d.Spec.Template.Spec.Containers {
			if v, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				reqCPU += v.AsApproximateFloat64() * float64(replicas)
			}
			if v, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				reqMem += v.AsApproximateFloat64() / 1e9 * float64(replicas)
			}
		}

		cpuCost := reqCPU * cpuUnitCost
		memCost := reqMem * memUnitCost
		totalCost := cpuCost + memCost

		ns.MonthlyCost += totalCost
		result.Summary.TotalMonthlyCost += totalCost

		if totalCost > 0 {
			spenders = append(spenders, CostSpender{
				Name: d.Name, Namespace: d.Namespace, Kind: "Deployment",
				MonthlyCost: totalCost, CPUCost: cpuCost, MemCost: memCost,
			})
		}

		// Detect anomalies
		// 1. Oversized: single container requesting > 4 CPU or > 8GB memory
		for _, c := range d.Spec.Template.Spec.Containers {
			if v, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cores := v.AsApproximateFloat64() * float64(replicas)
				if cores > 8 {
					anomalies = append(anomalies, CostAnomalyItem{
						Workload: d.Name, Namespace: d.Namespace, Kind: "Deployment",
						Type: "oversized", Severity: "high",
						Detail:      fmt.Sprintf("CPU 请求 %.1f 核，超过单工作负载 8 栍阈值", cores),
						CurrentCost: cpuCost, ExpectedCost: cpuCost * 0.5, PotentialSave: cpuCost * 0.5,
					})
					result.Summary.OversizedCount++
				}
			}
			if v, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				memGB := v.AsApproximateFloat64() / 1e9 * float64(replicas)
				if memGB > 16 {
					anomalies = append(anomalies, CostAnomalyItem{
						Workload: d.Name, Namespace: d.Namespace, Kind: "Deployment",
						Type: "oversized", Severity: "high",
						Detail:      fmt.Sprintf("内存请求 %.1f GB，超过单工作负载 16GB 阈值", memGB),
						CurrentCost: memCost, ExpectedCost: memCost * 0.5, PotentialSave: memCost * 0.5,
					})
					result.Summary.OversizedCount++
				}
			}
		}

		// 2. Idle: 0 replicas but Deployment exists
		if replicas == 0 {
			anomalies = append(anomalies, CostAnomalyItem{
				Workload: d.Name, Namespace: d.Namespace, Kind: "Deployment",
				Type: "idle", Severity: "low",
				Detail: "副本数为 0，Deployment 可能已废弃",
			})
			result.Summary.IdleCount++
		}

		// 3. No resource requests at all
		if reqCPU == 0 && reqMem == 0 {
			anomalies = append(anomalies, CostAnomalyItem{
				Workload: d.Name, Namespace: d.Namespace, Kind: "Deployment",
				Type: "misconfigured", Severity: "medium",
				Detail: "未设置资源请求，无法准确计费",
			})
		}
	}

	// Check pods for running status (idle detection)
	runningPods := 0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		if pod.Status.Phase == corev1.PodRunning {
			runningPods++
		}
	}

	// NS stats
	for _, ns := range nsStats {
		nsAnomCount := 0
		for _, a := range anomalies {
			if a.Namespace == ns.Namespace {
				nsAnomCount++
			}
		}
		ns.Anomalies = nsAnomCount
		if ns.MonthlyCost > 0 {
			for _, a := range anomalies {
				if a.Namespace == ns.Namespace && a.PotentialSave > 0 {
					ns.WastePct += a.PotentialSave / ns.MonthlyCost * 100
				}
			}
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MonthlyCost > result.ByNamespace[j].MonthlyCost
	})

	// Top spenders
	sort.Slice(spenders, func(i, j int) bool {
		return spenders[i].MonthlyCost > spenders[j].MonthlyCost
	})
	if len(spenders) > 15 {
		spenders = spenders[:15]
	}
	result.TopSpenders = spenders

	// Anomalies
	sort.Slice(anomalies, func(i, j int) bool {
		return anomalies[i].PotentialSave > anomalies[j].PotentialSave
	})
	result.Anomalies = anomalies
	result.Summary.AnomalyCount = len(anomalies)

	// Waste estimate
	var totalWaste, oversizedWaste, idleWaste float64
	for _, a := range anomalies {
		totalWaste += a.PotentialSave
		if a.Type == "oversized" {
			oversizedWaste += a.PotentialSave
		}
		if a.Type == "idle" {
			idleWaste += a.PotentialSave
		}
	}
	result.Summary.AnomalyCost = totalWaste
	result.EstimatedWaste = CostWasteEstimate{
		TotalWasteUSD: totalWaste, OversizedWaste: oversizedWaste, IdleWaste: idleWaste,
	}
	if result.Summary.TotalMonthlyCost > 0 {
		result.EstimatedWaste.WastePct = totalWaste / result.Summary.TotalMonthlyCost * 100
	}

	// Score
	score := 100
	if result.Summary.TotalWorkloads > 0 {
		anomRate := float64(result.Summary.AnomalyCount) / float64(result.Summary.TotalWorkloads) * 100
		score -= int(anomRate * 2)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 65:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildCostAnomalyRecs(&result)
	writeJSON(w, result)
}

func buildCostAnomalyRecs(r *CostAnomalyResult) []string {
	recs := []string{}
	if r.Summary.AnomalyCount == 0 {
		recs = append(recs, "未检测到成本异常")
		return recs
	}
	if r.Summary.OversizedCount > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载资源过大，优化后每月可节省 $%.2f", r.Summary.OversizedCount, r.EstimatedWaste.OversizedWaste))
	}
	if r.Summary.IdleCount > 0 {
		recs = append(recs, fmt.Sprintf("%d 个零副本工作负载，建议清理", r.Summary.IdleCount))
	}
	if r.EstimatedWaste.WastePct > 20 {
		recs = append(recs, fmt.Sprintf("资源浪费率 %.0f%%，建议系统性优化", r.EstimatedWaste.WastePct))
	}
	if len(r.TopSpenders) > 0 {
		recs = append(recs, fmt.Sprintf("最大开支者: %s/%s ($%.2f/月)", r.TopSpenders[0].Namespace, r.TopSpenders[0].Name, r.TopSpenders[0].MonthlyCost))
	}
	return recs
}
