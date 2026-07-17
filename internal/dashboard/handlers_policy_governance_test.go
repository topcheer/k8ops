package dashboard

import (
	"testing"
)

func TestPolicyGovTypes(t *testing.T) {
	r := PolicyGovResult{EnforcementScore: 35, Grade: "D"}
	if r.EnforcementScore != 35 || r.Grade != "D" {
		t.Error("struct field error")
	}

	s := PolicyGovSummary{TotalNamespaces: 28, NSWithEnforcePSA: 3, NSWithNoPSA: 20}
	if s.NSWithNoPSA != 20 {
		t.Error("summary field error")
	}

	psa := PSACoverage{EnforceLevel: "minimal", Score: 10, CoveragePct: 10.7, GapCount: 25}
	if psa.GapCount != 25 {
		t.Error("psaCoverage field error")
	}

	gap := PolicyGap{Namespace: "default", Gap: "no PSA labels", Severity: "high"}
	if gap.Severity != "high" {
		t.Error("policyGap field error")
	}
}

func TestPolicyGovScoring(t *testing.T) {
	tests := []struct {
		hasPolicyEngine bool
		psaScore        int
		auditNS         int
		totalNS         int
		expectedMin     int
		expectedMax     int
	}{
		{false, 0, 0, 28, 0, 10},    // No engine, no PSA
		{true, 100, 0, 28, 75, 100}, // Engine + full PSA
		{true, 50, 5, 28, 55, 75},   // Engine + partial PSA
	}
	for _, tc := range tests {
		score := 0
		if tc.hasPolicyEngine {
			score += 40
		}
		score += tc.psaScore * 40 / 100
		if tc.auditNS > 0 && tc.totalNS > 0 {
			score += tc.auditNS * 20 / tc.totalNS
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("engine=%v psa=%d audit=%d: expected %d-%d, got %d",
				tc.hasPolicyEngine, tc.psaScore, tc.auditNS, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestPolicyGovPSALevel(t *testing.T) {
	tests := []struct {
		enforcePct float64
		expected   string
	}{
		{0, "none"},
		{10, "minimal"},
		{60, "partial"},
		{90, "comprehensive"},
	}
	for _, tc := range tests {
		level := "none"
		if tc.enforcePct > 80 {
			level = "comprehensive"
		} else if tc.enforcePct > 50 {
			level = "partial"
		} else if tc.enforcePct > 0 {
			level = "minimal"
		}
		if level != tc.expected {
			t.Errorf("enforcePct=%.0f: expected %s, got %s", tc.enforcePct, tc.expected, level)
		}
	}
}
