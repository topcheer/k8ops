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

// TenantPressureResult is the multi-tenant resource pressure & quota competition audit.
type TenantPressureResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         TenantPressureSummary `json:"summary"`
	ByNamespace     []TenantPressureEntry `json:"byNamespace"`
	QuotaConflicts  []QuotaConflict       `json:"quotaConflicts"`
	Hotspots        []TenantHotspot       `json:"hotspots"`
	Recommendations []string              `json:"recommendations"`
}

// TenantPressureSummary aggregates multi-tenant pressure statistics.
type TenantPressureSummary struct {
	TotalNamespaces     int `json:"totalNamespaces"`
	NamespacesWithQuota int `json:"namespacesWithQuota"`
	NamespacesNoQuota   int `json:"namespacesNoQuota"`
	SaturatedQuotas     int `json:"saturatedQuotas"`     // quota usage > 80%
	CriticalQuotas      int `json:"criticalQuotas"`      // quota usage > 95%
	NoLimitRange        int `json:"noLimitRange"`        // namespaces without LimitRange
	UnboundedNamespaces int `json:"unboundedNamespaces"` // no quota + no LimitRange
	HealthScore         int `json:"healthScore"`
}

// TenantPressureEntry describes one namespace's resource pressure.
type TenantPressureEntry struct {
	Namespace     string                `json:"namespace"`
	HasQuota      bool                  `json:"hasQuota"`
	HasLimitRange bool                  `json:"hasLimitRange"`
	QuotaUsage    map[string]QuotaUsage `json:"quotaUsage,omitempty"`
	PodCount      int                   `json:"podCount"`
	CPURequest    string                `json:"cpuRequest"`
	MemRequest    string                `json:"memRequest"`
	RiskLevel     string                `json:"riskLevel"`
	Issue         string                `json:"issue,omitempty"`
}

// QuotaUsage shows a single resource quota usage.
type QuotaUsage struct {
	Hard    string  `json:"hard"`
	Used    string  `json:"used"`
	Percent float64 `json:"percent"`
}

// QuotaConflict describes a cross-namespace quota competition.
type QuotaConflict struct {
	Resource    string   `json:"resource"`
	Namespaces  []string `json:"namespaces"`
	TotalUsed   string   `json:"totalUsed"`
	TotalHard   string   `json:"totalHard"`
	Severity    string   `json:"severity"`
	Description string   `json:"description"`
}

// TenantHotspot identifies a namespace consuming disproportionate resources.
type TenantHotspot struct {
	Namespace string  `json:"namespace"`
	Resource  string  `json:"resource"`
	Usage     string  `json:"usage"`
	Percent   float64 `json:"percentOfCluster"`
	Severity  string  `json:"severity"`
}

// tenantPressureAuditCore performs the audit on resource quotas, limit ranges, and pods.
func tenantPressureAuditCore(
	quotas []corev1.ResourceQuota,
	limitRanges []corev1.LimitRange,
	pods []corev1.Pod,
	nodes []corev1.Node,
) TenantPressureResult {
	result := TenantPressureResult{
		ScannedAt: time.Now(),
	}

	// Build lookup maps
	quotaByNS := make(map[string][]corev1.ResourceQuota)
	for i := range quotas {
		q := &quotas[i]
		quotaByNS[q.Namespace] = append(quotaByNS[q.Namespace], *q)
	}

	limitRangeByNS := make(map[string]bool)
	for i := range limitRanges {
		limitRangeByNS[limitRanges[i].Namespace] = true
	}

	// Count pods per namespace and sum resource requests
	podCountByNS := make(map[string]int)
	cpuReqByNS := make(map[string]resource.Quantity)
	memReqByNS := make(map[string]resource.Quantity)

	for i := range pods {
		pod := &pods[i]
		ns := pod.Namespace
		podCountByNS[ns]++

		for j := range pod.Spec.Containers {
			c := &pod.Spec.Containers[j]
			if cpu := c.Resources.Requests.Cpu(); cpu != nil {
				if existing, ok := cpuReqByNS[ns]; ok {
					existing.Add(*cpu)
					cpuReqByNS[ns] = existing
				} else {
					cpuReqByNS[ns] = cpu.DeepCopy()
				}
			}
			if mem := c.Resources.Requests.Memory(); mem != nil {
				if existing, ok := memReqByNS[ns]; ok {
					existing.Add(*mem)
					memReqByNS[ns] = existing
				} else {
					memReqByNS[ns] = mem.DeepCopy()
				}
			}
		}
	}

	// Collect all unique namespaces
	allNS := make(map[string]bool)
	for ns := range quotaByNS {
		allNS[ns] = true
	}
	for ns := range limitRangeByNS {
		allNS[ns] = true
	}
	for ns := range podCountByNS {
		allNS[ns] = true
	}

	// Calculate cluster capacity
	clusterCPUCapacity := resource.Quantity{}
	clusterMemCapacity := resource.Quantity{}
	for i := range nodes {
		n := &nodes[i]
		if cpu := n.Status.Allocatable.Cpu(); cpu != nil {
			clusterCPUCapacity.Add(*cpu)
		}
		if mem := n.Status.Allocatable.Memory(); mem != nil {
			clusterMemCapacity.Add(*mem)
		}
	}

	result.Summary.TotalNamespaces = len(allNS)

	// Analyze each namespace
	for ns := range allNS {
		quotaList := quotaByNS[ns]
		hasQuota := len(quotaList) > 0
		hasLR := limitRangeByNS[ns]

		cpuReq := cpuReqByNS[ns]
		memReq := memReqByNS[ns]
		entry := TenantPressureEntry{
			Namespace:     ns,
			HasQuota:      hasQuota,
			HasLimitRange: hasLR,
			PodCount:      podCountByNS[ns],
			CPURequest:    cpuReq.String(),
			MemRequest:    memReq.String(),
		}

		risk := "low"
		var issue string

		if !hasQuota {
			result.Summary.NamespacesNoQuota++
			if !hasLR {
				result.Summary.UnboundedNamespaces++
				risk = "high"
				issue = "no ResourceQuota and no LimitRange — unbounded resource consumption"
			} else {
				risk = "medium"
				issue = "no ResourceQuota — LimitRange provides defaults but no upper bound"
			}
		} else {
			result.Summary.NamespacesWithQuota++
			// Check quota saturation
			saturatedCount := 0
			criticalCount := 0
			entry.QuotaUsage = make(map[string]QuotaUsage)

			for _, q := range quotaList {
				for resourceName, hard := range q.Status.Hard {
					used, hasUsed := q.Status.Used[resourceName]
					if !hasUsed {
						continue
					}

					percent := 0.0
					if !hard.IsZero() {
						percent = float64(used.MilliValue()) / float64(hard.MilliValue()) * 100
					}

					usage := QuotaUsage{
						Hard:    hard.String(),
						Used:    used.String(),
						Percent: percent,
					}
					key := string(resourceName)
					if existing, ok := entry.QuotaUsage[key]; ok {
						// Merge — keep the higher percentage
						if percent > existing.Percent {
							entry.QuotaUsage[key] = usage
						}
					} else {
						entry.QuotaUsage[key] = usage
					}

					if percent > 95 {
						criticalCount++
					} else if percent > 80 {
						saturatedCount++
					}
				}
			}

			if criticalCount > 0 {
				result.Summary.CriticalQuotas += criticalCount
				risk = "critical"
				issue = fmt.Sprintf("%d quota resources at >95%% utilization — scheduling will fail", criticalCount)
			} else if saturatedCount > 0 {
				result.Summary.SaturatedQuotas += saturatedCount
				risk = "high"
				issue = fmt.Sprintf("%d quota resources at >80%% utilization — approaching limit", saturatedCount)
			}
		}

		if !hasLR && hasQuota {
			result.Summary.NoLimitRange++
			if risk == "low" {
				risk = "medium"
				if issue == "" {
					issue = "has ResourceQuota but no LimitRange — pods without explicit requests get defaults"
				}
			}
		}

		entry.RiskLevel = risk
		entry.Issue = issue

		result.ByNamespace = append(result.ByNamespace, entry)

		// Track hotspots
		if clusterCPUCapacity.MilliValue() > 0 {
			nsCPU := cpuReqByNS[ns]
			nsCPUPct := float64(nsCPU.MilliValue()) / float64(clusterCPUCapacity.MilliValue()) * 100
			if nsCPUPct > 50 {
				result.Hotspots = append(result.Hotspots, TenantHotspot{
					Namespace: ns,
					Resource:  "cpu",
					Usage:     nsCPU.String(),
					Percent:   nsCPUPct,
					Severity:  "high",
				})
			} else if nsCPUPct > 30 {
				result.Hotspots = append(result.Hotspots, TenantHotspot{
					Namespace: ns,
					Resource:  "cpu",
					Usage:     nsCPU.String(),
					Percent:   nsCPUPct,
					Severity:  "medium",
				})
			}
		}
	}

	// Sort by risk level (critical first)
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return riskOrder[result.ByNamespace[i].RiskLevel] < riskOrder[result.ByNamespace[j].RiskLevel]
	})

	// Sort hotspots by percentage descending
	sort.Slice(result.Hotspots, func(i, j int) bool {
		return result.Hotspots[i].Percent > result.Hotspots[j].Percent
	})

	result.Summary.HealthScore = tenantPressureScore(result.Summary)
	result.Recommendations = tenantPressureRecommendations(result.Summary)

	return result
}

// tenantPressureScore calculates health score.
func tenantPressureScore(s TenantPressureSummary) int {
	base := 100
	if s.TotalNamespaces == 0 {
		return 100
	}
	// Unbounded namespaces are the most risky
	base -= s.UnboundedNamespaces * 10
	// Critical quotas
	base -= s.CriticalQuotas * 8
	// Saturated quotas
	base -= s.SaturatedQuotas * 4
	// No limit range
	base -= s.NoLimitRange * 2

	if base < 0 {
		base = 0
	}
	return base
}

// tenantPressureRecommendations generates recommendations.
func tenantPressureRecommendations(s TenantPressureSummary) []string {
	var recs []string
	if s.UnboundedNamespaces > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have no ResourceQuota and no LimitRange — add both to prevent unbounded resource consumption", s.UnboundedNamespaces))
	}
	if s.CriticalQuotas > 0 {
		recs = append(recs, fmt.Sprintf("%d quota resources are at >95%% utilization — increase quota or optimize resource usage in affected namespaces", s.CriticalQuotas))
	}
	if s.SaturatedQuotas > 0 {
		recs = append(recs, fmt.Sprintf("%d quota resources are at >80%% utilization — plan capacity expansion before scheduling fails", s.SaturatedQuotas))
	}
	if s.NamespacesNoQuota > 0 && s.UnboundedNamespaces == 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces lack ResourceQuota — consider adding quotas for better resource governance", s.NamespacesNoQuota))
	}
	if s.CriticalQuotas == 0 && s.SaturatedQuotas == 0 && s.UnboundedNamespaces == 0 {
		recs = append(recs, "multi-tenant resource pressure is well managed — no critical issues detected")
	}
	return recs
}

// handleTenantPressure audits multi-tenant resource pressure and quota competition.
// GET /api/scalability/tenant-pressure
func (s *Server) handleTenantPressure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})

	result := tenantPressureAuditCore(quotas.Items, limitRanges.Items, pods.Items, nodes.Items)
	writeJSON(w, result)
}

// Suppress unused import (strings used in future expansion)
var _ = strings.Contains
