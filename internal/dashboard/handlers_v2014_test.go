package dashboard

import "testing"

func TestCrashLoopResult2014(t *testing.T) {
	r := CrashLoopResult2014{Summary: CrashLoopSummary2014{TotalPods: 90, CrashLooping: 2, HighRestart: 5}}
	if r.Summary.CrashLooping != 2 {
		t.Errorf("expected 2")
	}
}
func TestCrashLoopEntry2014(t *testing.T) {
	e := CrashLoopEntry2014{Pod: "app", Namespace: "prod", Status: "CrashLoopBackOff", Restarts: 15}
	if e.Restarts != 15 {
		t.Errorf("expected 15")
	}
}
func TestReplicaHealthResult2014(t *testing.T) {
	r := ReplicaHealthResult2014{Summary: ReplicaHealthSummary2014{TotalDeployments: 30, FullyHealthy: 25, UnderReplicated: 5}}
	if r.Summary.UnderReplicated != 5 {
		t.Errorf("expected 5")
	}
}
func TestReplicaHealthEntry2014(t *testing.T) {
	e := ReplicaHealthEntry2014{Name: "api", Namespace: "prod", Desired: 3, Ready: 2, Available: 2}
	if e.Ready != 2 {
		t.Errorf("expected 2")
	}
}
func TestEvtHotspotResult2014(t *testing.T) {
	r := EvtHotspotResult2014{Summary: EvtHotspotSummary2014{TotalEvents: 200, WarningEvents: 30, WarningRatio: 0.15}}
	if r.Summary.WarningRatio != 0.15 {
		t.Errorf("expected 0.15")
	}
}
func TestEvtHotspotEntry2014(t *testing.T) {
	e := EvtHotspotEntry2014{Namespace: "prod", WarningCount: 10, TotalEvents: 30, Ratio: 0.33}
	if e.Ratio != 0.33 {
		t.Errorf("expected 0.33")
	}
}
func TestCrashLoopSummary2014(t *testing.T) {
	s := CrashLoopSummary2014{TotalRestarts: 100}
	if s.TotalRestarts != 100 {
		t.Errorf("expected 100")
	}
}
func TestReplicaHealthSummary2014(t *testing.T) {
	s := ReplicaHealthSummary2014{ZeroReplicas: 3}
	if s.ZeroReplicas != 3 {
		t.Errorf("expected 3")
	}
}
