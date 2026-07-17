package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetPolicyResult analyzes network policy effectiveness, namespace isolation,
// and zero-trust posture across the cluster.
type NetPolicyResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         NetPolicySummary `json:"summary"`
	UnprotectedNS   []UnprotectedNS  `json:"unprotectedNamespaces"`
	PolicyAnalysis  []PolicyAnalysis `json:"policyAnalysis"`
	ByNamespace     []NetPolicyNS    `json:"byNamespace"`
	IsolationScore  int              `json:"isolationScore"`
	ZeroTrustLevel  string           `json:"zeroTrustLevel"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

// NetPolicySummary aggregates network policy statistics.
type NetPolicySummary struct {
	TotalNamespaces     int     `json:"totalNamespaces"`
	NamespacesWithNP    int     `json:"namespacesWithNP"`
	NamespacesWithoutNP int     `json:"namespacesWithoutNP"`
	TotalPolicies       int     `json:"totalPolicies"`
	DenyAllPolicies     int     `json:"denyAllPolicies"`
	AllowAllIngress     int     `json:"allowAllIngress"`
	AllowAllEgress      int     `json:"allowAllEgress"`
	DefaultDenyNS       int     `json:"defaultDenyNS"`
	EgressRestricted    int     `json:"egressRestricted"`
	IsolationPct        float64 `json:"isolationPct"`
}

// UnprotectedNS is a namespace without network policies.
type UnprotectedNS struct {
	Namespace    string `json:"namespace"`
	PodCount     int    `json:"podCount"`
	ServiceCount int    `json:"serviceCount"`
	RiskLevel    string `json:"riskLevel"`
	Impact       string `json:"impact"`
}

// PolicyAnalysis examines each network policy.
type PolicyAnalysis struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Type         string `json:"type"` // deny-all, allow-all, restrictive, permissive
	IngressRules int    `json:"ingressRules"`
	EgressRules  int    `json:"egressRules"`
	Verdict      string `json:"verdict"`
}

// NetPolicyNS shows per-namespace policy posture.
type NetPolicyNS struct {
	Namespace      string `json:"namespace"`
	HasPolicy      bool   `json:"hasPolicy"`
	PolicyCount    int    `json:"policyCount"`
	DefaultDeny    bool   `json:"defaultDeny"`
	EgressControl  bool   `json:"egressControl"`
	IsolationScore int    `json:"isolationScore"`
}

// handleNetPolicyEffectiveness provides network policy effectiveness & isolation scoring.
// GET /api/security/net-policy-effectiveness
func (s *Server) handleNetPolicyEffectiveness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := NetPolicyResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nps, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})

	// Build namespace stats
	type nsData struct {
		pods     int
		services int
		policies []*netv1.NetworkPolicy
	}
	nsMap := make(map[string]*nsData)

	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		nsMap[ns.Name] = &nsData{}
		result.Summary.TotalNamespaces++
	}

	// Count pods per namespace
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		if d, ok := nsMap[pod.Namespace]; ok {
			d.pods++
		}
	}

	// Count services per namespace
	for _, svc := range services.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		if d, ok := nsMap[svc.Namespace]; ok {
			d.services++
		}
	}

	// Collect policies per namespace
	for i := range nps.Items {
		np := &nps.Items[i]
		if systemNS[np.Namespace] {
			continue
		}
		if d, ok := nsMap[np.Namespace]; ok {
			d.policies = append(d.policies, np)
		}
	}

	totalPolicies := 0
	defaultDenyCount := 0
	egressRestrictedCount := 0
	allowAllIngressCount := 0
	allowAllEgressCount := 0

	for nsName, d := range nsMap {
		hasPolicy := len(d.policies) > 0
		defaultDeny := false
		egressControl := false
		nsScore := 0

		if hasPolicy {
			result.Summary.NamespacesWithNP++
			totalPolicies += len(d.policies)

			for _, np := range d.policies {
				pa := PolicyAnalysis{
					Name: np.Name, Namespace: nsName,
					IngressRules: len(np.Spec.Ingress),
					EgressRules:  len(np.Spec.Egress),
				}

				// Check for default deny (empty ingress = deny all ingress)
				if len(np.Spec.Ingress) == 0 && len(np.Spec.Egress) == 0 {
					pa.Type = "deny-all"
					pa.Verdict = "restrictive"
					defaultDeny = true
					defaultDenyCount++
					result.Summary.DenyAllPolicies++
					nsScore += 30
				} else if len(np.Spec.Ingress) == 0 {
					// Deny all ingress but allow some egress
					defaultDeny = true
					pa.Type = "restrictive-ingress"
					pa.Verdict = "good"
					nsScore += 20
				}

				// Check for allow-all ingress
				for _, rule := range np.Spec.Ingress {
					if len(rule.From) == 0 {
						pa.Type = "allow-all-ingress"
						pa.Verdict = "permissive"
						allowAllIngressCount++
						nsScore -= 10
					}
				}

				// Check for allow-all egress (no egress rules = allow all egress)
				if len(np.Spec.Egress) > 0 {
					egressControl = true
					egressRestrictedCount++
					pa.EgressRules = len(np.Spec.Egress)
				} else if np.Spec.PolicyTypes != nil {
					for _, pt := range np.Spec.PolicyTypes {
						if pt == netv1.PolicyTypeEgress && len(np.Spec.Egress) == 0 {
							// Egress policy type with no rules = deny all egress
							egressControl = true
							pa.Type = "deny-all-egress"
							pa.Verdict = "restrictive"
							nsScore += 15
						}
					}
				}

				result.PolicyAnalysis = append(result.PolicyAnalysis, pa)
			}
		} else {
			result.Summary.NamespacesWithoutNP++
		}

		if defaultDeny {
			result.Summary.DefaultDenyNS++
		}
		if egressControl {
			result.Summary.EgressRestricted++
		}

		if nsScore < 0 {
			nsScore = 0
		}
		if !hasPolicy {
			nsScore = 0
		}

		result.ByNamespace = append(result.ByNamespace, NetPolicyNS{
			Namespace:      nsName,
			HasPolicy:      hasPolicy,
			PolicyCount:    len(d.policies),
			DefaultDeny:    defaultDeny,
			EgressControl:  egressControl,
			IsolationScore: nsScore,
		})

		// Unprotected namespace
		if !hasPolicy {
			risk := "medium"
			impact := "No network isolation — all pods can communicate"
			if d.pods > 5 {
				risk = "high"
				impact = fmt.Sprintf("%d pods with no network isolation — any compromise spreads laterally", d.pods)
			}
			result.UnprotectedNS = append(result.UnprotectedNS, UnprotectedNS{
				Namespace:    nsName,
				PodCount:     d.pods,
				ServiceCount: d.services,
				RiskLevel:    risk,
				Impact:       impact,
			})
		}
	}

	result.Summary.TotalPolicies = totalPolicies
	result.Summary.AllowAllIngress = allowAllIngressCount
	result.Summary.AllowAllEgress = allowAllEgressCount

	if result.Summary.TotalNamespaces > 0 {
		result.Summary.IsolationPct = float64(result.Summary.NamespacesWithNP) / float64(result.Summary.TotalNamespaces) * 100
	}

	// Isolation score
	score := int(result.Summary.IsolationPct * 0.5)
	if result.Summary.DefaultDenyNS > 0 {
		score += int(float64(result.Summary.DefaultDenyNS) / float64(result.Summary.TotalNamespaces) * 30)
	}
	if result.Summary.EgressRestricted > 0 {
		score += int(float64(result.Summary.EgressRestricted) / float64(result.Summary.TotalNamespaces) * 20)
	}
	score -= allowAllIngressCount * 5
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	result.IsolationScore = score
	result.Grade = goldenScoreToGrade(score)

	// Zero trust level
	switch {
	case score >= 80:
		result.ZeroTrustLevel = "high"
	case score >= 50:
		result.ZeroTrustLevel = "moderate"
	case score >= 25:
		result.ZeroTrustLevel = "low"
	default:
		result.ZeroTrustLevel = "none"
	}

	// Sort
	sort.Slice(result.UnprotectedNS, func(i, j int) bool {
		return severityRankMap(result.UnprotectedNS[i].RiskLevel) > severityRankMap(result.UnprotectedNS[j].RiskLevel)
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].IsolationScore < result.ByNamespace[j].IsolationScore
	})

	result.Recommendations = generateNetPolicyRecs(result)

	writeJSON(w, result)
}

// generateNetPolicyRecs produces actionable recommendations.
func generateNetPolicyRecs(result NetPolicyResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("Network isolation: %d/100 (grade %s), zero-trust: %s", result.IsolationScore, result.Grade, result.ZeroTrustLevel))

	if result.Summary.NamespacesWithoutNP > 0 {
		recs = append(recs, fmt.Sprintf("%d/%d namespaces have no network policies — apply default deny policies for basic isolation",
			result.Summary.NamespacesWithoutNP, result.Summary.TotalNamespaces))
	}

	if result.Summary.DefaultDenyNS == 0 && result.Summary.TotalNamespaces > 0 {
		recs = append(recs, "No namespace implements default-deny — start with Calico/Cilium deny-all then add allow rules")
	}

	if result.Summary.AllowAllIngress > 0 {
		recs = append(recs, fmt.Sprintf("%d policies allow all ingress — restrict to known sources", result.Summary.AllowAllIngress))
	}

	if result.Summary.EgressRestricted == 0 {
		recs = append(recs, "No egress policies — pods can connect to any external service (data exfiltration risk)")
	}

	if len(result.UnprotectedNS) > 0 {
		top := result.UnprotectedNS[0]
		recs = append(recs, fmt.Sprintf("Highest risk: namespace '%s' has %d pods with zero isolation", top.Namespace, top.PodCount))
	}

	if len(recs) == 1 {
		recs = append(recs, "Network policies are effective — maintain current isolation posture")
	}

	return recs
}

// Suppress unused imports
var _ corev1.Pod
var _ strings.Builder
