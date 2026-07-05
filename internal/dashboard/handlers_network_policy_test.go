package dashboard

import (
	"testing"

	netv1 "k8s.io/api/networking/v1"
)

func TestNPAnalyzePolicy(t *testing.T) {
	// Default deny ingress (no rules, ingress in policyTypes)
	policy := netv1.NetworkPolicy{
		Spec: netv1.NetworkPolicySpec{
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress},
			Ingress:     []netv1.NetworkPolicyIngressRule{},
		},
	}
	entry := npAnalyzePolicy(policy)
	if !entry.HasDefaultDenyIn {
		t.Error("Expected default deny ingress")
	}

	// Permissive egress (0.0.0.0/0)
	policy = netv1.NetworkPolicy{
		Spec: netv1.NetworkPolicySpec{
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeEgress},
			Egress: []netv1.NetworkPolicyEgressRule{
				{
					To: []netv1.NetworkPolicyPeer{
						{IPBlock: &netv1.IPBlock{CIDR: "0.0.0.0/0"}},
					},
				},
			},
		},
	}
	entry = npAnalyzePolicy(policy)
	if !entry.IsPermissive {
		t.Error("Expected permissive egress")
	}
	if entry.RiskLevel != "high" {
		t.Errorf("Expected high risk, got %s", entry.RiskLevel)
	}

	// Default deny all
	policy = netv1.NetworkPolicy{
		Spec: netv1.NetworkPolicySpec{
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
		},
	}
	entry = npAnalyzePolicy(policy)
	if !entry.HasDefaultDenyIn || !entry.HasDefaultDenyEg {
		t.Error("Expected default deny both directions")
	}
}

func TestNPNamespaceScore(t *testing.T) {
	// No pods
	if score := npNamespaceScore(0, 0, 0, false); score != 100 {
		t.Errorf("Expected 100 for empty NS, got %d", score)
	}

	// Pods but no policies
	if score := npNamespaceScore(5, 0, 0, false); score != 0 {
		t.Errorf("Expected 0 for no policies, got %d", score)
	}

	// Partial coverage
	if score := npNamespaceScore(10, 5, 3, false); score != 50 {
		t.Errorf("Expected 50 for 50%% coverage, got %d", score)
	}

	// Full coverage + default deny
	score := npNamespaceScore(10, 10, 2, true)
	if score != 100 {
		t.Errorf("Expected 100 for full+defaultDeny, got %d", score)
	}

	// Partial + default deny (boost)
	score = npNamespaceScore(10, 5, 2, true)
	// coverage=50, boost: (50+100)/2 = 75
	if score != 75 {
		t.Errorf("Expected 75 for partial+defaultDeny, got %d", score)
	}
}

func TestNPNamespaceRisk(t *testing.T) {
	if level := npNamespaceRisk(10); level != "critical" {
		t.Errorf("Expected critical for 10, got %s", level)
	}
	if level := npNamespaceRisk(40); level != "high" {
		t.Errorf("Expected high for 40, got %s", level)
	}
	if level := npNamespaceRisk(70); level != "medium" {
		t.Errorf("Expected medium for 70, got %s", level)
	}
	if level := npNamespaceRisk(90); level != "low" {
		t.Errorf("Expected low for 90, got %s", level)
	}
}

func TestNPClusterScore(t *testing.T) {
	// No pods
	if score := npClusterScore(NPASummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// 50% coverage
	s := NPASummary{TotalPods: 10, ProtectedPods: 5}
	if score := npClusterScore(s); score != 50 {
		t.Errorf("Expected 50, got %d", score)
	}

	// With permissive penalty
	s = NPASummary{TotalPods: 10, ProtectedPods: 8, PermissiveCount: 3}
	// 80 - 15 = 65
	if score := npClusterScore(s); score != 65 {
		t.Errorf("Expected 65, got %d", score)
	}
}

func TestNPGenRecs(t *testing.T) {
	s := NPASummary{
		TotalPods:       20,
		ProtectedPods:   8,
		UnprotectedPods: 12,
		NamespacesNoNP:  3,
		DefaultDenyNS:   0,
		PermissiveCount: 2,
		IsolationScore:  40,
	}
	byNS := []NPNamespaceStat{
		{Namespace: "app1", PodCount: 5, IsolationScore: 0},
	}
	permissive := []NPPolicyEntry{
		{Namespace: "app1", Name: "allow-all"},
	}

	recs := npGenRecs(s, byNS, permissive)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundNoNP := false
	foundUnprot := false
	foundPermissive := false
	for _, r := range recs {
		if containsSubstr(r, "ZERO NetworkPolicies") {
			foundNoNP = true
		}
		if containsSubstr(r, "UNPROTECTED") {
			foundUnprot = true
		}
		if containsSubstr(r, "permissive") {
			foundPermissive = true
		}
	}
	if !foundNoNP {
		t.Error("Expected recommendation about no NetworkPolicies")
	}
	if !foundUnprot {
		t.Error("Expected recommendation about unprotected pods")
	}
	if !foundPermissive {
		t.Error("Expected recommendation about permissive policies")
	}
}

func TestNPRiskRank(t *testing.T) {
	if npRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if npRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if npRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if npRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestNPIssueRank(t *testing.T) {
	if npIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if npIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if npIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
