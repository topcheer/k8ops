package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceBudgetEnforceResult audits namespace resource budgets and enforcement gaps.
type NamespaceBudgetEnforceResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         BudgetEnforceSummary `json:"summary"`
	ByNamespace     []BudgetEnforceEntry `json:"byNamespace"`
	OverBudgetNS    []BudgetEnforceEntry `json:"overBudgetNamespaces"`
	UnprotectedNS   []BudgetEnforceEntry `json:"unprotectedNamespaces"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type BudgetEnforceSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithQuota       int `json:"withResourceQuota"`
	WithoutQuota    int `json:"withoutResourceQuota"`
	WithLimitRange  int `json:"withLimitRange"`
	OverBudget      int `json:"overBudgetNamespaces"`
	HighSpendNS     int `json:"highSpendNamespaces"`
}

type BudgetEnforceEntry struct {
	Namespace     string   `json:"namespace"`
	PodCount      int      `json:"podCount"`
	CPURequest    float64  `json:"cpuRequestCores"`
	MemRequestGB  float64  `json:"memRequestGB"`
	HasQuota      bool     `json:"hasResourceQuota"`
	HasLimitRange bool     `json:"hasLimitRange"`
	EstCostMo     float64  `json:"estimatedCostPerMonth"`
	RiskLevel     string   `json:"riskLevel"`
	Issues        []string `json:"issues"`
}

// handleNamespaceBudgetEnforce handles GET /api/scalability/namespace-budget-enforce
func (s *Server) handleNamespaceBudgetEnforce(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := NamespaceBudgetEnforceResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})

	// Build quota and limitrange maps
	nsQuota := make(map[string]bool)
	for _, rq := range quotas.Items {
		nsQuota[rq.Namespace] = true
	}
	nsLimitRange := make(map[string]bool)
	for _, lr := range limitRanges.Items {
		nsLimitRange[lr.Namespace] = true
	}

	// Aggregate per-namespace resource usage
	nsData := make(map[string]*BudgetEnforceEntry)
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++
		nsData[ns.Name] = &BudgetEnforceEntry{
			Namespace:     ns.Name,
			HasQuota:      nsQuota[ns.Name],
			HasLimitRange: nsLimitRange[ns.Name],
		}
		if nsQuota[ns.Name] {
			result.Summary.WithQuota++
		} else {
			result.Summary.WithoutQuota++
		}
		if nsLimitRange[ns.Name] {
			result.Summary.WithLimitRange++
		}
	}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		entry, ok := nsData[pod.Namespace]
		if !ok {
			entry = &BudgetEnforceEntry{Namespace: pod.Namespace}
			nsData[pod.Namespace] = entry
			result.Summary.TotalNamespaces++
		}
		entry.PodCount++
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				entry.CPURequest += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				entry.MemRequestGB += float64(req.Value()) / (1024 * 1024 * 1024)
			}
		}
	}

	// Estimate monthly cost: CPU $20/core/mo, Memory $3/GB/mo
	for _, entry := range nsData {
		entry.EstCostMo = entry.CPURequest*20 + entry.MemRequestGB*3

		var issues []string
		if !entry.HasQuota {
			issues = append(issues, "no-resource-quota")
		}
		if !entry.HasLimitRange {
			issues = append(issues, "no-limit-range")
		}
		if entry.EstCostMo > 50 {
			issues = append(issues, fmt.Sprintf("high-spend $%.0f/mo", entry.EstCostMo))
			result.Summary.HighSpendNS++
		}

		entry.Issues = issues
		switch {
		case len(issues) >= 3:
			entry.RiskLevel = "critical"
			result.Summary.OverBudget++
		case len(issues) >= 2:
			entry.RiskLevel = "high"
		case len(issues) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.OverBudgetNS = append(result.OverBudgetNS, *entry)
		}
		if !entry.HasQuota {
			result.UnprotectedNS = append(result.UnprotectedNS, *entry)
		}
	}

	var entries []BudgetEnforceEntry
	for _, e := range nsData {
		entries = append(entries, *e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].EstCostMo > entries[j].EstCostMo
	})
	result.ByNamespace = entries

	if result.Summary.TotalNamespaces > 0 {
		result.HealthScore = result.Summary.WithQuota * 100 / result.Summary.TotalNamespaces
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("命名空间预算审计: %d 命名空间, %d 有配额, %d 无配额, %d 高消费, %d 无 LimitRange",
			result.Summary.TotalNamespaces, result.Summary.WithQuota, result.Summary.WithoutQuota,
			result.Summary.HighSpendNS, result.Summary.TotalNamespaces-result.Summary.WithLimitRange),
	}
	if result.Summary.WithoutQuota > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个命名空间无 ResourceQuota, 资源不受约束", result.Summary.WithoutQuota))
	}
	if result.Summary.HighSpendNS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个命名空间月成本超过 $50", result.Summary.HighSpendNS))
	}
	writeJSON(w, result)
}
