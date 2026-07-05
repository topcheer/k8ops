package dashboard

import (
	"testing"
)

func TestHasVerb(t *testing.T) {
	if !hasVerb([]string{"get", "list", "watch"}, "get") {
		t.Error("Expected true for 'get'")
	}
	if hasVerb([]string{"get", "list"}, "create") {
		t.Error("Expected false for 'create'")
	}
	if !hasVerb([]string{"get", "*"}, "*") {
		t.Error("Expected true for '*'")
	}
	if hasVerb(nil, "get") {
		t.Error("Expected false for nil")
	}
}

func TestHasResource(t *testing.T) {
	if !hasResource([]string{"pods", "services"}, "pods") {
		t.Error("Expected true for 'pods'")
	}
	if hasResource([]string{"pods", "services"}, "secrets") {
		t.Error("Expected false for 'secrets'")
	}
	if !hasResource([]string{"pods", "*"}, "*") {
		t.Error("Expected true for '*'")
	}
	if hasResource(nil, "pods") {
		t.Error("Expected false for nil")
	}
}

func TestAssessRBACSubjectRisk(t *testing.T) {
	// Cluster admin
	entry := RBACSubjectEntry{IsClusterAdmin: true}
	if level := assessRBACSubjectRisk(entry); level != "critical" {
		t.Errorf("Expected critical for cluster-admin, got %s", level)
	}

	// Can escalate + wildcard
	entry = RBACSubjectEntry{
		HasWildcard: true,
		CanEscalate: true, // +25
	}
	// 20 + 25 = 45 → critical
	if level := assessRBACSubjectRisk(entry); level != "critical" {
		t.Errorf("Expected critical for wildcard+escalate, got %s", level)
	}

	// Secret reader + exec
	entry = RBACSubjectEntry{
		CanReadSecrets: true, // +10
		CanExec:        true, // +10
	}
	// 10 + 10 = 20 → high
	if level := assessRBACSubjectRisk(entry); level != "high" {
		t.Errorf("Expected high for secrets+exec, got %s", level)
	}

	// Just secret reader
	entry = RBACSubjectEntry{
		CanReadSecrets: true, // +10
	}
	// 10 → medium
	if level := assessRBACSubjectRisk(entry); level != "medium" {
		t.Errorf("Expected medium for secrets only, got %s", level)
	}

	// Clean
	entry = RBACSubjectEntry{}
	if level := assessRBACSubjectRisk(entry); level != "low" {
		t.Errorf("Expected low for clean, got %s", level)
	}
}

func TestCalculateRBACEffectiveScore(t *testing.T) {
	// Perfect
	perfect := RBACEffectiveSummary{TotalSubjects: 10}
	if score := calculateRBACEffectiveScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := RBACEffectiveSummary{
		TotalSubjects:   20,
		ClusterAdmins:   2, // -16
		WithWildcards:   3, // -15
		EscalationPaths: 1, // -10
	}
	// 100 - 16 - 15 - 10 = 59
	score := calculateRBACEffectiveScore(withIssues)
	if score != 59 {
		t.Errorf("Expected 59, got %d", score)
	}

	// Floor at 0
	terrible := RBACEffectiveSummary{
		TotalSubjects: 5,
		ClusterAdmins: 10, // -80
		WithWildcards: 10, // -50
	}
	if score := calculateRBACEffectiveScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Empty
	if score := calculateRBACEffectiveScore(RBACEffectiveSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestGenerateRBACEffectiveRecs(t *testing.T) {
	s := RBACEffectiveSummary{
		ClusterAdmins:   2,
		EscalationPaths: 1,
		WithWildcards:   3,
		SecretReaders:   5,
		ExecAccess:      4,
		SecurityScore:   35,
	}

	recs := generateRBACEffectiveRecs(s)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundAdmin := false
	foundEscalation := false
	foundWildcard := false
	for _, r := range recs {
		if containsSubstr(r, "cluster-admin") {
			foundAdmin = true
		}
		if containsSubstr(r, "escalation") {
			foundEscalation = true
		}
		if containsSubstr(r, "wildcard") {
			foundWildcard = true
		}
	}
	if !foundAdmin {
		t.Error("Expected recommendation about cluster-admin")
	}
	if !foundEscalation {
		t.Error("Expected recommendation about escalation")
	}
	if !foundWildcard {
		t.Error("Expected recommendation about wildcards")
	}
}

func TestGenerateRBACEffectiveRecsClean(t *testing.T) {
	s := RBACEffectiveSummary{
		TotalSubjects: 10,
		SecurityScore: 100,
	}

	recs := generateRBACEffectiveRecs(s)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestRBACSubjectRiskRank(t *testing.T) {
	if rbacSubjectRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if rbacSubjectRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if rbacSubjectRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if rbacSubjectRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}

func TestRBACIssueRank(t *testing.T) {
	if rbacIssueRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if rbacIssueRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if rbacIssueRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}
