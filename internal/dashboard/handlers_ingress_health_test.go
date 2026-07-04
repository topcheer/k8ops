package dashboard

import (
	"sort"
	"testing"
)

func TestAssessIngressStatus(t *testing.T) {
	tests := []struct {
		name   string
		issues []IngressIssue
		expect string
	}{
		{"no-issues", nil, "healthy"},
		{"warning-only", []IngressIssue{{Severity: "warning"}}, "warning"},
		{"has-critical", []IngressIssue{{Severity: "warning"}, {Severity: "critical"}}, "critical"},
		{"multiple-warnings", []IngressIssue{{Severity: "warning"}, {Severity: "warning"}}, "warning"},
	}

	for _, tt := range tests {
		got := assessIngressStatus(len(tt.issues), tt.issues)
		if got != tt.expect {
			t.Errorf("assessIngressStatus(%s) = %q, want %q", tt.name, got, tt.expect)
		}
	}
}

func TestCalculateIngressScore(t *testing.T) {
	// Perfect
	perfect := IngressSummary{
		TotalIngresses: 5,
		IngressWithTLS: 5,
	}
	if score := calculateIngressScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := IngressSummary{
		TotalIngresses: 10,
		NoBackend:      2, // -30
		HostConflicts:  1, // -8
		IngressNoTLS:   3, // -6
		MissingClass:   1, // -5
	}
	// 100 - 30 - 8 - 6 - 5 = 51
	score := calculateIngressScore(withIssues)
	if score != 51 {
		t.Errorf("Expected 51, got %d", score)
	}

	// Floor at 0
	terrible := IngressSummary{
		TotalIngresses: 5,
		NoBackend:      10, // -150
	}
	if score := calculateIngressScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Empty
	empty := IngressSummary{}
	if score := calculateIngressScore(empty); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestGenerateIngressRecommendations(t *testing.T) {
	result := IngressHealthResult{
		Summary: IngressSummary{
			NoBackend:     2,
			HostConflicts: 1,
			MissingClass:  1,
			IngressNoTLS:  3,
			NoRules:       1,
			HealthScore:   45,
		},
	}

	recs := generateIngressRecommendations(result)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundNoBackend := false
	foundConflict := false
	foundNoTLS := false
	for _, r := range recs {
		if containsSubstr(r, "non-existent backend") {
			foundNoBackend = true
		}
		if containsSubstr(r, "conflict") {
			foundConflict = true
		}
		if containsSubstr(r, "TLS") {
			foundNoTLS = true
		}
	}
	if !foundNoBackend {
		t.Error("Expected recommendation about missing backends")
	}
	if !foundConflict {
		t.Error("Expected recommendation about host conflicts")
	}
	if !foundNoTLS {
		t.Error("Expected recommendation about TLS")
	}
}

func TestGenerateIngressRecommendationsClean(t *testing.T) {
	result := IngressHealthResult{
		Summary: IngressSummary{
			TotalIngresses: 5,
			IngressWithTLS: 5,
			HealthScore:    100,
		},
	}

	recs := generateIngressRecommendations(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean ingresses, got %d", len(recs))
	}
}

func TestGetOrCreateIngressNs(t *testing.T) {
	m := make(map[string]*IngressNsStat)

	e1 := getOrCreateIngressNs(m, "default")
	e1.Total = 5

	e2 := getOrCreateIngressNs(m, "default")
	if e2.Total != 5 {
		t.Errorf("Expected same entry with Total=5, got %d", e2.Total)
	}

	e3 := getOrCreateIngressNs(m, "kube-system")
	if e3.Namespace != "kube-system" {
		t.Errorf("Expected kube-system, got %s", e3.Namespace)
	}
}

func TestIngressSeverityRank(t *testing.T) {
	if ingressSeverityRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if ingressSeverityRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if ingressSeverityRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}

func TestIngressIssueSorting(t *testing.T) {
	issues := []IngressIssue{
		{Severity: "warning", Message: "w1"},
		{Severity: "critical", Message: "c1"},
		{Severity: "info", Message: "i1"},
		{Severity: "critical", Message: "c2"},
	}

	sort.Slice(issues, func(i, j int) bool {
		return ingressSeverityRank(issues[i].Severity) < ingressSeverityRank(issues[j].Severity)
	})

	if issues[0].Severity != "critical" {
		t.Errorf("Expected critical first, got %s", issues[0].Severity)
	}
	if issues[3].Severity != "info" {
		t.Errorf("Expected info last, got %s", issues[3].Severity)
	}
}
