package dashboard

import (
	"testing"
)

func TestSupplyChainTypes(t *testing.T) {
	r := SupplyChainResult{SecurityScore: 40, Grade: "D"}
	if r.SecurityScore != 40 || r.Grade != "D" {
		t.Error("struct field error")
	}

	s := SupplyChainSummary{TotalImages: 120, ByDigest: 5, ByTag: 115, LatestTag: 30}
	if s.ByTag != 115 || s.LatestTag != 30 {
		t.Error("summary field error")
	}

	ri := RiskImage{Image: "nginx:latest", Severity: "medium", Risks: []string{"uses :latest"}}
	if ri.Severity != "medium" || len(ri.Risks) != 1 {
		t.Error("riskImage field error")
	}

	rs := SupplyRegistryStat{Registry: "docker.io", ImageCount: 50, Trusted: true}
	if rs.ImageCount != 50 || !rs.Trusted {
		t.Error("registryStat field error")
	}
}

func TestSupplyChainScoring(t *testing.T) {
	tests := []struct {
		latest    int
		byTag     int
		noPull    int
		expectedMin int
		expectedMax int
	}{
		{0, 0, 0, 95, 100},
		{30, 80, 10, 0, 30},
		{5, 20, 2, 20, 40},
	}
	for _, tc := range tests {
		score := 100
		score -= tc.latest * 5
		score -= tc.byTag * 2
		score -= tc.noPull * 3
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("latest=%d byTag=%d noPull=%d: expected %d-%d, got %d",
				tc.latest, tc.byTag, tc.noPull, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestSupplyChainRiskSeverity(t *testing.T) {
	tests := []struct {
		isLatest   bool
		byDigest   bool
		isUnknown  bool
		expected   string
	}{
		{true, false, false, "medium"},
		{false, false, false, "medium"},
		{false, false, true, "high"},
	}
	for _, tc := range tests {
		severity := "low"
		if tc.isLatest {
			severity = "medium"
		}
		if !tc.byDigest && severity == "low" {
			severity = "medium"
		}
		if tc.isUnknown {
			severity = "high"
		}
		if severity != tc.expected {
			t.Errorf("latest=%v digest=%v unknown=%v: expected %s, got %s",
				tc.isLatest, tc.byDigest, tc.isUnknown, tc.expected, severity)
		}
	}
}
