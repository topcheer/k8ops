package dashboard

import (
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
)

func TestHasWildcardVerb(t *testing.T) {
	tests := []struct {
		name  string
		rules []rbacv1.PolicyRule
		want  bool
	}{
		{"wildcard verb", []rbacv1.PolicyRule{{Verbs: []string{"*"}}}, true},
		{"specific verbs", []rbacv1.PolicyRule{{Verbs: []string{"get", "list"}}}, false},
		{"mixed", []rbacv1.PolicyRule{{Verbs: []string{"get"}}, {Verbs: []string{"*"}}}, true},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasWildcardVerb(tt.rules); got != tt.want {
				t.Errorf("hasWildcardVerb() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasWildcardResource(t *testing.T) {
	tests := []struct {
		name  string
		rules []rbacv1.PolicyRule
		want  bool
	}{
		{"wildcard resource", []rbacv1.PolicyRule{{Resources: []string{"*"}}}, true},
		{"specific resources", []rbacv1.PolicyRule{{Resources: []string{"pods", "services"}}}, false},
		{"nonResourceURL wildcard", []rbacv1.PolicyRule{{NonResourceURLs: []string{"/*"}}}, true},
		{"empty", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasWildcardResource(tt.rules); got != tt.want {
				t.Errorf("hasWildcardResource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSystemRole(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"system:coredns", true},
		{"cluster-admin", true},
		{"admin", true},
		{"view", true},
		{"my-custom-role", false},
		{"my-app-reader", false},
	}
	for _, tt := range tests {
		if got := isSystemRole(tt.name); got != tt.want {
			t.Errorf("isSystemRole(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRBACScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  RBACAuditSummary
		minScore int
		maxScore int
	}{
		{"clean", RBACAuditSummary{}, 95, 100},
		{"cluster-admin bindings", RBACAuditSummary{ClusterAdminBindings: 2}, 60, 75},
		{"wildcards", RBACAuditSummary{WildcardVerbCount: 5, WildcardResourceCount: 3}, 30, 55},
		{"severe", RBACAuditSummary{ClusterAdminBindings: 3, WildcardVerbCount: 8, WildcardResourceCount: 5}, 0, 25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := rbacAuditScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestRBACRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &RBACAuditResult{Summary: RBACAuditSummary{}}
		recs := rbacAuditRecs(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &RBACAuditResult{Summary: RBACAuditSummary{
			ClusterAdminBindings: 2, WildcardVerbCount: 3,
			WildcardResourceCount: 1, OverprivilegedCount: 5,
		}}
		recs := rbacAuditRecs(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
