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

// QuotaSecurityResult is the resource quota & limit range security audit.
type QuotaSecurityResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         QuotaSecSummary      `json:"summary"`
	ByNamespace     []QuotaSecNSStat     `json:"byNamespace"`
	UnprotectedNS   []QuotaSecNamespace  `json:"unprotectedNamespaces"`
	LimitRangeGaps  []QuotaSecNamespace  `json:"limitRangeGaps"`
	QuotaPressure   []QuotaPressureEntry `json:"quotaPressure"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// QuotaSecSummary aggregates resource quota security statistics.
type QuotaSecSummary struct {
	TotalNamespaces       int `json:"totalNamespaces"`
	WithResourceQuota     int `json:"withResourceQuota"`
	WithLimitRange        int `json:"withLimitRange"`
	UnprotectedNamespaces int `json:"unprotectedNamespaces"` // no quota at all
	NoLimitRange          int `json:"noLimitRange"`          // no limit range
	CPUPressure           int `json:"cpuPressure"`           // quota usage > 80%
	MemoryPressure        int `json:"memoryPressure"`        // quota usage > 80%
	PodPressure           int `json:"podPressure"`           // pod count near quota limit
	SystemNamespaces      int `json:"systemNamespaces"`      // kube-system etc (expected to have quotas)
}

// QuotaSecNSStat shows quota & limit range status per namespace.
type QuotaSecNSStat struct {
	Namespace        string `json:"namespace"`
	HasResourceQuota bool   `json:"hasResourceQuota"`
	HasLimitRange    bool   `json:"hasLimitRange"`
	QuotaCount       int    `json:"quotaCount"`
	LimitRangeCount  int    `json:"limitRangeCount"`
	CPUUsed          int    `json:"cpuUsedPercent"`
	MemoryUsed       int    `json:"memoryUsedPercent"`
	PodUsed          int    `json:"podUsedPercent"`
	RiskLevel        string `json:"riskLevel"`
}

// QuotaSecNamespace describes an unprotected namespace.
type QuotaSecNamespace struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	Reason    string `json:"reason"`
	Severity  string `json:"severity"`
}

// QuotaPressureEntry describes a namespace under quota pressure.
type QuotaPressureEntry struct {
	Namespace   string `json:"namespace"`
	Resource    string `json:"resource"`
	UsedPercent int    `json:"usedPercent"`
	HardLimit   string `json:"hardLimit"`
	Used        string `json:"used"`
	Severity    string `json:"severity"`
}

// quotaSecurityAuditCore performs the audit on namespaces, quotas, limit ranges, and pods (testable).
func quotaSecurityAuditCore(
	namespaces []corev1.Namespace,
	quotas []corev1.ResourceQuota,
	limitRanges []corev1.LimitRange,
	pods []corev1.Pod,
) QuotaSecurityResult {
	result := QuotaSecurityResult{
		ScannedAt: time.Now(),
	}

	// System namespaces that should have quotas
	systemNS := map[string]bool{
		"kube-system":       true,
		"kube-public":       true,
		"kube-node-lease":   true,
		"k8ops-system":      true,
		"gatekeeper-system": true,
		"istio-system":      true,
		"monitoring":        true,
		"ingress-nginx":     true,
		"cert-manager":      true,
	}

	// Build maps
	quotaByNS := make(map[string][]corev1.ResourceQuota)
	for i := range quotas {
		q := &quotas[i]
		quotaByNS[q.Namespace] = append(quotaByNS[q.Namespace], *q)
	}

	limitRangeByNS := make(map[string][]corev1.LimitRange)
	for i := range limitRanges {
		lr := &limitRanges[i]
		limitRangeByNS[lr.Namespace] = append(limitRangeByNS[lr.Namespace], *lr)
	}

	podCountByNS := make(map[string]int)
	for i := range pods {
		podCountByNS[pods[i].Namespace]++
	}

	// Analyze each namespace
	for i := range namespaces {
		ns := &namespaces[i]
		nsName := ns.Name

		// Skip terminating namespaces
		if ns.Status.Phase == corev1.NamespaceTerminating {
			continue
		}

		result.Summary.TotalNamespaces++
		_, isSystem := systemNS[nsName]
		if isSystem {
			result.Summary.SystemNamespaces++
		}

		nsQuotas := quotaByNS[nsName]
		nsLimitRanges := limitRangeByNS[nsName]

		stat := QuotaSecNSStat{
			Namespace:        nsName,
			HasResourceQuota: len(nsQuotas) > 0,
			HasLimitRange:    len(nsLimitRanges) > 0,
			QuotaCount:       len(nsQuotas),
			LimitRangeCount:  len(nsLimitRanges),
			RiskLevel:        "low",
		}

		if len(nsQuotas) > 0 {
			result.Summary.WithResourceQuota++
		} else {
			result.Summary.UnprotectedNamespaces++
			stat.RiskLevel = "high"
			result.UnprotectedNS = append(result.UnprotectedNS, QuotaSecNamespace{
				Namespace: nsName,
				PodCount:  podCountByNS[nsName],
				Reason:    "no ResourceQuota — pods can consume unlimited cluster resources (DoS risk)",
				Severity:  "high",
			})
		}

		if len(nsLimitRanges) > 0 {
			result.Summary.WithLimitRange++
		} else {
			result.Summary.NoLimitRange++
			if stat.RiskLevel == "low" {
				stat.RiskLevel = "medium"
			}
			result.LimitRangeGaps = append(result.LimitRangeGaps, QuotaSecNamespace{
				Namespace: nsName,
				PodCount:  podCountByNS[nsName],
				Reason:    "no LimitRange — individual pods can request unbounded resources",
				Severity:  "medium",
			})
		}

		// Calculate quota pressure from the first quota that has hard limits
		for _, q := range nsQuotas {
			hard := q.Status.Hard
			used := q.Status.Used

			// CPU
			if hard.Cpu() != nil && !hard.Cpu().IsZero() {
				cpuUsed := calcPercent(used.Cpu(), hard.Cpu())
				stat.CPUUsed = cpuUsed
				if cpuUsed >= 80 {
					result.Summary.CPUPressure++
					result.QuotaPressure = append(result.QuotaPressure, QuotaPressureEntry{
						Namespace:   nsName,
						Resource:    "cpu",
						UsedPercent: cpuUsed,
						HardLimit:   hard.Cpu().String(),
						Used:        used.Cpu().String(),
						Severity:    "high",
					})
				}
			}

			// Memory
			if hard.Memory() != nil && !hard.Memory().IsZero() {
				memUsed := calcPercent(used.Memory(), hard.Memory())
				stat.MemoryUsed = memUsed
				if memUsed >= 80 {
					result.Summary.MemoryPressure++
					result.QuotaPressure = append(result.QuotaPressure, QuotaPressureEntry{
						Namespace:   nsName,
						Resource:    "memory",
						UsedPercent: memUsed,
						HardLimit:   hard.Memory().String(),
						Used:        used.Memory().String(),
						Severity:    "high",
					})
				}
			}

			// Pods
			if hard.Pods() != nil && !hard.Pods().IsZero() {
				podHard := hard.Pods()
				podUsedVal := used.Pods()
				if podHard != nil && podUsedVal != nil {
					hardVal := podHard.Value()
					usedVal := podUsedVal.Value()
					if hardVal > 0 {
						podUsed := int((usedVal * 100) / hardVal)
						stat.PodUsed = podUsed
						if podUsed >= 80 {
							result.Summary.PodPressure++
							result.QuotaPressure = append(result.QuotaPressure, QuotaPressureEntry{
								Namespace:   nsName,
								Resource:    "pods",
								UsedPercent: podUsed,
								HardLimit:   fmt.Sprintf("%d", hardVal),
								Used:        fmt.Sprintf("%d", usedVal),
								Severity:    "medium",
							})
						}
					}
				}
			}
		}

		result.ByNamespace = append(result.ByNamespace, stat)
	}

	// Sort by risk level (high first)
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskLevel > result.ByNamespace[j].RiskLevel
	})

	sort.Slice(result.UnprotectedNS, func(i, j int) bool {
		return result.UnprotectedNS[i].Severity > result.UnprotectedNS[j].Severity
	})

	sort.Slice(result.QuotaPressure, func(i, j int) bool {
		return result.QuotaPressure[i].UsedPercent > result.QuotaPressure[j].UsedPercent
	})

	result.HealthScore = quotaSecScore(result.Summary)
	result.Recommendations = quotaSecRecommendations(result.Summary)

	return result
}

// calcPercent calculates the percentage of used vs hard limit.
func calcPercent(used, hard *resource.Quantity) int {
	if hard == nil || hard.IsZero() {
		return 0
	}
	if used == nil {
		return 0
	}
	usedMillicores := used.MilliValue()
	hardMillicores := hard.MilliValue()
	if hardMillicores == 0 {
		return 0
	}
	pct := int((usedMillicores * 100) / hardMillicores)
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct
}

// quotaSecScore calculates the health score.
func quotaSecScore(s QuotaSecSummary) int {
	if s.TotalNamespaces == 0 {
		return 100
	}
	base := 100
	// No quota is worst (high risk)
	base -= (s.UnprotectedNamespaces * 10)
	// No limit range is medium risk
	base -= (s.NoLimitRange * 3)
	// Quota pressure
	base -= (s.CPUPressure * 5)
	base -= (s.MemoryPressure * 5)
	base -= (s.PodPressure * 2)
	if base < 0 {
		base = 0
	}
	return base
}

// quotaSecRecommendations generates actionable recommendations.
func quotaSecRecommendations(s QuotaSecSummary) []string {
	var recs []string
	if s.UnprotectedNamespaces > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) have no ResourceQuota — add quotas to prevent resource exhaustion attacks (DoS)", s.UnprotectedNamespaces))
	}
	if s.NoLimitRange > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) have no LimitRange — add default resource limits to prevent unbounded pod requests", s.NoLimitRange))
	}
	if s.CPUPressure > 0 || s.MemoryPressure > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) under CPU quota pressure, %d under memory pressure — review quota limits or scale resources", s.CPUPressure, s.MemoryPressure))
	}
	if s.PodPressure > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) near pod quota limit — increase pod quota or clean up stale workloads", s.PodPressure))
	}
	if s.UnprotectedNamespaces == 0 && s.NoLimitRange == 0 {
		recs = append(recs, "all namespaces have ResourceQuotas and LimitRanges — resource exhaustion protection is complete")
	}
	return recs
}

// handleQuotaSecurity audits resource quota and limit range security posture.
// GET /api/security/quota-security
func (s *Server) handleQuotaSecurity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	namespaces, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	quotas, err := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	limitRanges, err := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// Pods are optional for the audit — proceed with empty list
		pods = &corev1.PodList{}
	}

	// Suppress unused variable warning for strings
	_ = strings.TrimSpace

	result := quotaSecurityAuditCore(namespaces.Items, quotas.Items, limitRanges.Items, pods.Items)
	writeJSON(w, result)
}
