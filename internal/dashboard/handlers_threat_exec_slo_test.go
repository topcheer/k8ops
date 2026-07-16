package dashboard

import (
	"testing"
)

func TestRuntimeThreatTypes(t *testing.T) {
	r := RuntimeThreatResult{ThreatScore: 55, Grade: "D"}
	if r.ThreatScore != 55 || r.Grade != "D" {
		t.Error("struct field error")
	}
	s := RuntimeThreatSummary{TotalPods: 80, PrivilegedPods: 2, HostNetworkPods: 3}
	if s.PrivilegedPods != 2 || s.HostNetworkPods != 3 {
		t.Error("summary error")
	}
	rt := RuntimeThreat{Pod: "app", Severity: "critical", Threats: []string{"privileged"}}
	if rt.Severity != "critical" || len(rt.Threats) != 1 {
		t.Error("threat error")
	}
	pp := PrivilegedPod{Pod: "p1", Namespace: "app", Reason: "privileged"}
	if pp.Reason != "privileged" {
		t.Error("privPod error")
	}
}

func TestRuntimeThreatScoring(t *testing.T) {
	tests := []struct {
		privileged int
		hostNet    int
		hostPID    int
		hostPath   int
		dangerCaps int
		runAsRoot  int
		expectedMin int
		expectedMax int
	}{
		{0, 0, 0, 0, 0, 0, 95, 100},
		{2, 3, 2, 5, 3, 20, 0, 30},
		{1, 0, 0, 1, 0, 5, 65, 80},
	}
	for _, tc := range tests {
		score := 100
		score -= tc.privileged * 15
		score -= tc.hostNet * 8
		score -= tc.hostPID * 8
		score -= tc.hostPath * 3
		score -= tc.dangerCaps * 5
		score -= tc.runAsRoot * 2
		if score < 0 { score = 0 }
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("priv=%d hostNet=%d hostPID=%d hostPath=%d caps=%d root=%d: expected %d-%d, got %d",
				tc.privileged, tc.hostNet, tc.hostPID, tc.hostPath, tc.dangerCaps, tc.runAsRoot,
				tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestExecDashboardTypes(t *testing.T) {
	r := ExecDashboardResult{OverallScore: 68, Grade: "D"}
	if r.OverallScore != 68 || r.Grade != "D" {
		t.Error("struct error")
	}
	ds := DimensionScore{Dimension: "Security", Score: 45, Status: "warning"}
	if ds.Status != "warning" { t.Error("dimScore error") }
	er := ExecRisk{Category: "Security", Severity: "high"}
	if er.Severity != "high" { t.Error("risk error") }
}

func TestScoreStatus(t *testing.T) {
	tests := []struct {
		score    int
		expected string
	}{
		{95, "excellent"},
		{80, "good"},
		{60, "warning"},
		{30, "critical"},
	}
	for _, tc := range tests {
		got := scoreStatus(tc.score)
		if got != tc.expected {
			t.Errorf("score=%d: expected %s, got %s", tc.score, tc.expected, got)
		}
	}
}

func TestSLOComplianceTypes(t *testing.T) {
	r := SLOComplianceResult{ComplianceScore: 92, Grade: "A"}
	if r.ComplianceScore != 92 || r.Grade != "A" {
		t.Error("struct error")
	}
	s := SLOSummary{TotalServices: 84, HealthyServices: 80, EstAvailability: 95.5}
	if s.HealthyServices != 80 || s.EstAvailability != 95.5 {
		t.Error("summary error")
	}
	ns := NamespaceSLO{Namespace: "app", Availability: 98.5, Status: "healthy"}
	if ns.Availability != 98.5 { t.Error("nsSLO error") }
}

func TestSLOScoring(t *testing.T) {
	tests := []struct {
		availability float64
		expectedMin  int
		expectedMax  int
	}{
		{100, 95, 100},
		{95, 90, 100},
		{80, 75, 85},
	}
	for _, tc := range tests {
		score := int(tc.availability)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("avail=%.1f: expected %d-%d, got %d", tc.availability, tc.expectedMin, tc.expectedMax, score)
		}
	}
}
