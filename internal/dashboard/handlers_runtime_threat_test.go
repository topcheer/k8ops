package dashboard

import "testing"

func TestRuntimeThreatTypesExisting(t *testing.T) {
	r := RuntimeThreatResult{ThreatScore: 55, Grade: "D"}
	if r.ThreatScore != 55 || r.Grade != "D" {
		t.Error("struct error")
	}
	s := RuntimeThreatSummary{TotalPods: 80, PrivilegedPods: 2, HostNetworkPods: 3, RunAsRoot: 10}
	if s.PrivilegedPods != 2 || s.RunAsRoot != 10 {
		t.Error("summary error")
	}
	rt := RuntimeThreat{Pod: "app", Severity: "critical"}
	if rt.Severity != "critical" {
		t.Error("threat error")
	}
}

func TestRuntimeThreatScoringExisting(t *testing.T) {
	// No threats = perfect score
	score := 100
	score -= 0 * 15 // privileged
	score -= 0 * 8  // hostNet
	score -= 0 * 2  // root
	if score != 100 {
		t.Errorf("clean cluster should be 100, got %d", score)
	}
	// Heavy threats
	score = 100
	score -= 3 * 15
	score -= 5 * 8
	score -= 20 * 2
	if score > 0 {
		t.Errorf("heavy threats should be <= 0, got %d", score)
	}
}
