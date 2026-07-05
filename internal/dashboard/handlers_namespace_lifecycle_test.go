package dashboard

import (
	"testing"
)

func TestIsNamespaceSystem(t *testing.T) {
	tests := []struct {
		name   string
		expect bool
	}{
		{"kube-system", true},
		{"kube-public", true},
		{"kube-node-lease", true},
		{"default", true},
		{"k8ops-system", true},
		{"kube-something", true},
		{"my-app", false},
		{"production", false},
		{"monitoring", false},
		{"ingress-nginx", true},
	}

	for _, tt := range tests {
		got := isNamespaceSystem(tt.name)
		if got != tt.expect {
			t.Errorf("isSystemNamespace(%q) = %v, want %v", tt.name, got, tt.expect)
		}
	}
}

func TestAssessNamespaceRisk(t *testing.T) {
	// Low risk — has everything
	entry := NamespaceAuditEntry{
		HasResourceQuota: true,
		HasNetworkPolicy: true,
		HasLimitRange:    true,
		Status:           "active",
		HasLabels:        true,
	}
	if level := assessNamespaceRisk(entry, false); level != "low" {
		t.Errorf("Expected low for compliant namespace, got %s", level)
	}

	// Critical — no quota, no netpol, stale
	entry = NamespaceAuditEntry{
		HasResourceQuota: false,   // +15
		HasNetworkPolicy: false,   // +15
		Status:           "stale", // +10
	}
	// 15+15+10 = 40 → critical
	if level := assessNamespaceRisk(entry, false); level != "critical" {
		t.Errorf("Expected critical for non-compliant, got %s", level)
	}

	// System namespace always low
	entry = NamespaceAuditEntry{
		HasResourceQuota: false,
		HasNetworkPolicy: false,
	}
	if level := assessNamespaceRisk(entry, true); level != "low" {
		t.Errorf("Expected low for system namespace, got %s", level)
	}

	// High — no quota + no netpol
	entry = NamespaceAuditEntry{
		HasResourceQuota: false, // +15
		HasNetworkPolicy: false, // +15
		Status:           "active",
		HasLimitRange:    true,
		HasLabels:        true,
	}
	// 15+15 = 30 → critical
	if level := assessNamespaceRisk(entry, false); level != "critical" {
		t.Errorf("Expected critical for no-quota+no-netpol, got %s", level)
	}

	// Medium — no quota only
	entry = NamespaceAuditEntry{
		HasResourceQuota: false, // +15
		HasNetworkPolicy: true,
		Status:           "active",
		HasLimitRange:    true,
		HasLabels:        true,
	}
	if level := assessNamespaceRisk(entry, false); level != "medium" {
		t.Errorf("Expected medium for no-quota only, got %s", level)
	}
}

func TestCalculateNamespaceGovernanceScore(t *testing.T) {
	// Perfect
	perfect := NamespaceLifecycleSummary{
		TotalNamespaces:  10,
		SystemNamespaces: 3,
	}
	if score := calculateNamespaceGovernanceScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := NamespaceLifecycleSummary{
		TotalNamespaces:   10,
		SystemNamespaces:  2,
		WithoutQuota:      4, // 50% of 8 app ns → -10
		WithoutNetPolicy:  4, // 50% → -10
		WithoutLimitRange: 2, // 25% → -2.5
	}
	// 100 - 10 - 10 - 2 (integer math) ≈ 78
	score := calculateNamespaceGovernanceScore(withIssues)
	if score < 70 || score > 80 {
		t.Errorf("Expected ~78, got %d", score)
	}

	// All system namespaces
	allSystem := NamespaceLifecycleSummary{
		TotalNamespaces:  3,
		SystemNamespaces: 3,
	}
	if score := calculateNamespaceGovernanceScore(allSystem); score != 100 {
		t.Errorf("Expected 100 for all system, got %d", score)
	}
}

func TestGenerateNamespaceRecommendations(t *testing.T) {
	s := NamespaceLifecycleSummary{
		TotalNamespaces:   10,
		SystemNamespaces:  2,
		WithoutQuota:      3,
		WithoutNetPolicy:  4,
		WithoutLimitRange: 2,
		StaleNamespaces:   1,
		MissingLabels:     5,
		WithoutDefaultSA:  2,
		GovernanceScore:   45,
	}

	recs := generateNamespaceRecommendations(s)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundQuota := false
	foundNetPol := false
	foundStale := false
	for _, r := range recs {
		if containsSubstr(r, "ResourceQuota") {
			foundQuota = true
		}
		if containsSubstr(r, "NetworkPolicy") {
			foundNetPol = true
		}
		if containsSubstr(r, "stale") {
			foundStale = true
		}
	}
	if !foundQuota {
		t.Error("Expected recommendation about missing ResourceQuota")
	}
	if !foundNetPol {
		t.Error("Expected recommendation about missing NetworkPolicy")
	}
	if !foundStale {
		t.Error("Expected recommendation about stale namespaces")
	}
}

func TestGenerateNamespaceRecommendationsClean(t *testing.T) {
	s := NamespaceLifecycleSummary{
		TotalNamespaces:  10,
		SystemNamespaces: 3,
		GovernanceScore:  100,
	}

	recs := generateNamespaceRecommendations(s)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestNamespaceRiskRank(t *testing.T) {
	if namespaceRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if namespaceRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if namespaceRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if namespaceRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}

func TestNamespaceIssueRank(t *testing.T) {
	if namespaceIssueRank("warning") != 0 {
		t.Error("Expected 0 for warning")
	}
	if namespaceIssueRank("info") != 1 {
		t.Error("Expected 1 for info")
	}
	if namespaceIssueRank("other") != 2 {
		t.Error("Expected 2 for other")
	}
}
