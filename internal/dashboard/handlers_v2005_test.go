package dashboard

import "testing"

func TestRestTrendResult2005(t *testing.T) {
	r := RestTrendResult2005{Summary: RestTrendSummary2005{TotalPods: 90, RestartedPods: 15, TotalRestarts: 40}}
	if r.Summary.TotalRestarts != 40 {
		t.Errorf("expected 40")
	}
}
func TestRestTrendEntry2005(t *testing.T) {
	e := RestTrendEntry2005{Pod: "app", Namespace: "prod", Restarts: 10}
	if e.Restarts != 10 {
		t.Errorf("expected 10")
	}
}
func TestRolloutResult2005(t *testing.T) {
	r := RolloutResult2005{Summary: RolloutSummary2005{TotalDeployments: 30, Complete: 25, InProgress: 3}}
	if r.Summary.InProgress != 3 {
		t.Errorf("expected 3")
	}
}
func TestRolloutEntry2005(t *testing.T) {
	e := RolloutEntry2005{Name: "api", Namespace: "prod", Updated: 3, Ready: 2, Status: "in-progress"}
	if e.Status != "in-progress" {
		t.Errorf("expected in-progress")
	}
}
func TestPVCHealthResult2005(t *testing.T) {
	r := PVCHealthResult2005{Summary: PVCHealthSummary2005{TotalPVCs: 15, Bound: 14, Pending: 1}}
	if r.Summary.Pending != 1 {
		t.Errorf("expected 1")
	}
}
func TestPVCHealthEntry2005(t *testing.T) {
	e := PVCHealthEntry2005{Name: "data", Namespace: "prod", Status: "Bound", Size: "10Gi"}
	if e.Size != "10Gi" {
		t.Errorf("expected 10Gi")
	}
}
func TestRestTrendSummary2005(t *testing.T) {
	s := RestTrendSummary2005{HighRestart: 5}
	if s.HighRestart != 5 {
		t.Errorf("expected 5")
	}
}
func TestRolloutSummary2005(t *testing.T) {
	s := RolloutSummary2005{StaleProgress: 2}
	if s.StaleProgress != 2 {
		t.Errorf("expected 2")
	}
}
