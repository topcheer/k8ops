package dashboard

import (
	"testing"
)

func TestClusterVersionSkewResultStruct1869(t *testing.T) {
	r := ClusterVersionSkewResult{
		Summary: VersionSkewSummary{
			ControlPlaneVersion: "v1.29.2",
			TotalNodes:          5,
			MatchingNodes:       3,
			MinorSkewNodes:      1,
			MajorSkewNodes:      1,
			SkewSupported:       true,
		},
		HealthScore: 60,
	}
	if r.Summary.MatchingNodes != 3 {
		t.Errorf("expected 3, got %d", r.Summary.MatchingNodes)
	}
	if !r.Summary.SkewSupported {
		t.Error("expected skew supported")
	}
}

func TestExtractMinorVer1869(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"v1.29.2", 29},
		{"1.28.4", 28},
		{"v1.27", 27},
		{"invalid", -1},
	}
	for _, c := range cases {
		got := extractMinorVer1869(c.input)
		if got != c.want {
			t.Errorf("extractMinorVer1869(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestNodeTaintImpactResultStruct1869(t *testing.T) {
	r := NodeTaintImpactResult{
		Summary: TaintImpactSummary{
			TotalNodes:     5,
			TaintedNodes:   2,
			CriticalTaints: 3,
			BlockedNodes:   1,
		},
		HealthScore: 70,
	}
	if r.Summary.CriticalTaints != 3 {
		t.Errorf("expected 3, got %d", r.Summary.CriticalTaints)
	}
}

func TestAPIServerSLOResultStruct1869(t *testing.T) {
	r := APIServerSLOResult{
		Summary: APIServerSLOSummary{
			TotalEvents:  100,
			ErrorEvents:  2,
			ErrorRate:    2.0,
			SLOTarget:    1.0,
			SLOCompliant: false,
		},
		HealthScore: 80,
	}
	if r.Summary.SLOCompliant {
		t.Error("expected not compliant (2% > 1%)")
	}
}
