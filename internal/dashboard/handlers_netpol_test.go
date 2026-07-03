package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// --- analyzePolicy tests ---

func TestAnalyzePolicy_AllowAllIngress(t *testing.T) {
	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "deny-all", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			// No ingress rules = allow all ingress
		},
	}

	info := analyzePolicy(pol)
	if !info.AllowsAllIngress {
		t.Error("expected AllowsAllIngress=true for Ingress type with no rules")
	}
	if info.AllowsAllEgress {
		t.Error("expected AllowsAllEgress=false")
	}
}

func TestAnalyzePolicy_AllowAllEgress(t *testing.T) {
	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "egress-open", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
		},
	}

	info := analyzePolicy(pol)
	if !info.AllowsAllEgress {
		t.Error("expected AllowsAllEgress=true")
	}
}

func TestAnalyzePolicy_RestrictedPolicy(t *testing.T) {
	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "restricted", Namespace: "app"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "web"},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": "api"},
						}},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{IPBlock: &networkingv1.IPBlock{CIDR: "10.0.0.0/8"}},
					},
				},
			},
		},
	}

	info := analyzePolicy(pol)
	if info.AllowsAllIngress {
		t.Error("expected AllowsAllIngress=false for restricted policy")
	}
	if info.AllowsAllEgress {
		t.Error("expected AllowsAllEgress=false for restricted policy")
	}
	if info.IngressRules != 1 {
		t.Errorf("IngressRules = %d, want 1", info.IngressRules)
	}
	if info.EgressRules != 1 {
		t.Errorf("EgressRules = %d, want 1", info.EgressRules)
	}
	if info.PodSelector == "" {
		t.Error("expected non-empty PodSelector")
	}
}

func TestAnalyzePolicy_IPBlock0000(t *testing.T) {
	pol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "wide-open", Namespace: "default"},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}},
					},
				},
			},
		},
	}

	info := analyzePolicy(pol)
	if !info.AllowsAllIngress {
		t.Error("expected AllowsAllIngress=true for 0.0.0.0/0 ipBlock")
	}
}

// --- selectorToString tests ---

func TestSelectorToString_Empty(t *testing.T) {
	sel := metav1.LabelSelector{}
	result := selectorToString(sel)
	if result != "all pods (empty selector)" {
		t.Errorf("got %q, want 'all pods (empty selector)'", result)
	}
}

func TestSelectorToString_MatchLabels(t *testing.T) {
	sel := metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "web", "tier": "frontend"},
	}
	result := selectorToString(sel)
	if result == "" {
		t.Error("expected non-empty selector string")
	}
}

// --- podMatchesSelector tests ---

func TestPodMatchesSelector_EmptySelector(t *testing.T) {
	pod := corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	sel := metav1.LabelSelector{}
	if !podMatchesSelector(pod, sel) {
		t.Error("empty selector should match all pods")
	}
}

func TestPodMatchesSelector_MatchLabels(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "web-1",
			Labels: map[string]string{"app": "web"},
		},
	}
	sel := metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "web"},
	}
	if !podMatchesSelector(pod, sel) {
		t.Error("pod with matching label should match")
	}

	sel2 := metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "api"},
	}
	if podMatchesSelector(pod, sel2) {
		t.Error("pod with non-matching label should not match")
	}
}

func TestPodMatchesSelector_InExpression(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"env": "prod"},
		},
	}
	sel := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "env", Operator: metav1.LabelSelectorOpIn, Values: []string{"prod", "staging"}},
		},
	}
	if !podMatchesSelector(pod, sel) {
		t.Error("pod label in In expression should match")
	}
}

func TestPodMatchesSelector_NotInExpression(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"env": "prod"},
		},
	}
	sel := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "env", Operator: metav1.LabelSelectorOpNotIn, Values: []string{"dev", "test"}},
		},
	}
	if !podMatchesSelector(pod, sel) {
		t.Error("pod label not in NotIn expression should match")
	}
}

func TestPodMatchesSelector_Exists(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "web"},
		},
	}
	sel := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "app", Operator: metav1.LabelSelectorOpExists},
		},
	}
	if !podMatchesSelector(pod, sel) {
		t.Error("Exists operator should match when label exists")
	}
}

func TestPodMatchesSelector_DoesNotExist(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": "web"},
		},
	}
	sel := metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: "env", Operator: metav1.LabelSelectorOpDoesNotExist},
		},
	}
	if !podMatchesSelector(pod, sel) {
		t.Error("DoesNotExist operator should match when label is absent")
	}
}

// --- auditNetworkPolicies tests ---

func TestAuditNetworkPolicies_NoPolicy(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "prod"}},
	}

	result := auditNetworkPoliciesFull(namespaces, pods, nil)

	if result.Summary.NamespacesWithout != 2 {
		t.Errorf("NamespacesWithout = %d, want 2", result.Summary.NamespacesWithout)
	}
	if result.Summary.CriticalCount != 2 {
		t.Errorf("CriticalCount = %d, want 2 (no policies)", result.Summary.CriticalCount)
	}
}

func TestAuditNetworkPolicies_FullCoverage(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "app", Labels: map[string]string{"app": "web"}}},
	}
	policies := []networkingv1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "deny-all", Namespace: "app"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress,
				},
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					{From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}},
					}},
				},
				Egress: []networkingv1.NetworkPolicyEgressRule{
					{To: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}},
					}},
				},
			},
		},
	}

	result := auditNetworkPoliciesFull(namespaces, pods, policies)

	if result.Summary.NamespacesWithout != 0 {
		t.Errorf("NamespacesWithout = %d, want 0", result.Summary.NamespacesWithout)
	}
	if result.Summary.CriticalCount != 0 {
		t.Errorf("CriticalCount = %d, want 0", result.Summary.CriticalCount)
	}
	// No allow-all since we have rules
	permissive := 0
	for _, p := range result.Policies {
		if p.AllowsAllIngress || p.AllowsAllEgress {
			permissive++
		}
	}
	if permissive != 0 {
		t.Errorf("permissive policies = %d, want 0", permissive)
	}
}

func TestAuditNetworkPolicies_PartialCoverage(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "default", Labels: map[string]string{"app": "web"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "db-1", Namespace: "default", Labels: map[string]string{"app": "db"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "cache-1", Namespace: "default", Labels: map[string]string{"app": "cache"}}},
	}
	policies := []networkingv1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "web-policy", Namespace: "default"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "web"},
				},
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					{From: []networkingv1.NetworkPolicyPeer{
						{PodSelector: &metav1.LabelSelector{}},
					}},
				},
			},
		},
	}

	result := auditNetworkPoliciesFull(namespaces, pods, policies)

	// 2 pods (db, cache) should be uncovered
	nsInfo := result.Namespaces[0]
	if nsInfo.ExposedPods != 2 {
		t.Errorf("ExposedPods = %d, want 2", nsInfo.ExposedPods)
	}
}

func TestAuditNetworkPolicies_SkipsSystemNamespaces(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "kube-system"}},
	}

	result := auditNetworkPoliciesFull(namespaces, pods, nil)

	// kube-system should be skipped, only default audited
	if result.Summary.TotalNamespaces != 1 {
		t.Errorf("TotalNamespaces = %d, want 1 (kube-system skipped)", result.Summary.TotalNamespaces)
	}
}

func TestAuditNetworkPolicies_IPBlockWideOpen(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "app", Labels: map[string]string{"x": "y"}}},
	}
	policies := []networkingv1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "wide-open", Namespace: "app"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
				Ingress: []networkingv1.NetworkPolicyIngressRule{
					{From: []networkingv1.NetworkPolicyPeer{
						{IPBlock: &networkingv1.IPBlock{CIDR: "0.0.0.0/0"}},
					}},
				},
			},
		},
	}

	result := auditNetworkPoliciesFull(namespaces, pods, policies)

	// Should have a high-severity finding for allowing all ingress
	found := false
	for _, f := range result.Findings {
		if f.Severity == "high" && f.Category == "permissive" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected high-severity finding for 0.0.0.0/0 policy")
	}
}

func TestAuditNetworkPolicies_PermissivePolicyCount(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "app"}},
	}
	policies := []networkingv1.NetworkPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "allow-all", Namespace: "app"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []networkingv1.PolicyType{
					networkingv1.PolicyTypeIngress,
					networkingv1.PolicyTypeEgress,
				},
				// No rules = allow all
			},
		},
	}

	result := auditNetworkPoliciesFull(namespaces, pods, policies)

	if result.Summary.PermissivePolicies != 1 {
		t.Errorf("PermissivePolicies = %d, want 1", result.Summary.PermissivePolicies)
	}
}

func TestSeverityScore(t *testing.T) {
	if severityScore("critical") >= severityScore("high") {
		t.Error("critical should score lower than high")
	}
	if severityScore("high") >= severityScore("medium") {
		t.Error("high should score lower than medium")
	}
}

// --- Handler integration ---

func TestHandleNetPolAudit_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/security/network-policies", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleNetPolAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestHandleNetPolAudit_WithData(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-1", Namespace: "app", Labels: map[string]string{"app": "web"}}},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/network-policies", clientset)
	rr := httptest.NewRecorder()

	srv := &Server{}
	srv.handleNetPolAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("HTTP status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "\"findings\"") {
		t.Error("response missing findings array")
	}
	if !strings.Contains(body, "\"summary\"") {
		t.Error("response missing summary")
	}
}
