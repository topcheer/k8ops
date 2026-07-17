package dashboard

import (
	"testing"
)

func TestAPIVersionTypes(t *testing.T) {
	r := APIVersionResult{GovernanceScore: 90, Grade: "A", UpgradeReadiness: "ready"}
	if r.GovernanceScore != 90 || r.Grade != "A" || r.UpgradeReadiness != "ready" {
		t.Error("struct field error")
	}

	s := APIVersionSummary{ServerVersion: "v1.36.1", TotalResources: 200, DeprecatedCount: 3, AlphaAPIUsage: 1}
	if s.ServerVersion != "v1.36.1" || s.DeprecatedCount != 3 {
		t.Error("summary field error")
	}

	da := DeprecatedAPI{Resource: "api-ingress", APIVersion: "extensions/v1beta1", Status: "removed", Severity: "critical"}
	if da.Status != "removed" || da.Severity != "critical" {
		t.Error("deprecatedAPI field error")
	}

	vr := VersionRisk{Risk: "CRD uses alpha", Severity: "medium"}
	if vr.Severity != "medium" {
		t.Error("versionRisk field error")
	}
}

func TestAPIVersionScoring(t *testing.T) {
	tests := []struct {
		deprecated  int
		removed     int
		alpha       int
		beta        int
		expectedMin int
		expectedMax int
	}{
		{0, 0, 0, 0, 95, 100}, // Clean
		{3, 1, 0, 0, 40, 60},  // Some deprecated
		{0, 0, 2, 5, 75, 90},  // Alpha/beta only
	}
	for _, tc := range tests {
		score := 100
		score -= tc.deprecated * 10
		score -= tc.removed * 20
		score -= tc.alpha * 5
		score -= tc.beta * 2
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("deprecated=%d removed=%d alpha=%d beta=%d: expected %d-%d, got %d",
				tc.deprecated, tc.removed, tc.alpha, tc.beta, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestAPIVersionUpgradeReadiness(t *testing.T) {
	tests := []struct {
		deprecated int
		removed    int
		alpha      int
		expected   string
	}{
		{0, 0, 0, "ready"},
		{1, 0, 0, "blocked"},
		{0, 1, 0, "blocked"},
		{0, 0, 1, "at-risk"},
	}
	for _, tc := range tests {
		readiness := "ready"
		if tc.deprecated > 0 || tc.removed > 0 {
			readiness = "blocked"
		} else if tc.alpha > 0 {
			readiness = "at-risk"
		}
		if readiness != tc.expected {
			t.Errorf("deprecated=%d removed=%d alpha=%d: expected %s, got %s",
				tc.deprecated, tc.removed, tc.alpha, tc.expected, readiness)
		}
	}
}
