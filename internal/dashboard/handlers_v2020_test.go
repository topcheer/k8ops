package dashboard

import "testing"

func TestPhaseDistResult2020(t *testing.T) {
	r := PhaseDistResult2020{Summary: PhaseDistSummary2020{TotalPods: 100, Running: 90, Pending: 5, Failed: 3}}
	if r.Summary.Failed != 3 {
		t.Errorf("expected 3")
	}
}
func TestPhaseDistEntry2020(t *testing.T) {
	e := PhaseDistEntry2020{Namespace: "prod", Running: 30, Pending: 2, Failed: 1}
	if e.Running != 30 {
		t.Errorf("expected 30")
	}
}
func TestRestReasonResult2020(t *testing.T) {
	r := RestReasonResult2020{Summary: RestReasonSummary2020{TotalRestarts: 100, OOMKilled: 30, Unknown: 20}}
	if r.Summary.OOMKilled != 30 {
		t.Errorf("expected 30")
	}
}
func TestRestReasonEntry2020(t *testing.T) {
	e := RestReasonEntry2020{Reason: "OOMKilled", Count: 10}
	if e.Count != 10 {
		t.Errorf("expected 10")
	}
}
func TestKubeletVerResult2020(t *testing.T) {
	r := KubeletVerResult2020{Summary: KubeletVerSummary2020{TotalNodes: 5, UniqueVersions: 2, DriftLevel: "low"}}
	if r.Summary.DriftLevel != "low" {
		t.Errorf("expected low")
	}
}
func TestKubeletVerEntry2020(t *testing.T) {
	e := KubeletVerEntry2020{Version: "v1.28.4", Count: 3}
	if e.Count != 3 {
		t.Errorf("expected 3")
	}
}
func TestPhaseDistSummary2020(t *testing.T) {
	s := PhaseDistSummary2020{Succeeded: 5}
	if s.Succeeded != 5 {
		t.Errorf("expected 5")
	}
}
func TestRestReasonSummary2020(t *testing.T) {
	s := RestReasonSummary2020{Exited: 40}
	if s.Exited != 40 {
		t.Errorf("expected 40")
	}
}
