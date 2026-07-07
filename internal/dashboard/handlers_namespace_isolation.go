package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NIResult is the namespace isolation & multi-tenancy audit.
type NIResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         NISummary `json:"summary"`
	ByNamespace     []NIEntry `json:"byNamespace"`
	Unisolated      []NIEntry `json:"unisolated"`   // missing NetworkPolicy
	NoQuota         []NIEntry `json:"noQuota"`      // missing ResourceQuota
	NoLimitRange    []NIEntry `json:"noLimitRange"` // missing LimitRange
	Issues          []NIIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// NISummary aggregates namespace isolation stats.
type NISummary struct {
	TotalNamespaces   int `json:"totalNamespaces"`
	WithNetworkPolicy int `json:"withNetworkPolicy"`
	WithResourceQuota int `json:"withResourceQuota"`
	WithLimitRange    int `json:"withLimitRange"`
	FullyIsolated     int `json:"fullyIsolated"` // all 3 controls
	SystemNamespaces  int `json:"systemNamespaces"`
	UserNamespaces    int `json:"userNamespaces"`
	IsolationScore    int `json:"isolationScore"` // 0-100
}

// NIEntry describes one namespace's isolation status.
type NIEntry struct {
	Name             string   `json:"name"`
	IsSystem         bool     `json:"isSystem"`
	HasNetworkPolicy bool     `json:"hasNetworkPolicy"`
	HasResourceQuota bool     `json:"hasResourceQuota"`
	HasLimitRange    bool     `json:"hasLimitRange"`
	PSALabel         string   `json:"psaLabel"` // privileged/baseline/restricted
	MissingControls  []string `json:"missingControls"`
	RiskLevel        string   `json:"riskLevel"`
}

// NIIssue is a detected isolation problem.
type NIIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleNamespaceIsolation audits namespace isolation for multi-tenancy.
// GET /api/scalability/namespace-isolation
func (s *Server) handleNamespaceIsolation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nss, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build NetworkPolicy map: ns → has any policy
	npList, err := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	npMap := make(map[string]bool)
	if err == nil && npList != nil {
		for _, np := range npList.Items {
			npMap[np.Namespace] = true
		}
	}

	// Build ResourceQuota map
	rqList, err := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	rqMap := make(map[string]bool)
	if err == nil && rqList != nil {
		for _, rq := range rqList.Items {
			rqMap[rq.Namespace] = true
		}
	}

	// Build LimitRange map
	lrList, err := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	lrMap := make(map[string]bool)
	if err == nil && lrList != nil {
		for _, lr := range lrList.Items {
			lrMap[lr.Namespace] = true
		}
	}

	result := NIResult{ScannedAt: time.Now()}

	systemPrefixes := []string{"kube-", " cattle-", "traefik", "metallb", "ingress"}

	for _, ns := range nss.Items {
		if ns.Status.Phase != corev1.NamespaceActive {
			continue
		}

		entry := NIEntry{Name: ns.Name}
		entry.IsSystem = niIsSystem(ns.Name, systemPrefixes)
		entry.HasNetworkPolicy = npMap[ns.Name]
		entry.HasResourceQuota = rqMap[ns.Name]
		entry.HasLimitRange = lrMap[ns.Name]

		// PSA label
		if v, ok := ns.Labels["pod-security.kubernetes.io/enforce"]; ok {
			entry.PSALabel = v
		} else {
			entry.PSALabel = "none"
		}

		result.Summary.TotalNamespaces++
		if entry.IsSystem {
			result.Summary.SystemNamespaces++
		} else {
			result.Summary.UserNamespaces++
		}

		// Only check controls for user namespaces (system namespaces are typically exempt)
		if !entry.IsSystem {
			var missing []string
			if !entry.HasNetworkPolicy {
				missing = append(missing, "NetworkPolicy")
				result.Unisolated = append(result.Unisolated, entry)
			}
			if !entry.HasResourceQuota {
				missing = append(missing, "ResourceQuota")
				result.NoQuota = append(result.NoQuota, entry)
			}
			if !entry.HasLimitRange {
				missing = append(missing, "LimitRange")
				result.NoLimitRange = append(result.NoLimitRange, entry)
			}
			entry.MissingControls = missing

			// PSA check
			if entry.PSALabel == "none" || entry.PSALabel == "privileged" {
				result.Issues = append(result.Issues, NIIssue{
					Severity: "warning", Type: "no-psa",
					Resource: ns.Name,
					Message:  fmt.Sprintf("Namespace %s has no PSA enforce label — pods can run with privileged security context", ns.Name),
				})
			}

			if len(missing) == 0 {
				result.Summary.FullyIsolated++
			} else if len(missing) >= 2 {
				result.Issues = append(result.Issues, NIIssue{
					Severity: "warning", Type: "insufficient-isolation",
					Resource: ns.Name,
					Message:  fmt.Sprintf("Namespace %s missing %d isolation controls: %s — not safe for multi-tenancy", ns.Name, len(missing), strings.Join(missing, ", ")),
				})
			}
		}

		if entry.HasNetworkPolicy {
			result.Summary.WithNetworkPolicy++
		}
		if entry.HasResourceQuota {
			result.Summary.WithResourceQuota++
		}
		if entry.HasLimitRange {
			result.Summary.WithLimitRange++
		}

		entry.RiskLevel = niAssessRisk(entry)
		result.ByNamespace = append(result.ByNamespace, entry)
	}

	// Sort
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return niRiskRank(result.ByNamespace[i].RiskLevel) < niRiskRank(result.ByNamespace[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return niIssueRank(result.Issues[i].Severity) < niIssueRank(result.Issues[j].Severity)
	})

	result.Summary.IsolationScore = niScore(result.Summary)
	result.Recommendations = niGenRecs(result.Summary, result.Unisolated, result.NoQuota)

	writeJSON(w, result)
}

// niIsSystem checks if a namespace is a system namespace.
func niIsSystem(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	if name == "default" {
		return true
	}
	return false
}

// niAssessRisk determines risk level.
func niAssessRisk(entry NIEntry) string {
	if entry.IsSystem {
		return "low"
	}
	if !entry.HasNetworkPolicy && !entry.HasResourceQuota && !entry.HasLimitRange {
		return "high"
	}
	if !entry.HasNetworkPolicy || !entry.HasResourceQuota {
		return "medium"
	}
	return "low"
}

// niScore computes isolation score 0-100.
func niScore(s NISummary) int {
	if s.UserNamespaces == 0 {
		return 100
	}
	score := 100
	unisolated := s.UserNamespaces - s.WithNetworkPolicy
	noQuota := s.UserNamespaces - s.WithResourceQuota
	noLR := s.UserNamespaces - s.WithLimitRange
	score -= unisolated * 5
	score -= noQuota * 3
	score -= noLR * 2
	if score < 0 {
		score = 0
	}
	return score
}

// niGenRecs produces actionable advice.
func niGenRecs(s NISummary, unisolated []NIEntry, noQuota []NIEntry) []string {
	var recs []string

	unisolatedCount := s.UserNamespaces - s.WithNetworkPolicy
	noQuotaCount := s.UserNamespaces - s.WithResourceQuota
	noLRCount := s.UserNamespaces - s.WithLimitRange

	if unisolatedCount > 0 {
		top := ""
		if len(unisolated) > 0 {
			top = fmt.Sprintf(" (e.g. %s)", unisolated[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d user namespace(s) lack NetworkPolicy%s — pods are accessible from any namespace, add default deny policies", unisolatedCount, top))
	}
	if noQuotaCount > 0 {
		recs = append(recs, fmt.Sprintf("%d user namespace(s) lack ResourceQuota — pods can consume unlimited resources, add quotas", noQuotaCount))
	}
	if noLRCount > 0 {
		recs = append(recs, fmt.Sprintf("%d user namespace(s) lack LimitRange — pods without explicit requests get defaults, add limit ranges", noLRCount))
	}
	if s.FullyIsolated < s.UserNamespaces {
		recs = append(recs, fmt.Sprintf("Only %d/%d user namespaces are fully isolated — apply all 3 controls for safe multi-tenancy", s.FullyIsolated, s.UserNamespaces))
	}
	if s.IsolationScore < 70 {
		recs = append(recs, fmt.Sprintf("Namespace isolation score is %d/100 — review multi-tenancy controls", s.IsolationScore))
	}
	if unisolatedCount == 0 && noQuotaCount == 0 {
		recs = append(recs, "All user namespaces have proper isolation controls — good multi-tenancy posture")
	}

	return recs
}

func niRiskRank(level string) int {
	switch level {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

func niIssueRank(s string) int {
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

var _ = networkingv1.NetworkPolicy{}
