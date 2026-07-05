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
	"k8s.io/apimachinery/pkg/labels"
)

// NPAuditResult is the network policy compliance & traffic isolation analysis.
type NPAuditResult struct {
	ScannedAt          time.Time         `json:"scannedAt"`
	Summary            NPASummary        `json:"summary"`
	ByNamespace        []NPNamespaceStat `json:"byNamespace"`
	UnprotectedPods    []NPPodEntry      `json:"unprotectedPods"`
	AllPolicies        []NPPolicyEntry   `json:"allPolicies"`
	PermissivePolicies []NPPolicyEntry   `json:"permissivePolicies"`
	Issues             []NPIssue         `json:"issues"`
	Recommendations    []string          `json:"recommendations"`
}

// NPASummary aggregates network policy compliance.
type NPASummary struct {
	TotalNamespaces  int `json:"totalNamespaces"`
	NamespacesWithNP int `json:"namespacesWithNP"`
	NamespacesNoNP   int `json:"namespacesNoNP"`
	TotalPods        int `json:"totalPods"`
	ProtectedPods    int `json:"protectedPods"`
	UnprotectedPods  int `json:"unprotectedPods"`
	TotalPolicies    int `json:"totalPolicies"`
	DefaultDenyNS    int `json:"defaultDenyNS"`   // NS with default-deny ingress/egress
	PermissiveCount  int `json:"permissiveCount"` // policies with 0.0.0.0/0 egress
	IsolationScore   int `json:"isolationScore"`  // 0-100
}

// NPNamespaceStat per-namespace stats.
type NPNamespaceStat struct {
	Namespace      string `json:"namespace"`
	PodCount       int    `json:"podCount"`
	PolicyCount    int    `json:"policyCount"`
	ProtectedPods  int    `json:"protectedPods"`
	HasDefaultDeny bool   `json:"hasDefaultDeny"`
	IsolationScore int    `json:"isolationScore"`
	RiskLevel      string `json:"riskLevel"`
}

// NPPodEntry describes an unprotected pod.
type NPPodEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Labels    string `json:"labels"`
	RiskLevel string `json:"riskLevel"`
}

// NPPolicyEntry describes one NetworkPolicy.
type NPPolicyEntry struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	IngressRules     int    `json:"ingressRules"`
	EgressRules      int    `json:"egressRules"`
	HasDefaultDenyIn bool   `json:"hasDefaultDenyIn"`
	HasDefaultDenyEg bool   `json:"hasDefaultDenyEg"`
	Selector         string `json:"selector"`
	IsPermissive     bool   `json:"isPermissive"`
	PermissiveReason string `json:"permissiveReason,omitempty"`
	RiskLevel        string `json:"riskLevel"`
}

// NPIssue is a detected problem.
type NPIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleNetworkPolicyAudit audits network policy compliance and traffic isolation.
// GET /api/product/network-policy
func (s *Server) handleNetworkPolicyAudit(w http.ResponseWriter, r *http.Request) {
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

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	policies, err := rc.clientset.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := NPAuditResult{ScannedAt: time.Now()}

	// Build policy → pod selection map
	// A pod is "protected" if at least one NetworkPolicy selects it
	// (meaning its traffic is restricted by that policy)
	type nsPolicySet struct {
		policies         []netv1.NetworkPolicy
		hasDefaultDenyIn bool
		hasDefaultDenyEg bool
	}
	nsPolicies := make(map[string]*nsPolicySet)
	nsHasPolicies := make(map[string]bool)

	for _, policy := range policies.Items {
		entry := npAnalyzePolicy(policy)
		result.Summary.TotalPolicies++
		result.AllPolicies = append(result.AllPolicies, entry)

		nps := nsPolicies[policy.Namespace]
		if nps == nil {
			nps = &nsPolicySet{}
			nsPolicies[policy.Namespace] = nps
		}
		nps.policies = append(nps.policies, policy)
		nps.hasDefaultDenyIn = nps.hasDefaultDenyIn || entry.HasDefaultDenyIn
		nps.hasDefaultDenyEg = nps.hasDefaultDenyEg || entry.HasDefaultDenyEg
		nsHasPolicies[policy.Namespace] = true

		if entry.IsPermissive {
			result.Summary.PermissiveCount++
			result.PermissivePolicies = append(result.PermissivePolicies, entry)
			result.Issues = append(result.Issues, NPIssue{
				Severity: "warning", Type: "permissive-egress",
				Resource: fmt.Sprintf("%s/%s", policy.Namespace, policy.Name),
				Message:  fmt.Sprintf("Policy %s/%s allows egress to 0.0.0.0/0 — data exfiltration risk", policy.Namespace, policy.Name),
			})
		}
	}

	// Build per-namespace pod maps
	type nsPodSet struct {
		pods      []corev1.Pod
		protected int
	}
	nsPods := make(map[string]*nsPodSet)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		// Skip system pods
		if pod.Namespace == "kube-system" && !strings.HasPrefix(pod.Name, "k8ops") {
			// Still count but mark low risk
		}

		result.Summary.TotalPods++
		pset := nsPods[pod.Namespace]
		if pset == nil {
			pset = &nsPodSet{}
			nsPods[pod.Namespace] = pset
		}
		pset.pods = append(pset.pods, pod)

		// Check if any policy selects this pod
		isProtected := false
		if nps, ok := nsPolicies[pod.Namespace]; ok {
			for _, policy := range nps.policies {
				if policy.Spec.PodSelector.Size() == 0 {
					// Empty selector = selects all pods in namespace
					isProtected = true
					break
				}
				sel, err := metav1.LabelSelectorAsSelector(&policy.Spec.PodSelector)
				if err == nil && sel.Matches(labels.Set(pod.Labels)) {
					isProtected = true
					break
				}
			}
		}

		if isProtected {
			result.Summary.ProtectedPods++
			pset.protected++
		} else {
			result.Summary.UnprotectedPods++
			podLabels := ""
			if len(pod.Labels) > 0 {
				podLabels = labels.Set(pod.Labels).String()
			}
			result.UnprotectedPods = append(result.UnprotectedPods, NPPodEntry{
				Name:      pod.Name,
				Namespace: pod.Namespace,
				Labels:    podLabels,
				RiskLevel: npPodRisk(pod),
			})
		}
	}

	// Namespace stats
	for _, namespace := range namespaces.Items {
		if namespace.Status.Phase != corev1.NamespaceActive {
			continue
		}
		// Skip system namespaces from count
		if namespace.Name == "kube-system" || namespace.Name == "kube-public" ||
			namespace.Name == "kube-node-lease" || namespace.Name == "default" {
			// Count but don't flag as critical
		}
		result.Summary.TotalNamespaces++

		pset := nsPods[namespace.Name]
		podCount := 0
		protected := 0
		if pset != nil {
			podCount = len(pset.pods)
			protected = pset.protected
		}

		policyCount := 0
		hasDefaultDeny := false
		if nps, ok := nsPolicies[namespace.Name]; ok {
			policyCount = len(nps.policies)
			hasDefaultDeny = nps.hasDefaultDenyIn || nps.hasDefaultDenyEg
			result.Summary.NamespacesWithNP++
		} else {
			if podCount > 0 {
				result.Summary.NamespacesNoNP++
			}
		}

		if hasDefaultDeny {
			result.Summary.DefaultDenyNS++
		}

		nsScore := npNamespaceScore(podCount, protected, policyCount, hasDefaultDeny)
		nsStat := NPNamespaceStat{
			Namespace:      namespace.Name,
			PodCount:       podCount,
			PolicyCount:    policyCount,
			ProtectedPods:  protected,
			HasDefaultDeny: hasDefaultDeny,
			IsolationScore: nsScore,
			RiskLevel:      npNamespaceRisk(nsScore),
		}
		result.ByNamespace = append(result.ByNamespace, nsStat)

		// Issue for namespaces with pods but no policies
		if podCount > 0 && policyCount == 0 {
			result.Issues = append(result.Issues, NPIssue{
				Severity: "warning", Type: "no-network-policy",
				Resource: namespace.Name,
				Message:  fmt.Sprintf("Namespace %s has %d pods but ZERO NetworkPolicies — all traffic is unrestricted", namespace.Name, podCount),
			})
		}
	}

	// Sort
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].IsolationScore < result.ByNamespace[j].IsolationScore
	})
	sort.Slice(result.UnprotectedPods, func(i, j int) bool {
		return npRiskRank(result.UnprotectedPods[i].RiskLevel) < npRiskRank(result.UnprotectedPods[j].RiskLevel)
	})
	sort.Slice(result.AllPolicies, func(i, j int) bool {
		return npRiskRank(result.AllPolicies[i].RiskLevel) < npRiskRank(result.AllPolicies[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return npIssueRank(result.Issues[i].Severity) < npIssueRank(result.Issues[j].Severity)
	})

	result.Summary.IsolationScore = npClusterScore(result.Summary)
	result.Recommendations = npGenRecs(result.Summary, result.ByNamespace, result.PermissivePolicies)

	writeJSON(w, result)
}

// npAnalyzePolicy analyzes a single NetworkPolicy.
func npAnalyzePolicy(policy netv1.NetworkPolicy) NPPolicyEntry {
	entry := NPPolicyEntry{
		Name:         policy.Name,
		Namespace:    policy.Namespace,
		Selector:     policy.Spec.PodSelector.String(),
		IngressRules: len(policy.Spec.Ingress),
		EgressRules:  len(policy.Spec.Egress),
	}

	// Default deny ingress: policy selects pods but has no ingress rules
	// (or policy types include ingress but no rules)
	for _, pt := range policy.Spec.PolicyTypes {
		if pt == netv1.PolicyTypeIngress && len(policy.Spec.Ingress) == 0 {
			entry.HasDefaultDenyIn = true
		}
		if pt == netv1.PolicyTypeEgress && len(policy.Spec.Egress) == 0 {
			entry.HasDefaultDenyEg = true
		}
	}

	// Check for permissive egress (0.0.0.0/0)
	for _, egress := range policy.Spec.Egress {
		for _, peer := range egress.To {
			if peer.IPBlock != nil && peer.IPBlock.CIDR == "0.0.0.0/0" {
				entry.IsPermissive = true
				entry.PermissiveReason = "egress allows 0.0.0.0/0 (all destinations)"
			}
		}
	}

	// Risk level
	entry.RiskLevel = "low"
	if entry.IsPermissive {
		entry.RiskLevel = "high"
	}
	if !entry.HasDefaultDenyIn && !entry.HasDefaultDenyEg && entry.IngressRules == 0 && entry.EgressRules == 0 {
		entry.RiskLevel = "low" // default deny all
	}

	return entry
}

// npPodRisk determines risk for unprotected pods.
func npPodRisk(pod corev1.Pod) string {
	// High-risk: exposed via service or has sensitive labels
	if pod.Namespace == "default" {
		return "high"
	}
	// System namespace pods
	if pod.Namespace == "kube-system" {
		return "low"
	}
	return "medium"
}

// npNamespaceScore computes 0-100.
func npNamespaceScore(podCount, protected, policyCount int, hasDefaultDeny bool) int {
	if podCount == 0 {
		return 100
	}
	if policyCount == 0 {
		return 0 // no policies = no isolation
	}
	coverage := float64(protected) / float64(podCount) * 100
	score := int(coverage)
	if hasDefaultDeny {
		score = (score + 100) / 2 // boost for default deny
	}
	return score
}

// npNamespaceRisk maps score to risk level.
func npNamespaceRisk(score int) string {
	switch {
	case score < 30:
		return "critical"
	case score < 60:
		return "high"
	case score < 85:
		return "medium"
	default:
		return "low"
	}
}

// npClusterScore computes cluster-wide isolation score.
func npClusterScore(s NPASummary) int {
	if s.TotalPods == 0 {
		return 100
	}
	coverage := float64(s.ProtectedPods) / float64(s.TotalPods) * 100
	score := int(coverage)
	score -= s.PermissiveCount * 5
	if score < 0 {
		score = 0
	}
	return score
}

// npGenRecs produces actionable advice.
func npGenRecs(s NPASummary, byNS []NPNamespaceStat, permissive []NPPolicyEntry) []string {
	var recs []string

	if s.NamespacesNoNP > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) have pods but ZERO NetworkPolicies — add default-deny policies immediately", s.NamespacesNoNP))
	}
	if s.UnprotectedPods > 0 {
		pct := float64(s.UnprotectedPods) / float64(s.TotalPods) * 100
		recs = append(recs, fmt.Sprintf("%d of %d pods (%.0f%%) are UNPROTECTED — no NetworkPolicy restricts their traffic", s.UnprotectedPods, s.TotalPods, pct))
	}
	if s.DefaultDenyNS == 0 && s.TotalPolicies > 0 {
		recs = append(recs, "No namespace has default-deny enabled — add deny-all policies to establish isolation baseline")
	} else if s.DefaultDenyNS > 0 {
		recs = append(recs, fmt.Sprintf("%d namespace(s) have default-deny policies — excellent isolation baseline", s.DefaultDenyNS))
	}
	if s.PermissiveCount > 0 {
		top := ""
		if len(permissive) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", permissive[0].Namespace, permissive[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d permissive egress policy(ies) allow 0.0.0.0/0%s — restrict to known destinations", s.PermissiveCount, top))
	}
	if s.IsolationScore < 50 {
		recs = append(recs, fmt.Sprintf("Cluster isolation score is %d/100 — network traffic is largely unrestricted", s.IsolationScore))
	}

	// Find worst namespace
	for _, nsStat := range byNS {
		if nsStat.PodCount > 0 && nsStat.IsolationScore == 0 {
			recs = append(recs, fmt.Sprintf("Namespace '%s' has %d pods with no isolation — highest priority for NetworkPolicy", nsStat.Namespace, nsStat.PodCount))
			break
		}
	}

	return recs
}

func npRiskRank(level string) int {
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

func npIssueRank(s string) int {
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
