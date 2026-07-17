package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CostTopologyResult provides per-namespace cost breakdown based on resource
// requests, showing CPU/memory cost attribution and identifying cost concentration.
type CostTopologyResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         CostTopologySummary `json:"summary"`
	ByNamespace     []CostTopologyEntry `json:"byNamespace"`
	TopSpenders     []CostTopologyEntry `json:"topSpenders"`
	CostScore       int                 `json:"costScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type CostTopologySummary struct {
	TotalNamespaces int     `json:"totalNamespaces"`
	TotalCPUCores   float64 `json:"totalCPUCores"`
	TotalMemoryGB   float64 `json:"totalMemoryGB"`
	EstMonthlyCost  float64 `json:"estMonthlyCostUSD"`
	TopNsShare      float64 `json:"topNamespaceSharePct"`
}

type CostTopologyEntry struct {
	Namespace   string  `json:"namespace"`
	PodCount    int     `json:"podCount"`
	CPUCores    float64 `json:"cpuCores"`
	MemoryGB    float64 `json:"memoryGB"`
	CPUCost     float64 `json:"cpuCostUSD"`
	MemCost     float64 `json:"memCostUSD"`
	TotalCost   float64 `json:"totalCostUSD"`
	MonthlyCost float64 `json:"monthlyCostUSD"`
	Share       float64 `json:"sharePct"`
	Efficiency  string  `json:"efficiency"`
}

// Pricing model (on-demand rates, rough estimates)
const (
	costPerCPUCoreHour = 0.034  // ~$25/mo per vCPU
	costPerGBHour      = 0.0046 // ~$3.4/mo per GB
	hoursPerMonth      = 730
)

// handleCostTopology handles GET /api/product/cost-topology
func (s *Server) handleCostTopology(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CostTopologyResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nsMap := make(map[string]*CostTopologyEntry)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		ns := pod.Namespace
		if _, ok := nsMap[ns]; !ok {
			nsMap[ns] = &CostTopologyEntry{Namespace: ns}
		}
		entry := nsMap[ns]
		entry.PodCount++

		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				entry.CPUCores += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				entry.MemoryGB += float64(req.ScaledValue(resource.Mega)) / 1024.0
			}
		}
	}

	var entries []CostTopologyEntry
	for _, e := range nsMap {
		e.CPUCost = e.CPUCores * costPerCPUCoreHour * hoursPerMonth
		e.MemCost = e.MemoryGB * costPerGBHour * hoursPerMonth
		e.TotalCost = e.CPUCost + e.MemCost
		e.MonthlyCost = e.TotalCost

		// Efficiency rating based on cost vs pod count
		switch {
		case e.PodCount == 0:
			e.Efficiency = "idle"
		case e.TotalCost/float64(e.PodCount) > 100:
			e.Efficiency = "expensive"
		case e.TotalCost/float64(e.PodCount) > 30:
			e.Efficiency = "moderate"
		default:
			e.Efficiency = "efficient"
		}

		result.Summary.TotalCPUCores += e.CPUCores
		result.Summary.TotalMemoryGB += e.MemoryGB
		result.Summary.EstMonthlyCost += e.TotalCost
		entries = append(entries, *e)
	}

	// Sort by cost descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].TotalCost > entries[j].TotalCost
	})

	// Calculate share percentages
	for i := range entries {
		if result.Summary.EstMonthlyCost > 0 {
			entries[i].Share = entries[i].TotalCost / result.Summary.EstMonthlyCost * 100
		}
	}

	result.Summary.TotalNamespaces = len(entries)
	result.ByNamespace = entries

	// Top 5 spenders
	topN := 5
	if len(entries) < topN {
		topN = len(entries)
	}
	result.TopSpenders = entries[:topN]
	if topN > 0 && result.Summary.EstMonthlyCost > 0 {
		topSum := 0.0
		for i := 0; i < topN; i++ {
			topSum += entries[i].TotalCost
		}
		result.Summary.TopNsShare = topSum / result.Summary.EstMonthlyCost * 100
	}

	// Cost score: lower cost per namespace = higher score
	result.CostScore = 100
	if result.Summary.TotalNamespaces > 0 {
		avgCost := result.Summary.EstMonthlyCost / float64(result.Summary.TotalNamespaces)
		if avgCost > 500 {
			result.CostScore = 30
		} else if avgCost > 200 {
			result.CostScore = 60
		} else if avgCost > 100 {
			result.CostScore = 80
		}
	}

	switch {
	case result.CostScore >= 80:
		result.Grade = "A"
	case result.CostScore >= 60:
		result.Grade = "B"
	case result.CostScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildCostTopologyRecs(&result)
	writeJSON(w, result)
}

func buildCostTopologyRecs(r *CostTopologyResult) []string {
	recs := []string{
		fmt.Sprintf("月度估算成本: $%.2f (CPU %.1f 核, 内存 %.1f GB)", r.Summary.EstMonthlyCost, r.Summary.TotalCPUCores, r.Summary.TotalMemoryGB),
	}
	if r.Summary.TopNsShare > 80 {
		recs = append(recs, fmt.Sprintf("成本集中: Top5 命名空间占总成本 %.1f%%", r.Summary.TopNsShare))
	}
	if len(r.TopSpenders) > 0 {
		recs = append(recs, fmt.Sprintf("最高支出: %s ($%.2f/月)", r.TopSpenders[0].Namespace, r.TopSpenders[0].MonthlyCost))
	}
	if r.CostScore < 60 {
		recs = append(recs, "建议: 审查高成本命名空间的资源请求，考虑使用 limit range 约束")
	}
	return recs
}

// keep resource import
var _ = resource.DecimalSI
