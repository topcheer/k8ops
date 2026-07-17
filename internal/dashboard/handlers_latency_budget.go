package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LatencyBudgetResult allocates latency budgets across service paths and
// identifies services exceeding their allocated latency SLO.
type LatencyBudgetResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	TargetP99       int                  `json:"targetP99Ms"`
	Summary         LatencyBudgetSummary `json:"summary"`
	ByService       []LatencyBudgetEntry `json:"byService"`
	OverBudget      []LatencyBudgetEntry `json:"overBudget"`
	BudgetScore     int                  `json:"budgetScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type LatencyBudgetSummary struct {
	TotalServices   int     `json:"totalServices"`
	WithinBudget    int     `json:"withinBudget"`
	OverBudgetCount int     `json:"overBudgetCount"`
	NoBudgetSet     int     `json:"noBudgetSet"`
	AvgAllocated    float64 `json:"avgAllocatedMs"`
	AvgEstimated    float64 `json:"avgEstimatedMs"`
	TotalBudgetMs   int     `json:"totalBudgetMs"`
}

type LatencyBudgetEntry struct {
	ServiceName    string             `json:"serviceName"`
	Namespace      string             `json:"namespace"`
	BackingPods    int                `json:"backingPods"`
	HasProbe       bool               `json:"hasProbe"`
	ProbeTimeoutMs int                `json:"probeTimeoutMs"`
	AllocatedMs    int                `json:"allocatedMs"`
	EstimatedMs    int                `json:"estimatedMs"`
	BudgetUtilPct  float64            `json:"budgetUtilPct"`
	Status         string             `json:"status"`
	RiskLevel      string             `json:"riskLevel"`
	Components     []LatencyComponent `json:"components"`
}

type LatencyComponent struct {
	Name     string `json:"name"`
	Ms       int    `json:"ms"`
	Category string `json:"category"`
}

// handleLatencyBudget handles GET /api/product/latency-budget
func (s *Server) handleLatencyBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	targetP99 := 500 // ms global target
	result := LatencyBudgetResult{
		ScannedAt: time.Now(),
		TargetP99: targetP99,
	}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Build deployment lookup by labels
	type depInfo struct {
		name   string
		labels map[string]string
	}
	depByNs := make(map[string][]depInfo)
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		depByNs[d.Namespace] = append(depByNs[d.Namespace], depInfo{d.Name, d.Spec.Template.Labels})
	}

	// Build pod health map for latency estimation
	nsRestartCount := make(map[string]int)
	nsPodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		nsPodCount[pod.Namespace]++
		for _, cs := range pod.Status.ContainerStatuses {
			nsRestartCount[pod.Namespace] += int(cs.RestartCount)
		}
	}

	var entries []LatencyBudgetEntry
	totalAllocated := 0
	totalEstimated := 0

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		if svc.Spec.ClusterIP == "None" && svc.Spec.ClusterIPs == nil {
			continue
		}

		result.Summary.TotalServices++
		entry := LatencyBudgetEntry{
			ServiceName: svc.Name,
			Namespace:   svc.Namespace,
			AllocatedMs: targetP99,
		}

		// Find backing deployment
		if svc.Spec.Selector != nil && len(svc.Spec.Selector) > 0 {
			for _, dep := range depByNs[svc.Namespace] {
				match := true
				for k, v := range svc.Spec.Selector {
					if dep.labels[k] != v {
						match = false
						break
					}
				}
				if match {
					// Check probes and estimate latency
					for _, d := range deployments.Items {
						if d.Namespace == svc.Namespace && d.Name == dep.name {
							for _, c := range d.Spec.Template.Spec.Containers {
								if c.ReadinessProbe != nil {
									entry.HasProbe = true
									if c.ReadinessProbe.TimeoutSeconds > 0 {
										entry.ProbeTimeoutMs = int(c.ReadinessProbe.TimeoutSeconds) * 1000
									}
								}
							}
							break
						}
					}
					break
				}
			}
		}

		// Count backing pods
		for _, pod := range pods.Items {
			if pod.Namespace != svc.Namespace || pod.Status.Phase != corev1.PodRunning {
				continue
			}
			if svc.Spec.Selector == nil {
				continue
			}
			match := true
			for k, v := range svc.Spec.Selector {
				if pod.Labels[k] != v {
					match = false
					break
				}
			}
			if match {
				entry.BackingPods++
			}
		}

		// Estimate latency from available signals
		estimated := 50 // base latency ms
		components := []LatencyComponent{
			{Name: "base-network", Ms: 50, Category: "network"},
		}

		// Probe overhead
		if entry.HasProbe {
			overhead := 20
			estimated += overhead
			components = append(components, LatencyComponent{Name: "probe-check", Ms: overhead, Category: "health"})
		} else {
			estimated += 50
			components = append(components, LatencyComponent{Name: "no-probe-penalty", Ms: 50, Category: "health"})
		}

		// Restart penalty
		restarts := nsRestartCount[svc.Namespace]
		if restarts > 0 && nsPodCount[svc.Namespace] > 0 {
			restartPenalty := restarts * 5 / nsPodCount[svc.Namespace]
			if restartPenalty > 200 {
				restartPenalty = 200
			}
			estimated += restartPenalty
			if restartPenalty > 0 {
				components = append(components, LatencyComponent{Name: "restart-penalty", Ms: restartPenalty, Category: "stability"})
			}
		}

		// Pod count factor (fewer pods = higher latency under load)
		if entry.BackingPods == 1 {
			estimated += 100
			components = append(components, LatencyComponent{Name: "single-pod-bottleneck", Ms: 100, Category: "capacity"})
		} else if entry.BackingPods <= 3 {
			estimated += 30
			components = append(components, LatencyComponent{Name: "low-replica-count", Ms: 30, Category: "capacity"})
		}

		entry.EstimatedMs = estimated
		entry.Components = components
		totalAllocated += entry.AllocatedMs
		totalEstimated += entry.EstimatedMs

		if entry.AllocatedMs > 0 {
			entry.BudgetUtilPct = float64(entry.EstimatedMs) / float64(entry.AllocatedMs) * 100
		}

		// Status
		switch {
		case entry.BudgetUtilPct > 100:
			entry.Status = "over-budget"
			entry.RiskLevel = "critical"
			result.Summary.OverBudgetCount++
		case entry.BudgetUtilPct > 80:
			entry.Status = "near-limit"
			entry.RiskLevel = "warning"
		case entry.BudgetUtilPct > 50:
			entry.Status = "within-budget"
			entry.RiskLevel = "low"
		default:
			entry.Status = "healthy"
			entry.RiskLevel = "none"
			result.Summary.WithinBudget++
		}

		entries = append(entries, entry)
	}

	// Sort by budget utilization descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].BudgetUtilPct > entries[j].BudgetUtilPct
	})
	result.ByService = entries

	// Collect over budget
	for _, e := range entries {
		if e.Status == "over-budget" {
			result.OverBudget = append(result.OverBudget, e)
		}
	}

	// Averages
	if result.Summary.TotalServices > 0 {
		result.Summary.AvgAllocated = float64(totalAllocated) / float64(result.Summary.TotalServices)
		result.Summary.AvgEstimated = float64(totalEstimated) / float64(result.Summary.TotalServices)
	}
	result.Summary.TotalBudgetMs = totalAllocated

	// Budget score
	if result.Summary.TotalServices > 0 {
		withinRatio := float64(result.Summary.WithinBudget) / float64(result.Summary.TotalServices)
		result.BudgetScore = int(withinRatio * 100)
	}

	switch {
	case result.BudgetScore >= 80:
		result.Grade = "A"
	case result.BudgetScore >= 60:
		result.Grade = "B"
	case result.BudgetScore >= 40:
		result.Grade = "C"
	case result.BudgetScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildLatencyBudgetRecs(&result)
	writeJSON(w, result)
}

func buildLatencyBudgetRecs(r *LatencyBudgetResult) []string {
	recs := []string{
		fmt.Sprintf("延迟预算: %d 服务, P99 目标 %dms, %d 超预算, %d 健康", r.Summary.TotalServices, r.TargetP99, r.Summary.OverBudgetCount, r.Summary.WithinBudget),
	}
	if r.Summary.OverBudgetCount > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个服务估计延迟超过预算", r.Summary.OverBudgetCount))
	}
	if r.Summary.AvgEstimated > float64(r.TargetP99) {
		recs = append(recs, fmt.Sprintf("平均估计延迟 %.0fms 超过目标 %dms", r.Summary.AvgEstimated, r.TargetP99))
	}
	if len(r.OverBudget) > 0 {
		top := r.OverBudget[0]
		recs = append(recs, fmt.Sprintf("最高超标: %s/%s (估计 %dms / 预算 %dms = %.0f%%)", top.Namespace, top.ServiceName, top.EstimatedMs, top.AllocatedMs, top.BudgetUtilPct))
	}
	if r.BudgetScore < 60 {
		recs = append(recs, "建议: 增加副本数, 添加 readiness probe, 减少重启频率")
	}
	return recs
}
