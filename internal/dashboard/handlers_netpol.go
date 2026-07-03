package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NetPolFinding represents a single network policy audit finding.
type NetPolFinding struct {
	Severity       string `json:"severity"` // critical, high, medium, low
	Category       string `json:"category"` // no-policy, permissive, exposed, ingress-open, egress-open
	Namespace      string `json:"namespace"`
	Resource       string `json:"resource"` // affected workload or namespace
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
}

// NetPolPolicyInfo describes a single NetworkPolicy.
type NetPolPolicyInfo struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	PodSelector      string   `json:"podSelector"`
	Types            []string `json:"types"`
	IngressRules     int      `json:"ingressRules"`
	EgressRules      int      `json:"egressRules"`
	AllowsAllIngress bool     `json:"allowsAllIngress"`
	AllowsAllEgress  bool     `json:"allowsAllEgress"`
}

// NetPolAuditResult is the full audit output.
type NetPolAuditResult struct {
	ScannedAt  time.Time          `json:"scannedAt"`
	Summary    NetPolAuditSummary `json:"summary"`
	Policies   []NetPolPolicyInfo `json:"policies"`
	Findings   []NetPolFinding    `json:"findings"`
	Namespaces []NetPolNamespace  `json:"namespaces"`
}

// NetPolAuditSummary is the aggregate result.
type NetPolAuditSummary struct {
	TotalNamespaces      int `json:"totalNamespaces"`
	NamespacesWithNetPol int `json:"namespacesWithNetPol"`
	NamespacesWithout    int `json:"namespacesWithout"`
	TotalPolicies        int `json:"totalPolicies"`
	PermissivePolicies   int `json:"permissivePolicies"`
	TotalFindings        int `json:"totalFindings"`
	CriticalCount        int `json:"criticalCount"`
	HighCount            int `json:"highCount"`
	MediumCount          int `json:"mediumCount"`
	LowCount             int `json:"lowCount"`
}

// NetPolNamespace shows network policy coverage per namespace.
type NetPolNamespace struct {
	Name             string `json:"name"`
	HasNetworkPolicy bool   `json:"hasNetworkPolicy"`
	PolicyCount      int    `json:"policyCount"`
	PodCount         int    `json:"podCount"`
	ExposedPods      int    `json:"exposedPods"`
}

// handleNetPolAudit scans the cluster for NetworkPolicy coverage and isolation gaps.
// GET /api/security/network-policies
func (s *Server) handleNetPolAudit(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)

	nsList, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	podList, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	netpolList, err := rc.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := auditNetworkPoliciesFull(nsList.Items, podList.Items, netpolList.Items)
	writeJSON(w, result)
}

// auditNetworkPoliciesFull performs the full network policy audit.
func auditNetworkPoliciesFull(namespaces []corev1.Namespace, pods []corev1.Pod, policies []networkingv1.NetworkPolicy) NetPolAuditResult {
	// Skip system namespaces
	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// Group policies by namespace
	policiesByNS := make(map[string][]networkingv1.NetworkPolicy)
	for _, p := range policies {
		policiesByNS[p.Namespace] = append(policiesByNS[p.Namespace], p)
	}

	// Group pods by namespace
	podsByNS := make(map[string][]corev1.Pod)
	for _, p := range pods {
		podsByNS[p.Namespace] = append(podsByNS[p.Namespace], p)
	}

	var allFindings []NetPolFinding
	var policyInfos []NetPolPolicyInfo
	var nsInfos []NetPolNamespace

	for _, ns := range namespaces {
		if systemNS[ns.Name] {
			continue
		}

		nsPolicies := policiesByNS[ns.Name]
		nsPods := podsByNS[ns.Name]
		podCount := len(nsPods)
		hasNetPol := len(nsPolicies) > 0

		// Count pods matched by at least one policy
		matchedPods := 0
		if hasNetPol {
			for _, p := range nsPods {
				if podMatchesAnyPolicy(p, nsPolicies) {
					matchedPods++
				}
			}
		}
		exposedPods := podCount - matchedPods

		nsInfos = append(nsInfos, NetPolNamespace{
			Name:             ns.Name,
			HasNetworkPolicy: hasNetPol,
			PolicyCount:      len(nsPolicies),
			PodCount:         podCount,
			ExposedPods:      exposedPods,
		})

		// Finding: namespace with pods but no NetworkPolicy
		if !hasNetPol && podCount > 0 {
			allFindings = append(allFindings, NetPolFinding{
				Severity:       "critical",
				Category:       "no-policy",
				Namespace:      ns.Name,
				Resource:       ns.Name,
				Description:    fmt.Sprintf("命名空间 %s 有 %d 个 Pod 但没有 NetworkPolicy，所有流量不受限制", ns.Name, podCount),
				Recommendation: "创建默认拒绝 NetworkPolicy，仅放行必要的 ingress/egress 流量",
			})
		}

		// Finding: namespace with NetworkPolicy but some pods not covered
		if hasNetPol && exposedPods > 0 {
			sev := "medium"
			if exposedPods > podCount/2 {
				sev = "high"
			}
			allFindings = append(allFindings, NetPolFinding{
				Severity:       sev,
				Category:       "partial-coverage",
				Namespace:      ns.Name,
				Resource:       fmt.Sprintf("%d/%d pods uncovered", exposedPods, podCount),
				Description:    fmt.Sprintf("命名空间 %s 有 %d/%d 个 Pod 不被任何 NetworkPolicy 覆盖", ns.Name, exposedPods, podCount),
				Recommendation: "扩大 NetworkPolicy 的 podSelector 覆盖范围，或添加 catch-all 策略",
			})
		}

		// Analyze each policy
		for _, pol := range nsPolicies {
			info := analyzePolicy(&pol)
			policyInfos = append(policyInfos, info)

			// Finding: policy that allows all ingress
			if info.AllowsAllIngress {
				allFindings = append(allFindings, NetPolFinding{
					Severity:       "high",
					Category:       "permissive",
					Namespace:      ns.Name,
					Resource:       pol.Name,
					Description:    fmt.Sprintf("NetworkPolicy %s/%s 允许所有入站流量 (0.0.0.0/0)", ns.Name, pol.Name),
					Recommendation: "限制入站来源 IP 范围或使用 namespaceSelector/podSelector",
				})
			}

			// Finding: policy that allows all egress
			if info.AllowsAllEgress {
				allFindings = append(allFindings, NetPolFinding{
					Severity:       "medium",
					Category:       "permissive",
					Namespace:      ns.Name,
					Resource:       pol.Name,
					Description:    fmt.Sprintf("NetworkPolicy %s/%s 允许所有出站流量", ns.Name, pol.Name),
					Recommendation: "限制出站目标，仅放行 DNS (UDP 53) 和必要的外部服务",
				})
			}

			// Finding: policy with no types specified (defaults)
			if len(pol.Spec.PolicyTypes) == 0 {
				allFindings = append(allFindings, NetPolFinding{
					Severity:       "low",
					Category:       "implicit-types",
					Namespace:      ns.Name,
					Resource:       pol.Name,
					Description:    fmt.Sprintf("NetworkPolicy %s/%s 未显式指定 policyTypes，将使用默认值", ns.Name, pol.Name),
					Recommendation: "显式声明 policyTypes: [Ingress, Egress] 以明确意图",
				})
			}
		}
	}

	// Sort findings by severity
	sort.Slice(allFindings, func(i, j int) bool {
		return severityScore(allFindings[i].Severity) < severityScore(allFindings[j].Severity)
	})

	// Sort policies by namespace
	sort.Slice(policyInfos, func(i, j int) bool {
		if policyInfos[i].Namespace != policyInfos[j].Namespace {
			return policyInfos[i].Namespace < policyInfos[j].Namespace
		}
		return policyInfos[i].Name < policyInfos[j].Name
	})

	// Build summary
	summary := NetPolAuditSummary{
		TotalNamespaces: len(nsInfos),
	}
	for _, n := range nsInfos {
		if n.HasNetworkPolicy {
			summary.NamespacesWithNetPol++
		} else {
			summary.NamespacesWithout++
		}
	}
	summary.TotalPolicies = len(policyInfos)
	for _, p := range policyInfos {
		if p.AllowsAllIngress || p.AllowsAllEgress {
			summary.PermissivePolicies++
		}
	}
	summary.TotalFindings = len(allFindings)
	for _, f := range allFindings {
		switch f.Severity {
		case "critical":
			summary.CriticalCount++
		case "high":
			summary.HighCount++
		case "medium":
			summary.MediumCount++
		case "low":
			summary.LowCount++
		}
	}

	return NetPolAuditResult{
		ScannedAt:  time.Now(),
		Summary:    summary,
		Policies:   policyInfos,
		Findings:   allFindings,
		Namespaces: nsInfos,
	}
}

// analyzePolicy extracts metadata from a NetworkPolicy.
func analyzePolicy(pol *networkingv1.NetworkPolicy) NetPolPolicyInfo {
	info := NetPolPolicyInfo{
		Name:        pol.Name,
		Namespace:   pol.Namespace,
		PodSelector: selectorToString(pol.Spec.PodSelector),
	}

	for _, t := range pol.Spec.PolicyTypes {
		info.Types = append(info.Types, string(t))
	}

	info.IngressRules = len(pol.Spec.Ingress)
	info.EgressRules = len(pol.Spec.Egress)

	// Check for allow-all-ingress: policy has Ingress type but no ingress rules
	for _, t := range pol.Spec.PolicyTypes {
		if t == networkingv1.PolicyTypeIngress && len(pol.Spec.Ingress) == 0 {
			info.AllowsAllIngress = true
		}
		if t == networkingv1.PolicyTypeEgress && len(pol.Spec.Egress) == 0 {
			info.AllowsAllEgress = true
		}
	}

	// Also check for ipBlock 0.0.0.0/0 in ingress rules
	for _, rule := range pol.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.IPBlock != nil && peer.IPBlock.CIDR == "0.0.0.0/0" {
				info.AllowsAllIngress = true
			}
		}
	}

	return info
}

// selectorToString converts a LabelSelector to a readable string.
func selectorToString(sel metav1.LabelSelector) string {
	if len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0 {
		return "all pods (empty selector)"
	}
	if len(sel.MatchLabels) > 0 {
		parts := make([]string, 0, len(sel.MatchLabels))
		for k, v := range sel.MatchLabels {
			parts = append(parts, k+"="+v)
		}
		sort.Strings(parts)
		return "labels: " + joinStrings(parts, ", ")
	}
	return "complex selector"
}

// joinStrings joins string slice with separator.
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}

// podMatchesAnyPolicy checks if a pod is selected by at least one policy.
func podMatchesAnyPolicy(pod corev1.Pod, policies []networkingv1.NetworkPolicy) bool {
	for _, pol := range policies {
		if podMatchesSelector(pod, pol.Spec.PodSelector) {
			return true
		}
	}
	return false
}

// podMatchesSelector checks if a pod's labels match a LabelSelector.
func podMatchesSelector(pod corev1.Pod, sel metav1.LabelSelector) bool {
	// Empty selector matches all pods
	if len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0 {
		return true
	}

	// Check MatchLabels (all must match)
	for k, v := range sel.MatchLabels {
		if pod.Labels[k] != v {
			return false
		}
	}

	// Check MatchExpressions
	for _, expr := range sel.MatchExpressions {
		if !matchExpression(pod.Labels, expr) {
			return false
		}
	}

	return true
}

// matchExpression evaluates a single LabelSelectorRequirement.
func matchExpression(labels map[string]string, expr metav1.LabelSelectorRequirement) bool {
	val, exists := labels[expr.Key]

	switch expr.Operator {
	case metav1.LabelSelectorOpIn:
		if !exists {
			return false
		}
		for _, v := range expr.Values {
			if val == v {
				return true
			}
		}
		return false

	case metav1.LabelSelectorOpNotIn:
		if !exists {
			return true
		}
		for _, v := range expr.Values {
			if val == v {
				return false
			}
		}
		return true

	case metav1.LabelSelectorOpExists:
		return exists

	case metav1.LabelSelectorOpDoesNotExist:
		return !exists
	}

	return false
}

// severityScore returns sort priority for severity (lower = more urgent).
func severityScore(sev string) int {
	switch sev {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

// Ensure intstr is used (for potential future port matching)
var _ = intstr.FromInt
