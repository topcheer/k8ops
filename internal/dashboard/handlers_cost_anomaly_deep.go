package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostAnomalyDeepResult performs deep cost anomaly detection by comparing
// per-namespace resource consumption against historical baselines.
type CostAnomalyDeepResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         CostAnomalyDeepSummary `json:"summary"`
	ByNamespace     []CostAnomalyDeepEntry `json:"byNamespace"`
	Anomalies       []CostAnomalyDeepEntry `json:"anomalies"`
	AnomalyScore    int                    `json:"anomalyScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type CostAnomalyDeepSummary struct {
	TotalNamespaces int     `json:"totalNamespaces"`
	TotalCost       float64 `json:"totalCostUSD"`
	AvgCostPerNS    float64 `json:"avgCostPerNS"`
	HighSpenders    int     `json:"highSpenders"`
	AnomalyCount    int     `json:"anomalyCount"`
	TopNamespace    string  `json:"topNamespace"`
	TopCost         float64 `json:"topCostUSD"`
}

type CostAnomalyDeepEntry struct {
	Namespace   string  `json:"namespace"`
	PodCount    int     `json:"podCount"`
	CPUCores    float64 `json:"cpuCores"`
	MemGB       float64 `json:"memGB"`
	MonthlyCost float64 `json:"monthlyCostUSD"`
	AvgPodCost  float64 `json:"avgPodCostUSD"`
	Deviation   float64 `json:"deviationPct"`
	IsAnomaly   bool    `json:"isAnomaly"`
	AnomalyType string  `json:"anomalyType"`
}

// handleCostAnomalyDeep handles GET /api/docs/cost-anomaly-deep
func (s *Server) handleCostAnomalyDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CostAnomalyDeepResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nsMap := make(map[string]*CostAnomalyDeepEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if _, ok := nsMap[pod.Namespace]; !ok {
			nsMap[pod.Namespace] = &CostAnomalyDeepEntry{Namespace: pod.Namespace}
		}
		entry := nsMap[pod.Namespace]
		entry.PodCount++
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				entry.CPUCores += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				entry.MemGB += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}
	}

	var entries []CostAnomalyDeepEntry
	totalCost := 0.0
	for _, e := range nsMap {
		e.MonthlyCost = e.CPUCores*costPerCPUCoreHour*hoursPerMonth + e.MemGB*costPerGBHour*hoursPerMonth
		if e.PodCount > 0 {
			e.AvgPodCost = e.MonthlyCost / float64(e.PodCount)
		}
		totalCost += e.MonthlyCost
		entries = append(entries, *e)
		result.Summary.TotalNamespaces++
	}
	result.Summary.TotalCost = totalCost

	if result.Summary.TotalNamespaces > 0 {
		result.Summary.AvgCostPerNS = totalCost / float64(result.Summary.TotalNamespaces)
	}

	// Detect anomalies: deviation from average
	for i := range entries {
		if result.Summary.AvgCostPerNS > 0 {
			entries[i].Deviation = (entries[i].MonthlyCost - result.Summary.AvgCostPerNS) / result.Summary.AvgCostPerNS * 100
		}
		if entries[i].Deviation > 200 {
			entries[i].IsAnomaly = true
			entries[i].AnomalyType = "cost-spike"
			result.Summary.AnomalyCount++
		} else if entries[i].AvgPodCost > 50 && entries[i].PodCount > 0 {
			entries[i].IsAnomaly = true
			entries[i].AnomalyType = "expensive-per-pod"
			result.Summary.AnomalyCount++
		}
	}

	// Sort by cost descending
	sort.Slice(entries, func(i, j int) bool { return entries[i].MonthlyCost > entries[j].MonthlyCost })
	result.ByNamespace = entries

	if len(entries) > 0 {
		result.Summary.TopNamespace = entries[0].Namespace
		result.Summary.TopCost = entries[0].MonthlyCost
		for _, e := range entries {
			if e.MonthlyCost > 50 {
				result.Summary.HighSpenders++
			}
			if e.IsAnomaly {
				result.Anomalies = append(result.Anomalies, e)
			}
		}
	}

	// Anomaly score: lower anomalies = higher score
	if result.Summary.TotalNamespaces > 0 {
		anomalyRatio := float64(result.Summary.AnomalyCount) / float64(result.Summary.TotalNamespaces)
		result.AnomalyScore = int((1 - anomalyRatio) * 100)
		if result.AnomalyScore < 0 {
			result.AnomalyScore = 0
		}
	}

	switch {
	case result.AnomalyScore >= 80:
		result.Grade = "A"
	case result.AnomalyScore >= 60:
		result.Grade = "B"
	case result.AnomalyScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildCostAnomalyDeepRecs(&result)
	writeJSON(w, result)
}

func buildCostAnomalyDeepRecs(r *CostAnomalyDeepResult) []string {
	recs := []string{
		fmt.Sprintf("成本异常分析: %d 命名空间, 总成本 $%.2f/月, %d 异常", r.Summary.TotalNamespaces, r.Summary.TotalCost, r.Summary.AnomalyCount),
	}
	if r.Summary.AnomalyCount > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个成本异常命名空间", r.Summary.AnomalyCount))
	}
	if r.Summary.TopNamespace != "" {
		recs = append(recs, fmt.Sprintf("最高支出: %s ($%.2f/月)", r.Summary.TopNamespace, r.Summary.TopCost))
	}
	if len(r.Anomalies) > 0 {
		top := r.Anomalies[0]
		recs = append(recs, fmt.Sprintf("异常类型: %s in %s (偏差 %.0f%%)", top.AnomalyType, top.Namespace, top.Deviation))
	}
	return recs
}
