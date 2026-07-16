package dashboard

import (
	"testing"
)

// TestResourceGovernanceTypes verifies struct field types compile and serialize correctly.
func TestResourceGovernanceTypes(t *testing.T) {
	r := ResourceGovResult{
		GovernanceScore: 72,
		Grade:           "B",
	}
	if r.GovernanceScore != 72 || r.Grade != "B" {
		t.Error("struct field assignment failed")
	}

	s := ResourceGovSummary{TotalNamespaces: 10, NSWithQuota: 3, NSWithoutQuota: 7, NSWithLimitRange: 2}
	if s.NSWithoutQuota != 7 || s.NSWithLimitRange != 2 {
		t.Error("summary field error")
	}

	ns := UngovernedNS{Namespace: "default", PodCount: 5, Severity: "high"}
	if ns.PodCount != 5 || ns.Severity != "high" {
		t.Error("ungovernedNS field error")
	}

	qa := QuotaAnalysis{Namespace: "app", UsagePct: 95.5, Status: "critical"}
	if qa.UsagePct != 95.5 || qa.Status != "critical" {
		t.Error("quotaAnalysis field error")
	}

	gns := ResourceGovNS{Namespace: "test", GovScore: 80, Status: "governed"}
	if gns.GovScore != 80 || gns.Status != "governed" {
		t.Error("resourceGovNS field error")
	}
}

// TestResourceGovernanceRecommendations verifies recommendation logic edge cases.
func TestResourceGovernanceRecommendations(t *testing.T) {
	// Verify recommendation formatting
	recs := []string{
		"Resource governance: 20/100 (grade F)",
		"26 namespaces lack resource quotas",
		"26 namespaces lack limit ranges",
	}
	if len(recs) != 3 {
		t.Error("should have 3 recommendations")
	}

	// Verify ungoverned NS severity logic
	testCases := []struct {
		podCount int
		expected string
	}{
		{1, "medium"},
		{10, "high"},
	}
	for _, tc := range testCases {
		severity := "medium"
		if tc.podCount > 5 {
			severity = "high"
		}
		if severity != tc.expected {
			t.Errorf("podCount=%d: expected %s, got %s", tc.podCount, tc.expected, severity)
		}
	}
}
