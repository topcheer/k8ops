package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespaceSecurityResult is the namespace security posture & trust boundary audit.
type NamespaceSecurityResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         NSSecuritySummary `json:"summary"`
	ByNamespace     []NSSecurityEntry `json:"byNamespace"`
	HighRiskNS      []NSSecurityEntry `json:"highRiskNS"`
	Risks           []NSSecurityRisk  `json:"risks"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
}

// NSSecuritySummary aggregates namespace security posture metrics.
type NSSecuritySummary struct {
	TotalNamespaces      int `json:"totalNamespaces"`
	SystemNamespaces     int `json:"systemNamespaces"` // kube-system, kube-public, etc.
	UserNamespaces       int `json:"userNamespaces"`
	WithPSAEnforce       int `json:"withPSAEnforce"`       // pod security admission enforce
	WithPSAWarn          int `json:"withPSAWarn"`          // pod security admission warn
	WithPSAAudit         int `json:"withPSAAudit"`         // pod security admission audit
	NoPSA                int `json:"noPSA"`                // no pod security admission
	DefaultSAToken       int `json:"defaultSAToken"`       // SA with token auto-mount
	WithNetworkPolicy    int `json:"withNetworkPolicy"`    // has at least one NP
	WithoutNetworkPolicy int `json:"withoutNetworkPolicy"` // no NP at all
	WithRBACBindings     int `json:"withRBACBindings"`     // has Role/RoleBinding
	WithoutRBAC          int `json:"withoutRBAC"`          // no RBAC (use default SA)
	HighRiskNamespaces   int `json:"highRiskNamespaces"`   // multiple security gaps
	LowRiskNamespaces    int `json:"lowRiskNamespaces"`    // well-protected
}

// NSSecurityEntry per-namespace security posture.
type NSSecurityEntry struct {
	Namespace          string   `json:"namespace"`
	IsSystem           bool     `json:"isSystem"`
	PSAEnforce         string   `json:"psaEnforce,omitempty"` // privileged, baseline, restricted
	PSAWarn            string   `json:"psaWarn,omitempty"`
	PSAAudit           string   `json:"psaAudit,omitempty"`
	HasDefaultSAToken  bool     `json:"hasDefaultSAToken"`
	SATokenAutoMount   bool     `json:"saTokenAutoMount"`
	HasNetworkPolicy   bool     `json:"hasNetworkPolicy"`
	NetworkPolicyCount int      `json:"networkPolicyCount"`
	HasRBAC            bool     `json:"hasRBAC"`
	RoleBindingCount   int      `json:"roleBindingCount"`
	HasResourceQuota   bool     `json:"hasResourceQuota"`
	HasLimitRange      bool     `json:"hasLimitRange"`
	TrustLevel         string   `json:"trustLevel"` // high, medium, low, untrusted
	RiskScore          int      `json:"riskScore"`  // 0-100 (higher = more risk)
	SecurityGaps       []string `json:"securityGaps,omitempty"`
}

// NSSecurityRisk describes a namespace security risk.
type NSSecurityRisk struct {
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleNamespaceSecurity audits namespace security posture & trust boundaries.
// GET /api/security/namespace-posture
func (s *Server) handleNamespaceSecurity(w http.ResponseWriter, r *http.Request) {
	result := NamespaceSecurityResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Get namespaces
	namespaces, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list namespaces: %v", err))
		return
	}

	// 2. Get network policies, RBAC, resource quotas, limit ranges, service accounts
	networkPolicies, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	roleBindings, _ := rc.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	roles, _ := rc.clientset.RbacV1().Roles("").List(r.Context(), metav1.ListOptions{})
	resourceQuotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(r.Context(), metav1.ListOptions{})
	serviceAccounts, _ := rc.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	// Build per-namespace maps
	nsNPCount := map[string]int{}
	if networkPolicies != nil {
		for _, np := range networkPolicies.Items {
			nsNPCount[np.Namespace]++
		}
	}

	nsRBACCount := map[string]int{}
	if roleBindings != nil {
		for _, rb := range roleBindings.Items {
			nsRBACCount[rb.Namespace]++
		}
	}
	nsRolesCount := map[string]int{}
	if roles != nil {
		for _, role := range roles.Items {
			nsRolesCount[role.Namespace]++
		}
	}

	nsQuotaCount := map[string]int{}
	if resourceQuotas != nil {
		for _, rq := range resourceQuotas.Items {
			nsQuotaCount[rq.Namespace]++
		}
	}

	nsLimitRangeCount := map[string]int{}
	if limitRanges != nil {
		for _, lr := range limitRanges.Items {
			nsLimitRangeCount[lr.Namespace]++
		}
	}

	nsSATokenAutoMount := map[string]bool{}
	nsHasDefaultSA := map[string]bool{}
	if serviceAccounts != nil {
		for _, sa := range serviceAccounts.Items {
			if sa.Name == "default" {
				nsHasDefaultSA[sa.Namespace] = true
				if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
					nsSATokenAutoMount[sa.Namespace] = true
				}
			}
		}
	}

	// System namespace patterns
	systemNSPatterns := []string{"kube-system", "kube-public", "kube-node-lease", "k8ops-system"}

	// 3. Analyze each namespace
	for _, ns := range namespaces.Items {
		result.Summary.TotalNamespaces++
		entry := NSSecurityEntry{
			Namespace:          ns.Name,
			SATokenAutoMount:   nsSATokenAutoMount[ns.Name],
			HasDefaultSAToken:  nsSATokenAutoMount[ns.Name],
			NetworkPolicyCount: nsNPCount[ns.Name],
			HasNetworkPolicy:   nsNPCount[ns.Name] > 0,
			RoleBindingCount:   nsRBACCount[ns.Name],
			HasRBAC:            nsRBACCount[ns.Name] > 0 || nsRolesCount[ns.Name] > 0,
			HasResourceQuota:   nsQuotaCount[ns.Name] > 0,
			HasLimitRange:      nsLimitRangeCount[ns.Name] > 0,
			RiskScore:          0,
		}

		// Check if system namespace
		for _, sysNS := range systemNSPatterns {
			if ns.Name == sysNS {
				entry.IsSystem = true
				break
			}
		}
		if entry.IsSystem {
			result.Summary.SystemNamespaces++
		} else {
			result.Summary.UserNamespaces++
		}

		// Check Pod Security Admission labels
		psaLabels := ns.Labels
		if v, ok := psaLabels["pod-security.kubernetes.io/enforce"]; ok {
			entry.PSAEnforce = v
			result.Summary.WithPSAEnforce++
		} else {
			result.Summary.NoPSA++
			entry.SecurityGaps = append(entry.SecurityGaps, "no pod security admission enforce")
			entry.RiskScore += 20
		}
		if v, ok := psaLabels["pod-security.kubernetes.io/warn"]; ok {
			entry.PSAWarn = v
			result.Summary.WithPSAWarn++
		}
		if v, ok := psaLabels["pod-security.kubernetes.io/audit"]; ok {
			entry.PSAAudit = v
			result.Summary.WithPSAAudit++
		}

		// Check default SA token auto-mount
		if nsSATokenAutoMount[ns.Name] {
			result.Summary.DefaultSAToken++
			if !entry.IsSystem {
				entry.SecurityGaps = append(entry.SecurityGaps, "default SA token auto-mount enabled")
				entry.RiskScore += 15
			}
		}

		// Check network policy
		if entry.HasNetworkPolicy {
			result.Summary.WithNetworkPolicy++
		} else {
			result.Summary.WithoutNetworkPolicy++
			if !entry.IsSystem {
				entry.SecurityGaps = append(entry.SecurityGaps, "no network policy")
				entry.RiskScore += 15
			}
		}

		// Check RBAC
		if entry.HasRBAC {
			result.Summary.WithRBACBindings++
		} else {
			result.Summary.WithoutRBAC++
			if !entry.IsSystem {
				entry.SecurityGaps = append(entry.SecurityGaps, "no RBAC role bindings")
				entry.RiskScore += 10
			}
		}

		// Check resource quota
		if !entry.HasResourceQuota && !entry.IsSystem {
			entry.SecurityGaps = append(entry.SecurityGaps, "no resource quota")
			entry.RiskScore += 5
		}

		// Check limit range
		if !entry.HasLimitRange && !entry.IsSystem {
			entry.SecurityGaps = append(entry.SecurityGaps, "no limit range")
			entry.RiskScore += 5
		}

		// Determine trust level
		if entry.RiskScore >= 40 {
			entry.TrustLevel = "untrusted"
			result.Summary.HighRiskNamespaces++
			result.HighRiskNS = append(result.HighRiskNS, entry)
			result.Risks = append(result.Risks, NSSecurityRisk{
				Namespace: ns.Name,
				Issue:     fmt.Sprintf("Namespace %s has %d security gaps (score %d): %s", ns.Name, len(entry.SecurityGaps), entry.RiskScore, strings.Join(entry.SecurityGaps, ", ")),
				Severity:  "high",
			})
		} else if entry.RiskScore >= 20 {
			entry.TrustLevel = "low"
		} else if entry.RiskScore >= 10 {
			entry.TrustLevel = "medium"
		} else {
			entry.TrustLevel = "high"
			result.Summary.LowRiskNamespaces++
		}

		result.ByNamespace = append(result.ByNamespace, entry)
	}

	// Sort by risk score (highest first)
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskScore > result.ByNamespace[j].RiskScore
	})

	// 4. Calculate health score
	score := 100
	if result.Summary.NoPSA > 0 {
		score -= min(25, result.Summary.NoPSA*3)
	}
	if result.Summary.WithoutNetworkPolicy > 0 {
		score -= min(20, result.Summary.WithoutNetworkPolicy*2)
	}
	if result.Summary.DefaultSAToken > 0 {
		score -= min(15, result.Summary.DefaultSAToken*2)
	}
	if result.Summary.HighRiskNamespaces > 0 {
		score -= min(20, result.Summary.HighRiskNamespaces*5)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 5. Recommendations
	if result.Summary.NoPSA > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespace(s) have no pod security admission — add PSA enforce labels (baseline or restricted)", result.Summary.NoPSA))
	}
	if result.Summary.WithoutNetworkPolicy > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespace(s) have no network policy — add default-deny NetworkPolicy for isolation", result.Summary.WithoutNetworkPolicy))
	}
	if result.Summary.DefaultSAToken > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespace(s) have default SA token auto-mount — set automountServiceAccountToken: false", result.Summary.DefaultSAToken))
	}
	if result.Summary.HighRiskNamespaces > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespace(s) are high-risk — address security gaps for production readiness", result.Summary.HighRiskNamespaces))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"All namespaces have adequate security posture — PSA, network policies, and RBAC are properly configured")
	}

	writeJSON(w, result)
}
