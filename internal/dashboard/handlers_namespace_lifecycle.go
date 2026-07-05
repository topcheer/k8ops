package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceLifecycleResult is the full namespace governance audit.
type NamespaceLifecycleResult struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	Summary         NamespaceLifecycleSummary `json:"summary"`
	Namespaces      []NamespaceAuditEntry     `json:"namespaces"`
	Issues          []NamespaceIssue          `json:"issues"`
	Recommendations []string                  `json:"recommendations"`
}

// NamespaceLifecycleSummary aggregates cluster-wide namespace governance.
type NamespaceLifecycleSummary struct {
	TotalNamespaces   int `json:"totalNamespaces"`
	ActiveNamespaces  int `json:"activeNamespaces"`  // with running pods
	StaleNamespaces   int `json:"staleNamespaces"`   // no pods for >24h
	WithoutQuota      int `json:"withoutQuota"`      // no ResourceQuota
	WithoutLimitRange int `json:"withoutLimitRange"` // no LimitRange
	WithoutNetPolicy  int `json:"withoutNetPolicy"`  // no NetworkPolicy
	WithoutDefaultSA  int `json:"withoutDefaultSA"`  // uses "default" SA only
	WithDefaultLabels int `json:"withDefaultLabels"` // has required labels
	MissingLabels     int `json:"missingLabels"`     // missing required labels
	SystemNamespaces  int `json:"systemNamespaces"`
	GovernanceScore   int `json:"governanceScore"` // 0-100
}

// NamespaceAuditEntry describes governance for one namespace.
type NamespaceAuditEntry struct {
	Name             string            `json:"name"`
	Status           string            `json:"status"` // active / stale / terminating
	Age              string            `json:"age"`
	PodCount         int               `json:"podCount"`
	HasResourceQuota bool              `json:"hasResourceQuota"`
	HasLimitRange    bool              `json:"hasLimitRange"`
	HasNetworkPolicy bool              `json:"hasNetworkPolicy"`
	HasDefaultSA     bool              `json:"hasDefaultSA"` // has SA other than "default"
	HasLabels        bool              `json:"hasLabels"`
	Labels           map[string]string `json:"labels,omitempty"`
	HasAnnotations   bool              `json:"hasAnnotations"`
	ComplianceFlags  []string          `json:"complianceFlags"`
	RiskLevel        string            `json:"riskLevel"` // critical / high / medium / low
}

// NamespaceIssue is a detected governance problem.
type NamespaceIssue struct {
	Namespace string `json:"namespace"`
	Severity  string `json:"severity"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

// handleNamespaceLifecycle audits namespace governance and lifecycle.
// GET /api/product/namespaces/lifecycle
func (s *Server) handleNamespaceLifecycle(w http.ResponseWriter, r *http.Request) {
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

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	netPolicies, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	serviceAccounts, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})

	// Build lookup maps: ns → has resource
	nsHasQuota := make(map[string]bool)
	for _, q := range quotas.Items {
		nsHasQuota[q.Namespace] = true
	}

	nsHasLimitRange := make(map[string]bool)
	for _, lr := range limitRanges.Items {
		nsHasLimitRange[lr.Namespace] = true
	}

	nsHasNetPolicy := make(map[string]bool)
	for _, np := range netPolicies.Items {
		nsHasNetPolicy[np.Namespace] = true
	}

	// SA: ns → has SA other than "default"
	nsHasCustomSA := make(map[string]bool)
	for _, sa := range serviceAccounts.Items {
		if sa.Name != "default" && sa.Name != "kubernetes.io/default-account" {
			nsHasCustomSA[sa.Namespace] = true
		}
	}

	// Pod count per namespace
	nsPodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			nsPodCount[pod.Namespace]++
		}
	}

	// Required labels (configurable, basic defaults)
	requiredLabels := []string{"app", "team", "env", "owner"}

	result := NamespaceLifecycleResult{ScannedAt: time.Now()}

	for _, ns := range namespaces.Items {
		entry := NamespaceAuditEntry{
			Name:   ns.Name,
			Labels: ns.Labels,
		}

		// Status
		entry.Status = string(ns.Status.Phase)
		if ns.Status.Phase == corev1.NamespaceTerminating {
			entry.Status = "terminating"
		}

		// Age
		entry.Age = time.Since(ns.CreationTimestamp.Time).Round(time.Hour).String()

		// Pod count
		entry.PodCount = nsPodCount[ns.Name]

		// Active vs stale
		if entry.PodCount > 0 {
			entry.Status = "active"
			result.Summary.ActiveNamespaces++
		} else if entry.Status != "terminating" {
			entry.Status = "stale"
			result.Summary.StaleNamespaces++
		}

		// Skip system namespaces for compliance checks but still report them
		isSystem := isNamespaceSystem(ns.Name)
		if isSystem {
			result.Summary.SystemNamespaces++
		}

		// Governance checks
		entry.HasResourceQuota = nsHasQuota[ns.Name]
		entry.HasLimitRange = nsHasLimitRange[ns.Name]
		entry.HasNetworkPolicy = nsHasNetPolicy[ns.Name]
		entry.HasDefaultSA = nsHasCustomSA[ns.Name]
		entry.HasLabels = len(ns.Labels) > 0
		entry.HasAnnotations = len(ns.Annotations) > 0

		var flags []string
		var nsIssues []NamespaceIssue

		// Check required labels (skip system namespaces)
		if !isSystem {
			var missingLabels []string
			for _, req := range requiredLabels {
				if _, ok := ns.Labels[req]; !ok {
					missingLabels = append(missingLabels, req)
				}
			}
			if len(missingLabels) > 0 {
				flags = append(flags, fmt.Sprintf("missing labels: %v", missingLabels))
				result.Summary.MissingLabels++
				nsIssues = append(nsIssues, NamespaceIssue{
					Namespace: ns.Name,
					Severity:  "info",
					Type:      "missing-labels",
					Message:   fmt.Sprintf("Namespace missing required labels: %v", missingLabels),
				})
			} else {
				result.Summary.WithDefaultLabels++
			}

			// Resource quota
			if !entry.HasResourceQuota {
				flags = append(flags, "no ResourceQuota")
				result.Summary.WithoutQuota++
				nsIssues = append(nsIssues, NamespaceIssue{
					Namespace: ns.Name,
					Severity:  "warning",
					Type:      "no-quota",
					Message:   "Namespace has no ResourceQuota — pods can consume unlimited resources",
				})
			}

			// LimitRange
			if !entry.HasLimitRange {
				flags = append(flags, "no LimitRange")
				result.Summary.WithoutLimitRange++
				nsIssues = append(nsIssues, NamespaceIssue{
					Namespace: ns.Name,
					Severity:  "info",
					Type:      "no-limitrange",
					Message:   "Namespace has no LimitRange — pods may have no default resource limits",
				})
			}

			// NetworkPolicy
			if !entry.HasNetworkPolicy {
				flags = append(flags, "no NetworkPolicy")
				result.Summary.WithoutNetPolicy++
				nsIssues = append(nsIssues, NamespaceIssue{
					Namespace: ns.Name,
					Severity:  "warning",
					Type:      "no-netpolicy",
					Message:   "Namespace has no NetworkPolicy — all ingress/egress traffic is unrestricted",
				})
			}

			// Service Account
			if !entry.HasDefaultSA {
				flags = append(flags, "only default SA")
				result.Summary.WithoutDefaultSA++
				nsIssues = append(nsIssues, NamespaceIssue{
					Namespace: ns.Name,
					Severity:  "info",
					Type:      "default-sa",
					Message:   "Namespace only has the default ServiceAccount — create dedicated SA for least privilege",
				})
			}

			// Stale namespace
			if entry.Status == "stale" && entry.PodCount == 0 {
				flags = append(flags, "stale (no running pods)")
				nsIssues = append(nsIssues, NamespaceIssue{
					Namespace: ns.Name,
					Severity:  "warning",
					Type:      "stale",
					Message:   "Namespace has no running pods — consider cleanup if no longer needed",
				})
			}
		}

		entry.ComplianceFlags = flags
		entry.RiskLevel = assessNamespaceRisk(entry, isSystem)
		result.Issues = append(result.Issues, nsIssues...)
		result.Summary.TotalNamespaces++
		result.Namespaces = append(result.Namespaces, entry)
	}

	// Sort namespaces by risk
	sort.Slice(result.Namespaces, func(i, j int) bool {
		return namespaceRiskRank(result.Namespaces[i].RiskLevel) < namespaceRiskRank(result.Namespaces[j].RiskLevel)
	})

	// Sort issues
	sort.Slice(result.Issues, func(i, j int) bool {
		return namespaceIssueRank(result.Issues[i].Severity) < namespaceIssueRank(result.Issues[j].Severity)
	})

	// Score
	result.Summary.GovernanceScore = calculateNamespaceGovernanceScore(result.Summary)

	// Recommendations
	result.Recommendations = generateNamespaceRecommendations(result.Summary)

	writeJSON(w, result)
}

// isSystemNamespace checks if a namespace is a system namespace.
func isNamespaceSystem(name string) bool {
	systemNamespaces := map[string]bool{
		"kube-system":             true,
		"kube-public":             true,
		"kube-node-lease":         true,
		"default":                 true,
		"k8ops-system":            true,
		"ingress-nginx":           true,
		"metallb-system":          true,
		"longhorn-system":         true,
		"cert-manager":            true,
		"cattle-system":           true,
		"cattle-epinio":           true,
		"cattle-fleet-system":     true,
		"cattle-neuvector-system": true,
	}
	if systemNamespaces[name] {
		return true
	}
	if len(name) > 5 && name[:5] == "kube-" {
		return true
	}
	return false
}

// assessNamespaceRisk determines risk level.
func assessNamespaceRisk(entry NamespaceAuditEntry, isSystem bool) string {
	if isSystem {
		return "low"
	}

	risk := 0
	if !entry.HasResourceQuota {
		risk += 15
	}
	if !entry.HasNetworkPolicy {
		risk += 15
	}
	if !entry.HasLimitRange {
		risk += 5
	}
	if entry.Status == "stale" {
		risk += 10
	}
	if !entry.HasLabels {
		risk += 5
	}

	switch {
	case risk >= 30:
		return "critical"
	case risk >= 20:
		return "high"
	case risk >= 10:
		return "medium"
	default:
		return "low"
	}
}

// calculateNamespaceGovernanceScore computes 0-100.
func calculateNamespaceGovernanceScore(s NamespaceLifecycleSummary) int {
	appNs := s.TotalNamespaces - s.SystemNamespaces
	if appNs <= 0 {
		return 100
	}
	score := 100
	// Quota missing is serious
	score -= (s.WithoutQuota * 100 / appNs) * 20 / 100
	// NetworkPolicy missing is serious
	score -= (s.WithoutNetPolicy * 100 / appNs) * 20 / 100
	// LimitRange missing is moderate
	score -= (s.WithoutLimitRange * 100 / appNs) * 10 / 100
	// Stale namespaces waste resources
	score -= (s.StaleNamespaces * 100 / appNs) * 10 / 100
	// Missing labels is minor
	score -= (s.MissingLabels * 100 / appNs) * 5 / 100
	if score < 0 {
		score = 0
	}
	return score
}

// generateNamespaceRecommendations produces actionable advice.
func generateNamespaceRecommendations(s NamespaceLifecycleSummary) []string {
	var recs []string
	appNs := s.TotalNamespaces - s.SystemNamespaces

	if appNs > 0 {
		if s.WithoutQuota > 0 {
			recs = append(recs, fmt.Sprintf("%d/%d application namespaces have no ResourceQuota — add quotas to prevent resource exhaustion", s.WithoutQuota, appNs))
		}
		if s.WithoutNetPolicy > 0 {
			recs = append(recs, fmt.Sprintf("%d/%d application namespaces have no NetworkPolicy — add default deny policies for security", s.WithoutNetPolicy, appNs))
		}
		if s.WithoutLimitRange > 0 {
			recs = append(recs, fmt.Sprintf("%d/%d application namespaces have no LimitRange — add default resource requests/limits", s.WithoutLimitRange, appNs))
		}
		if s.StaleNamespaces > 0 {
			recs = append(recs, fmt.Sprintf("%d stale namespaces with no running pods — consider cleanup to reduce attack surface", s.StaleNamespaces))
		}
		if s.MissingLabels > 0 {
			recs = append(recs, fmt.Sprintf("%d namespaces missing required labels (app, team, env, owner) — add for governance tracking", s.MissingLabels))
		}
		if s.WithoutDefaultSA > 0 {
			recs = append(recs, fmt.Sprintf("%d namespaces only have the default ServiceAccount — create dedicated SAs for least privilege", s.WithoutDefaultSA))
		}
	}
	if s.GovernanceScore < 60 {
		recs = append(recs, fmt.Sprintf("Namespace governance score is %d/100 — review namespace policies and enforcement", s.GovernanceScore))
	}

	return recs
}

func namespaceRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func namespaceIssueRank(s string) int {
	switch s {
	case "warning":
		return 0
	case "info":
		return 1
	default:
		return 2
	}
}

// Ensure imports are used.
var _ = networkingv1.NetworkPolicy{}
