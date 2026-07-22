package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.15 — Product Dimension (Round 5)
// 1. LimitRange Audit
// 2. Tenant Isolation Score
// 3. Resource Share Ratio
// ============================================================

// ---------------------------------------------------------------
// 1. LimitRange Audit — default resource limits coverage
// ---------------------------------------------------------------

type LimitRangeResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         LimitRangeSummary   `json:"summary"`
	ByNamespace     []LimitRangeNSEntry `json:"byNamespace"`
	MissingLimits   []string            `json:"missingLimits"`
	Recommendations []string            `json:"recommendations"`
}

type LimitRangeSummary struct {
	TotalNamespaces   int `json:"totalNamespaces"`
	WithLimitRange    int `json:"withLimitRange"`
	WithoutLimitRange int `json:"withoutLimitRange"`
	WithDefaultCPU    int `json:"withDefaultCPU"`
	WithDefaultMem    int `json:"withDefaultMem"`
	WithMaxCPU        int `json:"withMaxCPU"`
	ContainersNoLimit int `json:"containersWithoutLimit"`
}

type LimitRangeNSEntry struct {
	Namespace     string `json:"namespace"`
	HasLimitRange bool   `json:"hasLimitRange"`
	Workloads     int    `json:"workloads"`
	DefaultCPU    string `json:"defaultCPU,omitempty"`
	DefaultMem    string `json:"defaultMem,omitempty"`
	MaxCPU        string `json:"maxCPU,omitempty"`
	RiskLevel     string `json:"riskLevel"`
}

func (s *Server) handleLimitRangeAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := LimitRangeResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Count workloads per namespace
	nsWorkloads := map[string]int{}
	for _, dep := range deps.Items {
		if !isSystemNamespace(dep.Namespace) {
			nsWorkloads[dep.Namespace]++
		}
	}

	// Build limit range map
	nsLimitRange := map[string]*corev1.LimitRange{}
	for _, lr := range limitRanges.Items {
		nsLimitRange[lr.Namespace] = &lr
	}

	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++
		entry := LimitRangeNSEntry{
			Namespace: ns.Name,
			Workloads: nsWorkloads[ns.Name],
		}

		lr := nsLimitRange[ns.Name]
		if lr != nil {
			entry.HasLimitRange = true
			result.Summary.WithLimitRange++
			for _, item := range lr.Spec.Limits {
				if item.Type == corev1.LimitTypeContainer {
					if d, ok := item.Default["cpu"]; ok {
						entry.DefaultCPU = d.String()
						result.Summary.WithDefaultCPU++
					}
					if d, ok := item.Default["memory"]; ok {
						entry.DefaultMem = d.String()
						result.Summary.WithDefaultMem++
					}
					if m, ok := item.Max["cpu"]; ok {
						entry.MaxCPU = m.String()
						result.Summary.WithMaxCPU++
					}
				}
			}
			entry.RiskLevel = "low"
		} else {
			entry.HasLimitRange = false
			result.Summary.WithoutLimitRange++
			if entry.Workloads > 0 {
				entry.RiskLevel = "high"
				result.Summary.ContainersNoLimit += entry.Workloads
				result.MissingLimits = append(result.MissingLimits, ns.Name)
			} else {
				entry.RiskLevel = "low"
			}
		}

		result.ByNamespace = append(result.ByNamespace, entry)
	}

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskLevel == "high" && result.ByNamespace[j].RiskLevel != "high"
	})

	// Score
	if result.Summary.TotalNamespaces > 0 {
		coveredPct := result.Summary.WithLimitRange * 100 / result.Summary.TotalNamespaces
		result.HealthScore = coveredPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildLimitRangeRecs1915(&result)
	writeJSON(w, result)
}

func buildLimitRangeRecs1915(r *LimitRangeResult) []string {
	recs := []string{fmt.Sprintf("LimitRange: %d/%d namespaces covered (%d without, %d workloads unprotected)",
		r.Summary.WithLimitRange, r.Summary.TotalNamespaces,
		r.Summary.WithoutLimitRange, r.Summary.ContainersNoLimit)}
	if r.Summary.WithoutLimitRange > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces without LimitRange - containers can consume unlimited resources", r.Summary.WithoutLimitRange))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Tenant Isolation Score — multi-tenancy assessment
// ---------------------------------------------------------------

type TenantIsoResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         TenantIsoSummary  `json:"summary"`
	ByNamespace     []TenantIsoNS     `json:"byNamespace"`
	Violations      []TenantViolation `json:"violations"`
	Recommendations []string          `json:"recommendations"`
}

type TenantIsoSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithNetPolicy   int `json:"withNetworkPolicy"`
	WithQuota       int `json:"withResourceQuota"`
	WithLimitRange  int `json:"withLimitRange"`
	WithRBAC        int `json:"withDedicatedRBAC"`
	IsolatedNS      int `json:"fullyIsolated"`
	SharedNS        int `json:"sharedNamespace"`
	IsoScore        int `json:"isolationScore"`
}

type TenantIsoNS struct {
	Namespace    string `json:"namespace"`
	HasNetPolicy bool   `json:"hasNetworkPolicy"`
	HasQuota     bool   `json:"hasResourceQuota"`
	HasLimit     bool   `json:"hasLimitRange"`
	HasRBAC      bool   `json:"hasDedicatedRBAC"`
	Score        int    `json:"score"`
	Workloads    int    `json:"workloads"`
	RiskLevel    string `json:"riskLevel"`
}

type TenantViolation struct {
	Namespace string `json:"namespace"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
}

func (s *Server) handleTenantIsolation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := TenantIsoResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	roleBindings, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})

	nsNetPol := map[string]bool{}
	for _, np := range netpols.Items {
		if !isSystemNamespace(np.Namespace) {
			nsNetPol[np.Namespace] = true
		}
	}
	nsQuota := map[string]bool{}
	for _, q := range quotas.Items {
		if !isSystemNamespace(q.Namespace) {
			nsQuota[q.Namespace] = true
		}
	}
	nsLimit := map[string]bool{}
	for _, lr := range limitRanges.Items {
		if !isSystemNamespace(lr.Namespace) {
			nsLimit[lr.Namespace] = true
		}
	}
	nsRBAC := map[string]bool{}
	for _, rb := range roleBindings.Items {
		if !isSystemNamespace(rb.Namespace) && rb.RoleRef.Kind == "Role" {
			nsRBAC[rb.Namespace] = true
		}
	}

	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		entry := TenantIsoNS{
			Namespace:    ns.Name,
			HasNetPolicy: nsNetPol[ns.Name],
			HasQuota:     nsQuota[ns.Name],
			HasLimit:     nsLimit[ns.Name],
			HasRBAC:      nsRBAC[ns.Name],
		}

		score := 0
		if entry.HasNetPolicy {
			score += 25
		}
		if entry.HasQuota {
			score += 25
		}
		if entry.HasLimit {
			score += 25
		}
		if entry.HasRBAC {
			score += 25
		}
		entry.Score = score

		switch {
		case score >= 75:
			entry.RiskLevel = "low"
			result.Summary.IsolatedNS++
		case score >= 50:
			entry.RiskLevel = "medium"
		case score >= 25:
			entry.RiskLevel = "high"
			result.Summary.SharedNS++
		default:
			entry.RiskLevel = "critical"
			result.Summary.SharedNS++
			if entry.Workloads > 0 {
				result.Violations = append(result.Violations, TenantViolation{
					Namespace: ns.Name, Severity: "critical",
					Violation: "zero isolation controls (no NetPolicy, Quota, LimitRange, or RBAC)",
				})
			}
		}

		if entry.HasNetPolicy {
			result.Summary.WithNetPolicy++
		}
		if entry.HasQuota {
			result.Summary.WithQuota++
		}
		if entry.HasLimit {
			result.Summary.WithLimitRange++
		}
		if entry.HasRBAC {
			result.Summary.WithRBAC++
		}

		result.ByNamespace = append(result.ByNamespace, entry)
	}

	// Sort by score ascending (weakest isolation first)

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Score < result.ByNamespace[j].Score
	})

	// Score
	if result.Summary.TotalNamespaces > 0 {
		result.Summary.IsoScore = result.Summary.IsolatedNS * 100 / result.Summary.TotalNamespaces
		result.HealthScore = result.Summary.IsoScore
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildTenantIsoRecs1915(&result)
	writeJSON(w, result)
}

func buildTenantIsoRecs1915(r *TenantIsoResult) []string {
	recs := []string{fmt.Sprintf("Tenant isolation: %d/%d namespaces isolated (%d%%), %d shared (NetPol:%d, Quota:%d, LimitRange:%d, RBAC:%d)",
		r.Summary.IsolatedNS, r.Summary.TotalNamespaces, r.Summary.IsoScore,
		r.Summary.SharedNS, r.Summary.WithNetPolicy, r.Summary.WithQuota,
		r.Summary.WithLimitRange, r.Summary.WithRBAC)}
	if r.Summary.SharedNS > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces with weak isolation - add NetworkPolicy and ResourceQuota", r.Summary.SharedNS))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Resource Share Ratio — namespace resource fairness
// ---------------------------------------------------------------

type ResourceShareResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ResourceShareSummary `json:"summary"`
	ByNamespace     []ResourceShareNS    `json:"byNamespace"`
	Imbalance       []ResourceShareNS    `json:"imbalanceEntries"`
	Recommendations []string             `json:"recommendations"`
}

type ResourceShareSummary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	TotalCPUm       int `json:"totalCPUm"`
	TotalMemMB      int `json:"totalMemMB"`
	AvgCPUmPerNS    int `json:"avgCPUmPerNS"`
	MaxCPUmNS       int `json:"maxCPUmNS"`
	MinCPUmNS       int `json:"minCPUmNS"`
	FairnessScore   int `json:"fairnessScore"`
	TopConsumerPct  int `json:"topConsumerPct"`
}

type ResourceShareNS struct {
	Namespace string  `json:"namespace"`
	CPUm      int     `json:"cpuMilli"`
	MemMB     int     `json:"memMB"`
	SharePct  float64 `json:"sharePct"`
	Workloads int     `json:"workloads"`
	RiskLevel string  `json:"riskLevel"`
}

func (s *Server) handleResourceShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ResourceShareResult{ScannedAt: time.Now()}

	nsData := map[string]*ResourceShareNS{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		nsE, ok := nsData[dep.Namespace]
		if !ok {
			nsE = &ResourceShareNS{Namespace: dep.Namespace}
			nsData[dep.Namespace] = nsE
		}
		nsE.Workloads++

		for _, c := range dep.Spec.Template.Spec.Containers {
			if qty, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				m := int(qty.MilliValue())
				nsE.CPUm += m
				result.Summary.TotalCPUm += m
			}
			if qty, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				mb := int(qty.Value() / (1024 * 1024))
				nsE.MemMB += mb
				result.Summary.TotalMemMB += mb
			}
		}
	}

	result.Summary.TotalNamespaces = len(nsData)
	if result.Summary.TotalNamespaces > 0 {
		result.Summary.AvgCPUmPerNS = result.Summary.TotalCPUm / result.Summary.TotalNamespaces
	}

	for _, ns := range nsData {
		if result.Summary.TotalCPUm > 0 {
			ns.SharePct = float64(ns.CPUm) * 100 / float64(result.Summary.TotalCPUm)
		}
		if ns.CPUm > result.Summary.MaxCPUmNS {
			result.Summary.MaxCPUmNS = ns.CPUm
		}
		if result.Summary.MinCPUmNS == 0 || (ns.CPUm > 0 && ns.CPUm < result.Summary.MinCPUmNS) {
			result.Summary.MinCPUmNS = ns.CPUm
		}

		// Risk assessment
		if ns.SharePct > 50 {
			ns.RiskLevel = "high"
			result.Imbalance = append(result.Imbalance, *ns)
		} else if ns.SharePct > 30 {
			ns.RiskLevel = "medium"
		} else {
			ns.RiskLevel = "low"
		}

		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	// Top consumer percentage
	if len(result.ByNamespace) > 0 {
		sort.Slice(result.ByNamespace, func(i, j int) bool {
			return result.ByNamespace[i].CPUm > result.ByNamespace[j].CPUm
		})
		result.Summary.TopConsumerPct = int(result.ByNamespace[0].SharePct)
	}

	// Fairness score: lower max share = more fair
	if result.Summary.TopConsumerPct > 0 {
		if result.Summary.TopConsumerPct > 80 {
			result.Summary.FairnessScore = 10
		} else if result.Summary.TopConsumerPct > 60 {
			result.Summary.FairnessScore = 30
		} else if result.Summary.TopConsumerPct > 40 {
			result.Summary.FairnessScore = 60
		} else {
			result.Summary.FairnessScore = 90
		}
	} else {
		result.Summary.FairnessScore = 100
	}

	result.HealthScore = result.Summary.FairnessScore
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildResourceShareRecs1915(&result)
	writeJSON(w, result)
}

func buildResourceShareRecs1915(r *ResourceShareResult) []string {
	recs := []string{fmt.Sprintf("Resource fairness: %d namespaces, total %dm CPU, top consumer %d%%, fairness score %d/100",
		r.Summary.TotalNamespaces, r.Summary.TotalCPUm,
		r.Summary.TopConsumerPct, r.Summary.FairnessScore)}
	if r.Summary.TopConsumerPct > 50 {
		recs = append(recs, fmt.Sprintf("Single namespace consumes %d%% of cluster CPU - apply ResourceQuota for fair distribution", r.Summary.TopConsumerPct))
	}
	return recs
}
