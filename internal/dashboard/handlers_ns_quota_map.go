package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceQuotaMapResult provides a comprehensive map of resource quotas,
// limit ranges, and actual usage per namespace. It identifies namespaces
// without quotas, over-quota risks, and limit range gaps.
type NamespaceQuotaMapResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         NSQuotaMapSummary `json:"summary"`
	Namespaces      []NSQuotaMapEntry `json:"namespaces"`
	WithoutQuota    []string          `json:"withoutQuota"`
	OverQuotaRisk   []NSQuotaRisk     `json:"overQuotaRisk"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type NSQuotaMapSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithQuota       int `json:"withQuota"`
	WithoutQuota    int `json:"withoutQuota"`
	WithLimitRange  int `json:"withLimitRange"`
	QuotaCount      int `json:"quotaCount"`
	AtRiskNS        int `json:"atRiskNamespaces"`
}

type NSQuotaMapEntry struct {
	Namespace     string        `json:"namespace"`
	HasQuota      bool          `json:"hasQuota"`
	HasLimitRange bool          `json:"hasLimitRange"`
	Quotas        []NSQuotaItem `json:"quotas"`
	Usage         NSQuotaUsage  `json:"usage"`
	RiskLevel     string        `json:"riskLevel"`
}

type NSQuotaItem struct {
	Resource string  `json:"resource"`
	Hard     string  `json:"hard"`
	Used     string  `json:"used"`
	UsagePct float64 `json:"usagePct"`
}

type NSQuotaUsage struct {
	CPURequests   float64 `json:"cpuRequests"`
	CPULimits     float64 `json:"cpuLimits"`
	MemRequestsGB float64 `json:"memRequestsGB"`
	MemLimitsGB   float64 `json:"memLimitsGB"`
	PodCount      int     `json:"podCount"`
}

type NSQuotaRisk struct {
	Namespace string  `json:"namespace"`
	Resource  string  `json:"resource"`
	UsagePct  float64 `json:"usagePct"`
	Severity  string  `json:"severity"`
}

// handleNamespaceQuotaMap handles GET /api/product/namespace-quota-map
func (s *Server) handleNamespaceQuotaMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NamespaceQuotaMapResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build maps
	quotaMap := make(map[string][]corev1.ResourceQuota)
	for _, q := range quotas.Items {
		quotaMap[q.Namespace] = append(quotaMap[q.Namespace], q)
	}
	lrMap := make(map[string]bool)
	for _, lr := range limitRanges.Items {
		lrMap[lr.Namespace] = true
	}

	// Calculate pod usage per namespace
	nsUsage := make(map[string]*NSQuotaUsage)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if _, ok := nsUsage[pod.Namespace]; !ok {
			nsUsage[pod.Namespace] = &NSQuotaUsage{}
		}
		u := nsUsage[pod.Namespace]
		u.PodCount++
		for _, c := range pod.Spec.Containers {
			if v, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				u.CPURequests += v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				u.CPULimits += v.AsApproximateFloat64()
			}
			if v, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				u.MemRequestsGB += v.AsApproximateFloat64() / 1e9
			}
			if v, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				u.MemLimitsGB += v.AsApproximateFloat64() / 1e9
			}
		}
	}

	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		entry := NSQuotaMapEntry{
			Namespace:     ns.Name,
			HasQuota:      len(quotaMap[ns.Name]) > 0,
			HasLimitRange: lrMap[ns.Name],
			RiskLevel:     "low",
		}

		if entry.HasQuota {
			result.Summary.WithQuota++
			result.Summary.QuotaCount += len(quotaMap[ns.Name])

			for _, q := range quotaMap[ns.Name] {
				for res, hard := range q.Status.Hard {
					used := q.Status.Used[res]
					usagePct := 0.0
					if hard.MilliValue() > 0 {
						usagePct = float64(used.MilliValue()) / float64(hard.MilliValue()) * 100
					}
					entry.Quotas = append(entry.Quotas, NSQuotaItem{
						Resource: string(res),
						Hard:     hard.String(),
						Used:     used.String(),
						UsagePct: usagePct,
					})
					if usagePct > 80 {
						severity := "high"
						if usagePct > 95 {
							severity = "critical"
						}
						result.OverQuotaRisk = append(result.OverQuotaRisk, NSQuotaRisk{
							Namespace: ns.Name, Resource: string(res),
							UsagePct: usagePct, Severity: severity,
						})
						entry.RiskLevel = "high"
						result.Summary.AtRiskNS++
					}
				}
			}
		} else {
			result.Summary.WithoutQuota++
			result.WithoutQuota = append(result.WithoutQuota, ns.Name)
		}

		if entry.HasLimitRange {
			result.Summary.WithLimitRange++
		}

		if u, ok := nsUsage[ns.Name]; ok {
			entry.Usage = *u
		}

		result.Namespaces = append(result.Namespaces, entry)
	}

	// Score based on quota coverage
	if result.Summary.TotalNamespaces > 0 {
		quotaPct := result.Summary.WithQuota * 100 / result.Summary.TotalNamespaces
		result.HealthScore = quotaPct
	}
	if result.Summary.AtRiskNS > 0 {
		result.HealthScore -= result.Summary.AtRiskNS * 5
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
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

	sort.Slice(result.OverQuotaRisk, func(i, j int) bool {
		return result.OverQuotaRisk[i].UsagePct > result.OverQuotaRisk[j].UsagePct
	})
	sort.Slice(result.Namespaces, func(i, j int) bool {
		return !result.Namespaces[i].HasQuota && result.Namespaces[j].HasQuota
	})

	result.Recommendations = buildNSQuotaRecs(&result)
	writeJSON(w, result)
}

func buildNSQuotaRecs(r *NamespaceQuotaMapResult) []string {
	recs := []string{}
	if r.Summary.WithoutQuota > 0 {
		recs = append(recs, fmt.Sprintf("%d 个命名空间缺少 ResourceQuota，无法限制资源使用", r.Summary.WithoutQuota))
	}
	if r.Summary.AtRiskNS > 0 {
		recs = append(recs, fmt.Sprintf("%d 个命名空间资源使用率超过 80%%，有超配风险", r.Summary.AtRiskNS))
	}
	if r.Summary.TotalNamespaces-r.Summary.WithLimitRange > 0 {
		recs = append(recs, fmt.Sprintf("%d 个命名空间缺少 LimitRange，未设置默认资源限制", r.Summary.TotalNamespaces-r.Summary.WithLimitRange))
	}
	if len(recs) == 0 {
		recs = append(recs, "命名空间配额管理良好")
	}
	return recs
}
