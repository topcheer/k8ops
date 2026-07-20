package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceQuotaDriftResult detects namespace resource quota drift and saturation.
type ResourceQuotaDriftResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         QuotaDriftSummary `json:"summary"`
	ByNamespace     []QuotaDriftEntry `json:"byNamespace"`
	SaturatedQuotas []QuotaDriftEntry `json:"saturatedQuotas"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type QuotaDriftSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithQuotas      int `json:"namespacesWithQuotas"`
	WithoutQuotas   int `json:"namespacesWithoutQuotas"`
	SaturatedQuotas int `json:"saturatedQuotas"` // >80% used
	OverLimitQuotas int `json:"overLimitQuotas"` // hard limit hit
	TotalQuotaItems int `json:"totalQuotaItems"`
}

type QuotaDriftEntry struct {
	Namespace      string          `json:"namespace"`
	QuotaName      string          `json:"quotaName"`
	Resources      []QuotaResource `json:"resources"`
	SaturatedCount int             `json:"saturatedCount"`
	RiskLevel      string          `json:"riskLevel"`
}

type QuotaResource struct {
	Name     string  `json:"name"`
	Hard     string  `json:"hard"`
	Used     string  `json:"used"`
	UsagePct float64 `json:"usagePct"`
	Status   string  `json:"status"`
}

// handleResourceQuotaDrift handles GET /api/deployment/resource-quota-drift
func (s *Server) handleResourceQuotaDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ResourceQuotaDriftResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})

	// Group quotas by namespace
	nsHasQuota := make(map[string]bool)
	for _, rq := range quotas.Items {
		nsHasQuota[rq.Namespace] = true
	}

	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++
		if !nsHasQuota[ns.Name] {
			result.Summary.WithoutQuotas++
		}
	}
	result.Summary.WithQuotas = result.Summary.TotalNamespaces - result.Summary.WithoutQuotas

	for _, rq := range quotas.Items {
		if isSystemNamespace(rq.Namespace) {
			continue
		}
		entry := QuotaDriftEntry{
			Namespace: rq.Namespace,
			QuotaName: rq.Name,
		}

		for resName, hardQty := range rq.Spec.Hard {
			var usedQty resource.Quantity
			if u, exists := rq.Status.Used[resName]; exists {
				usedQty = u
			}
			res := QuotaResource{
				Name: string(resName),
				Hard: hardQty.String(),
				Used: usedQty.String(),
			}

			// Calculate usage percentage
			hardVal := float64(hardQty.MilliValue())
			usedVal := float64(usedQty.MilliValue())
			if hardVal > 0 {
				res.UsagePct = usedVal / hardVal * 100
			}

			switch {
			case res.UsagePct >= 100:
				res.Status = "exhausted"
				entry.SaturatedCount++
				result.Summary.OverLimitQuotas++
			case res.UsagePct >= 80:
				res.Status = "saturated"
				entry.SaturatedCount++
				result.Summary.SaturatedQuotas++
			case res.UsagePct >= 50:
				res.Status = "moderate"
			default:
				res.Status = "healthy"
			}

			entry.Resources = append(entry.Resources, res)
			result.Summary.TotalQuotaItems++
		}

		switch {
		case entry.SaturatedCount >= 3:
			entry.RiskLevel = "critical"
		case entry.SaturatedCount >= 1:
			entry.RiskLevel = "high"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.SaturatedQuotas = append(result.SaturatedQuotas, entry)
		}
		result.ByNamespace = append(result.ByNamespace, entry)
	}

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].SaturatedCount > result.ByNamespace[j].SaturatedCount
	})

	// Score: penalize for namespaces without quotas and saturated quotas
	if result.Summary.TotalNamespaces > 0 {
		quotaCoverage := float64(result.Summary.WithQuotas) / float64(result.Summary.TotalNamespaces)
		result.HealthScore = int(quotaCoverage * 60)
		if result.Summary.SaturatedQuotas == 0 && result.Summary.OverLimitQuotas == 0 {
			result.HealthScore += 40
		} else {
			result.HealthScore -= (result.Summary.SaturatedQuotas + result.Summary.OverLimitQuotas*2) * 5
		}
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("配额漂移: %d 命名空间, %d 有配额, %d 无配额, %d 饱和, %d 超限",
			result.Summary.TotalNamespaces, result.Summary.WithQuotas,
			result.Summary.WithoutQuotas, result.Summary.SaturatedQuotas, result.Summary.OverLimitQuotas),
	}
	if result.Summary.WithoutQuotas > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个命名空间缺少 ResourceQuota, 存在资源耗尽风险", result.Summary.WithoutQuotas))
	}
	if result.Summary.OverLimitQuotas > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个配额已耗尽, 新 Pod 无法调度", result.Summary.OverLimitQuotas))
	}
	if result.HealthScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 为所有命名空间设置 ResourceQuota, 监控配额使用率")
	}
	writeJSON(w, result)
}

// helper to safely format quota resource name
func formatQuotaResName(name string) string {
	return strings.ReplaceAll(name, "requests.", "")
}
