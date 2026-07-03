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

// QuotaStatus describes the utilization level of a resource quota.
type QuotaStatus string

const (
	QuotaOK       QuotaStatus = "ok"       // under 70% usage
	QuotaWarning  QuotaStatus = "warning"  // 70-85% usage
	QuotaCritical QuotaStatus = "critical" // 85-100% usage
	QuotaExceeded QuotaStatus = "exceeded" // over 100% usage
	QuotaNoLimit  QuotaStatus = "no-limit" // no quota set
)

// QuotaItem represents a single resource within a namespace quota.
type QuotaItem struct {
	Resource     string      `json:"resource"`     // e.g. "requests.cpu", "limits.memory"
	Hard         string      `json:"hard"`         // hard limit
	Used         string      `json:"used"`         // current usage
	UsagePercent float64     `json:"usagePercent"` // percentage used
	Status       QuotaStatus `json:"status"`
}

// NamespaceQuota summarizes quota utilization for a single namespace.
type NamespaceQuota struct {
	Namespace     string      `json:"namespace"`
	HasQuota      bool        `json:"hasQuota"`
	HasLimitRange bool        `json:"hasLimitRange"`
	Items         []QuotaItem `json:"items,omitempty"`
	LimitRanges   []LimitItem `json:"limitRanges,omitempty"`
	WorstStatus   QuotaStatus `json:"worstStatus"`
	IssueCount    int         `json:"issueCount"`
	ExceededCount int         `json:"exceededCount"`
}

// LimitItem represents a single limit range entry.
type LimitItem struct {
	Type        string `json:"type"`     // Container, Pod, PVC
	Resource    string `json:"resource"` // cpu, memory
	Default     string `json:"default,omitempty"`
	DefaultReq  string `json:"defaultRequest,omitempty"`
	Min         string `json:"min,omitempty"`
	Max         string `json:"max,omitempty"`
	MaxLimitReq string `json:"maxLimitRequestRatio,omitempty"`
}

// QuotaReport is the full scan output.
type QuotaReport struct {
	ScannedAt    time.Time        `json:"scannedAt"`
	Summary      QuotaSummary     `json:"summary"`
	Namespaces   []NamespaceQuota `json:"namespaces"`
	TopOffenders []NamespaceQuota `json:"topOffenders"`
}

// QuotaSummary aggregates quota statistics.
type QuotaSummary struct {
	TotalNamespaces   int            `json:"totalNamespaces"`
	WithQuota         int            `json:"withQuota"`
	WithoutQuota      int            `json:"withoutQuota"`
	WithLimitRange    int            `json:"withLimitRange"`
	ExceededResources int            `json:"exceededResources"`
	CriticalResources int            `json:"criticalResources"`
	ByStatus          map[string]int `json:"byStatus"`
}

// handleQuotaMonitor scans all namespaces for ResourceQuota and LimitRange utilization.
// GET /api/resources/quota
func (s *Server) handleQuotaMonitor(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	nsFilter := r.URL.Query().Get("namespace")
	ctx := r.Context()

	// List all namespaces
	nsList, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List all resource quotas (cluster-wide, filter later)
	rqList, err := rc.clientset.CoreV1().ResourceQuotas(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// List all limit ranges
	lrList, err := rc.clientset.CoreV1().LimitRanges(nsFilter).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build quota index: namespace -> []ResourceQuota
	quotaByNs := make(map[string][]corev1.ResourceQuota)
	for i := range rqList.Items {
		rq := &rqList.Items[i]
		quotaByNs[rq.Namespace] = append(quotaByNs[rq.Namespace], *rq)
	}

	// Build limit range index: namespace -> []LimitRange
	lrByNs := make(map[string][]corev1.LimitRange)
	for i := range lrList.Items {
		lr := &lrList.Items[i]
		lrByNs[lr.Namespace] = append(lrByNs[lr.Namespace], *lr)
	}

	var nsQuotas []NamespaceQuota
	summary := QuotaSummary{ByStatus: make(map[string]int)}

	for i := range nsList.Items {
		ns := &nsList.Items[i]
		if nsFilter != "" && ns.Name != nsFilter {
			continue
		}

		nq := analyzeNamespaceQuota(ns.Name, quotaByNs[ns.Name], lrByNs[ns.Name])

		summary.TotalNamespaces++
		if nq.HasQuota {
			summary.WithQuota++
		} else {
			summary.WithoutQuota++
		}
		if nq.HasLimitRange {
			summary.WithLimitRange++
		}
		for _, item := range nq.Items {
			summary.ByStatus[string(item.Status)]++
			if item.Status == QuotaExceeded {
				summary.ExceededResources++
			}
			if item.Status == QuotaCritical {
				summary.CriticalResources++
			}
		}

		nsQuotas = append(nsQuotas, nq)
	}

	// Sort: worst status first, then by exceeded count
	sort.Slice(nsQuotas, func(i, j int) bool {
		ri := quotaStatusRank(nsQuotas[i].WorstStatus)
		rj := quotaStatusRank(nsQuotas[j].WorstStatus)
		if ri != rj {
			return ri < rj
		}
		return nsQuotas[i].ExceededCount > nsQuotas[j].ExceededCount
	})

	// Build top offenders (namespaces with critical or exceeded resources)
	var offenders []NamespaceQuota
	for _, nq := range nsQuotas {
		if nq.WorstStatus == QuotaExceeded || nq.WorstStatus == QuotaCritical {
			offenders = append(offenders, nq)
		}
	}

	writeJSON(w, QuotaReport{
		ScannedAt:    time.Now(),
		Summary:      summary,
		Namespaces:   nsQuotas,
		TopOffenders: offenders,
	})
}

// analyzeNamespaceQuota evaluates quota utilization for a single namespace.
func analyzeNamespaceQuota(nsName string, quotas []corev1.ResourceQuota, limitRanges []corev1.LimitRange) NamespaceQuota {
	nq := NamespaceQuota{
		Namespace:   nsName,
		WorstStatus: QuotaNoLimit,
	}

	if len(quotas) == 0 && len(limitRanges) == 0 {
		nq.WorstStatus = QuotaNoLimit
		return nq
	}

	nq.HasQuota = len(quotas) > 0
	nq.HasLimitRange = len(limitRanges) > 0

	// Process ResourceQuotas
	for _, rq := range quotas {
		// Process hard limits vs used
		for resourceName, hard := range rq.Status.Hard {
			used := rq.Status.Used[resourceName]

			hardStr := hard.String()
			usedStr := used.String()

			pct := 0.0
			if hard.IsZero() {
				pct = 0
			} else {
				// Calculate percentage based on resource type
				pct = calculateUsagePercent(resourceName, used, hard)
			}

			status := classifyQuotaStatus(pct)

			item := QuotaItem{
				Resource:     string(resourceName),
				Hard:         hardStr,
				Used:         usedStr,
				UsagePercent: roundTo(pct, 1),
				Status:       status,
			}

			nq.Items = append(nq.Items, item)

			if status == QuotaExceeded {
				nq.ExceededCount++
			}
			if status == QuotaExceeded || status == QuotaCritical || status == QuotaWarning {
				nq.IssueCount++
			}

			// Track worst status
			if quotaStatusRank(status) < quotaStatusRank(nq.WorstStatus) {
				nq.WorstStatus = status
			}
		}
	}

	// Sort items by usage percent descending
	sort.Slice(nq.Items, func(i, j int) bool {
		return nq.Items[i].UsagePercent > nq.Items[j].UsagePercent
	})

	// Process LimitRanges
	for _, lr := range limitRanges {
		for _, limit := range lr.Spec.Limits {
			li := LimitItem{
				Type: string(limit.Type),
			}

			for res, val := range limit.Default {
				li.Resource = string(res)
				li.Default = val.String()
			}
			for res, val := range limit.DefaultRequest {
				li.Resource = string(res)
				li.DefaultReq = val.String()
			}
			for res, val := range limit.Min {
				li.Resource = string(res)
				li.Min = val.String()
			}
			for res, val := range limit.Max {
				li.Resource = string(res)
				li.Max = val.String()
			}
			for res, val := range limit.MaxLimitRequestRatio {
				li.Resource = string(res)
				li.MaxLimitReq = val.String()
			}

			if li.Resource != "" {
				nq.LimitRanges = append(nq.LimitRanges, li)
			}
		}
	}

	return nq
}

// calculateUsagePercent computes the percentage of quota used for a given resource.
func calculateUsagePercent(resourceName corev1.ResourceName, used, hard resource.Quantity) float64 {
	rn := string(resourceName)

	// CPU resources use millicores
	if strings.HasSuffix(rn, ".cpu") {
		usedVal := used.MilliValue()
		hardVal := hard.MilliValue()
		if hardVal == 0 {
			return 0
		}
		return float64(usedVal) / float64(hardVal) * 100
	}

	// Count-based resources (pods, configmaps, secrets, services, etc.)
	if strings.HasPrefix(rn, "count.") || strings.HasPrefix(rn, "requests.") || strings.HasPrefix(rn, "limits.") {
		usedVal := used.Value()
		hardVal := hard.Value()
		if hardVal == 0 {
			return 0
		}
		return float64(usedVal) / float64(hardVal) * 100
	}

	// Storage-based resources (bytes)
	if strings.Contains(rn, "storage") || strings.Contains(rn, "ephemeral") {
		usedVal := used.Value()
		hardVal := hard.Value()
		if hardVal == 0 {
			return 0
		}
		return float64(usedVal) / float64(hardVal) * 100
	}

	// Default: use MilliValue for safety
	usedVal := used.MilliValue()
	hardVal := hard.MilliValue()
	if hardVal == 0 {
		return 0
	}
	return float64(usedVal) / float64(hardVal) * 100
}

// classifyQuotaStatus determines the status based on usage percentage.
func classifyQuotaStatus(pct float64) QuotaStatus {
	if pct > 100 {
		return QuotaExceeded
	}
	if pct >= 85 {
		return QuotaCritical
	}
	if pct >= 70 {
		return QuotaWarning
	}
	return QuotaOK
}

// quotaStatusRank returns sort priority (lower = more problematic).
func quotaStatusRank(status QuotaStatus) int {
	switch status {
	case QuotaExceeded:
		return 0
	case QuotaCritical:
		return 1
	case QuotaWarning:
		return 2
	case QuotaOK:
		return 3
	case QuotaNoLimit:
		return 4
	default:
		return 5
	}
}

// quotaReportFormatNumber formats a quantity string for display.
func quotaReportFormatNumber(q string) string {
	return q
}

// summarizeQuotaIssues returns a human-readable summary of quota issues.
func summarizeQuotaIssues(items []QuotaItem) []string {
	var issues []string
	for _, item := range items {
		if item.Status == QuotaExceeded {
			issues = append(issues, fmt.Sprintf("%s exceeded: %s/%s (%.1f%%)", item.Resource, item.Used, item.Hard, item.UsagePercent))
		} else if item.Status == QuotaCritical {
			issues = append(issues, fmt.Sprintf("%s near limit: %s/%s (%.1f%%)", item.Resource, item.Used, item.Hard, item.UsagePercent))
		}
	}
	return issues
}
