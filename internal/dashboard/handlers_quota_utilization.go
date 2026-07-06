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

// QUResult is the resource quota utilization & limit compliance analysis.
type QUResult struct {
	ScannedAt       time.Time    `json:"scannedAt"`
	Summary         QUSummary    `json:"summary"`
	Quotas          []QUEntry    `json:"quotas"`
	CriticalQuotas  []QUEntry    `json:"criticalQuotas"` // >80% utilization
	LimitRanges     []QULREntry  `json:"limitRanges"`
	UnboundedPods   []QUPodEntry `json:"unboundedPods"` // pods without requests/limits
	ByNamespace     []QUNSEntry  `json:"byNamespace"`
	Issues          []QUIssue    `json:"issues"`
	Recommendations []string     `json:"recommendations"`
}

// QUSummary aggregates quota utilization.
type QUSummary struct {
	TotalNamespaces  int     `json:"totalNamespaces"`
	NSWithQuota      int     `json:"nsWithQuota"`
	NSWithoutQuota   int     `json:"nsWithoutQuota"`
	NSWithLimitRange int     `json:"nsWithLimitRange"`
	TotalQuotas      int     `json:"totalQuotas"`
	CriticalQuotas   int     `json:"criticalQuotas"` // >80% used
	TotalContainers  int     `json:"totalContainers"`
	NoRequests       int     `json:"noRequests"`      // containers without requests
	NoLimits         int     `json:"noLimits"`        // containers without limits
	UnboundedRatio   float64 `json:"unboundedRatio"`  // % containers without both
	ComplianceScore  int     `json:"complianceScore"` // 0-100
}

// QUEntry describes one ResourceQuota.
type QUEntry struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Hard        map[string]string `json:"hard"`
	Used        map[string]string `json:"used"`
	Utilization []QUUtilItem      `json:"utilization"`
	MaxUtil     float64           `json:"maxUtilization"` // highest utilization %
	RiskLevel   string            `json:"riskLevel"`
}

// QUUtilItem describes utilization for one resource.
type QUUtilItem struct {
	Resource    string  `json:"resource"`
	Hard        string  `json:"hard"`
	Used        string  `json:"used"`
	Utilization float64 `json:"utilization"` // %
}

// QULREntry describes one LimitRange.
type QULREntry struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Limits            int    `json:"limitCount"`
	HasDefaultRequest bool   `json:"hasDefaultRequest"`
	HasDefaultLimit   bool   `json:"hasDefaultLimit"`
	HasMaxLimit       bool   `json:"hasMaxLimit"`
}

// QUPodEntry describes an unbounded pod.
type QUPodEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Container       string `json:"container"`
	MissingRequests bool   `json:"missingRequests"`
	MissingLimits   bool   `json:"missingLimits"`
}

// QUNSEntry per-namespace stats.
type QUNSEntry struct {
	Namespace      string  `json:"namespace"`
	HasQuota       bool    `json:"hasQuota"`
	HasLimitRange  bool    `json:"hasLimitRange"`
	MaxUtilization float64 `json:"maxUtilization"`
	UnboundedPct   float64 `json:"unboundedPct"`
	RiskLevel      string  `json:"riskLevel"`
}

// QUIssue is a detected problem.
type QUIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleQuotaUtilization audits resource quota utilization and limit compliance.
// GET /api/scalability/quota-utilization
func (s *Server) handleQuotaUtilization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	namespaces, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	quotas, err := rc.clientset.CoreV1().ResourceQuotas(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	limitRanges, err := rc.clientset.CoreV1().LimitRanges(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := QUResult{ScannedAt: time.Now()}

	// Build namespace sets
	nsQuotaMap := make(map[string]bool)
	nsLRMap := make(map[string]bool)
	nsMap := make(map[string]*QUNSEntry)

	// Analyze quotas
	for _, rq := range quotas.Items {
		entry := QUEntry{
			Name:      rq.Name,
			Namespace: rq.Namespace,
			Hard:      make(map[string]string),
			Used:      make(map[string]string),
		}

		for k, v := range rq.Status.Hard {
			entry.Hard[string(k)] = v.String()
		}
		for k, v := range rq.Status.Used {
			entry.Used[string(k)] = v.String()
		}

		// Calculate utilization per resource
		for k, hardQty := range rq.Status.Hard {
			usedQty := rq.Status.Used[k]
			util := 0.0
			if !hardQty.IsZero() {
				// Use MilliValue for precise comparison
				hardMi := hardQty.MilliValue()
				usedMi := usedQty.MilliValue()
				if hardMi > 0 {
					util = float64(usedMi) / float64(hardMi) * 100
				}
			}
			entry.Utilization = append(entry.Utilization, QUUtilItem{
				Resource:    string(k),
				Hard:        hardQty.String(),
				Used:        usedQty.String(),
				Utilization: util,
			})
			if util > entry.MaxUtil {
				entry.MaxUtil = util
			}
		}

		entry.RiskLevel = quAssessQuotaRisk(entry.MaxUtil)
		nsQuotaMap[rq.Namespace] = true

		result.Summary.TotalQuotas++
		if entry.MaxUtil > 80 {
			result.Summary.CriticalQuotas++
			result.CriticalQuotas = append(result.CriticalQuotas, entry)
			result.Issues = append(result.Issues, QUIssue{
				Severity: "critical", Type: "quota-near-limit",
				Resource: fmt.Sprintf("%s/%s", rq.Namespace, rq.Name),
				Message:  fmt.Sprintf("Quota %s/%s is %.0f%% utilized — pods will be rejected soon", rq.Namespace, rq.Name, entry.MaxUtil),
			})
		}
		result.Quotas = append(result.Quotas, entry)
	}

	// Analyze LimitRanges
	for _, lr := range limitRanges.Items {
		entry := QULREntry{
			Name:      lr.Name,
			Namespace: lr.Namespace,
			Limits:    len(lr.Spec.Limits),
		}
		for _, limit := range lr.Spec.Limits {
			if limit.DefaultRequest != nil {
				entry.HasDefaultRequest = true
			}
			if limit.Default != nil {
				entry.HasDefaultLimit = true
			}
			if limit.Max != nil {
				entry.HasMaxLimit = true
			}
		}
		nsLRMap[lr.Namespace] = true
		result.LimitRanges = append(result.LimitRanges, entry)

		if !entry.HasDefaultLimit || !entry.HasDefaultRequest {
			result.Issues = append(result.Issues, QUIssue{
				Severity: "info", Type: "incomplete-limit-range",
				Resource: fmt.Sprintf("%s/%s", lr.Namespace, lr.Name),
				Message:  fmt.Sprintf("LimitRange %s/%s missing default request/limit — unbounded containers get no defaults", lr.Namespace, lr.Name),
			})
		}
	}

	// Analyze pods for missing requests/limits
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			hasCPUReq := false
			hasMemReq := false
			hasCPULim := false
			hasMemLim := false

			if req := c.Resources.Requests; len(req) > 0 {
				if _, ok := req[corev1.ResourceCPU]; ok {
					hasCPUReq = true
				}
				if _, ok := req[corev1.ResourceMemory]; ok {
					hasMemReq = true
				}
			}
			if lim := c.Resources.Limits; len(lim) > 0 {
				if _, ok := lim[corev1.ResourceCPU]; ok {
					hasCPULim = true
				}
				if _, ok := lim[corev1.ResourceMemory]; ok {
					hasMemLim = true
				}
			}

			missingReq := !hasCPUReq || !hasMemReq
			missingLim := !hasCPULim || !hasMemLim

			if missingReq {
				result.Summary.NoRequests++
			}
			if missingLim {
				result.Summary.NoLimits++
			}

			if missingReq || missingLim {
				result.UnboundedPods = append(result.UnboundedPods, QUPodEntry{
					Name:            pod.Name,
					Namespace:       pod.Namespace,
					Container:       c.Name,
					MissingRequests: missingReq,
					MissingLimits:   missingLim,
				})
			}
		}
	}

	// Namespace stats
	for _, namespace := range namespaces.Items {
		if namespace.Status.Phase != corev1.NamespaceActive {
			continue
		}
		result.Summary.TotalNamespaces++
		nsStat := quGetOrCreateNS(nsMap, namespace.Name)

		if nsQuotaMap[namespace.Name] {
			nsStat.HasQuota = true
			result.Summary.NSWithQuota++
		} else {
			nsStat.HasQuota = false
			result.Summary.NSWithoutQuota++
			result.Issues = append(result.Issues, QUIssue{
				Severity: "warning", Type: "no-quota",
				Resource: namespace.Name,
				Message:  fmt.Sprintf("Namespace %s has NO ResourceQuota — unbounded resource consumption", namespace.Name),
			})
		}
		if nsLRMap[namespace.Name] {
			nsStat.HasLimitRange = true
			result.Summary.NSWithLimitRange++
		}

		// Max utilization from quotas
		for _, q := range result.Quotas {
			if q.Namespace == namespace.Name && q.MaxUtil > nsStat.MaxUtilization {
				nsStat.MaxUtilization = q.MaxUtil
			}
		}

		nsStat.RiskLevel = quNSRisk(*nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Calculate unbounded ratio
	if result.Summary.TotalContainers > 0 {
		result.Summary.UnboundedRatio = float64(result.Summary.NoRequests+result.Summary.NoLimits) / float64(result.Summary.TotalContainers*2) * 100
	}

	// Sort
	sort.Slice(result.Quotas, func(i, j int) bool {
		return result.Quotas[i].MaxUtil > result.Quotas[j].MaxUtil
	})
	sort.Slice(result.CriticalQuotas, func(i, j int) bool {
		return result.CriticalQuotas[i].MaxUtil > result.CriticalQuotas[j].MaxUtil
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MaxUtilization > result.ByNamespace[j].MaxUtilization
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return quIssueRank(result.Issues[i].Severity) < quIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ComplianceScore = quScore(result.Summary)
	result.Recommendations = quGenRecs(result.Summary, result.CriticalQuotas, result.UnboundedPods)

	writeJSON(w, result)
}

// quAssessQuotaRisk determines risk based on utilization.
func quAssessQuotaRisk(maxUtil float64) string {
	switch {
	case maxUtil > 90:
		return "critical"
	case maxUtil > 80:
		return "high"
	case maxUtil > 60:
		return "medium"
	default:
		return "low"
	}
}

// quNSRisk determines namespace risk.
func quNSRisk(ns QUNSEntry) string {
	if !ns.HasQuota {
		return "high"
	}
	if ns.MaxUtilization > 90 {
		return "critical"
	}
	if ns.MaxUtilization > 80 {
		return "high"
	}
	return "low"
}

// quScore computes 0-100.
func quScore(s QUSummary) int {
	if s.TotalNamespaces == 0 {
		return 100
	}
	score := 100
	score -= s.NSWithoutQuota * 5
	score -= s.CriticalQuotas * 8
	score -= int(s.UnboundedRatio * 0.3)
	if score < 0 {
		score = 0
	}
	return score
}

// quGenRecs produces actionable advice.
func quGenRecs(s QUSummary, critical []QUEntry, unbounded []QUPodEntry) []string {
	var recs []string

	if s.NSWithoutQuota > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) have NO ResourceQuota — add quotas to prevent unbounded resource consumption", s.NSWithoutQuota))
	}
	if s.CriticalQuotas > 0 {
		top := ""
		if len(critical) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %.0f%%)", critical[0].Namespace, critical[0].Name, critical[0].MaxUtil)
		}
		recs = append(recs, fmt.Sprintf("%d quota(s) at >80%% utilization%s — pods will be rejected when quota is exhausted", s.CriticalQuotas, top))
	}
	if s.NoRequests > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing resource requests — scheduler cannot make informed placement decisions", s.NoRequests))
	}
	if s.NoLimits > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) missing resource limits — risk of resource starvation and noisy neighbors", s.NoLimits))
	}
	if s.UnboundedRatio > 30 {
		recs = append(recs, fmt.Sprintf("%.0f%% of containers lack requests or limits — add LimitRange with defaults or update deployments", s.UnboundedRatio))
	}
	if s.NSWithLimitRange < s.TotalNamespaces {
		recs = append(recs, fmt.Sprintf("%d of %d namespace(s) have NO LimitRange — add LimitRange for default requests/limits", s.TotalNamespaces-s.NSWithLimitRange, s.TotalNamespaces))
	}
	if s.ComplianceScore < 60 {
		recs = append(recs, fmt.Sprintf("Quota compliance score is %d/100 — multiple namespaces lack resource governance", s.ComplianceScore))
	}
	if s.NSWithoutQuota == 0 && s.CriticalQuotas == 0 && s.UnboundedRatio < 10 {
		recs = append(recs, "Quota and limit compliance is healthy — good resource governance")
	}

	return recs
}

func quGetOrCreateNS(m map[string]*QUNSEntry, ns string) *QUNSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &QUNSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func quIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

// Ensure resource import is used
var _ = resource.Quantity{}
