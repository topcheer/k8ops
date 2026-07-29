package dashboard

import "testing"

func TestRestartBudgetResult2032(t *testing.T) {
	r := RestartBudgetResult2032{Summary: RestartBudgetSummary2032{TotalPods: 100, WithRestarts: 30, HighRestartRate: 5, AvgRestartRate: 0.3}}
	if r.Summary.HighRestartRate != 5 {
		t.Errorf("expected 5")
	}
}
func TestRestartBudgetEntry2032(t *testing.T) {
	e := RestartBudgetEntry2032{Pod: "app", Namespace: "prod", Restarts: 10, PodAgeHours: 5.0, RestartRate: 2.0}
	if e.RestartRate != 2.0 {
		t.Errorf("expected 2.0")
	}
}
func TestVolIOPSResult2032(t *testing.T) {
	r := VolIOPSResult2032{Summary: VolIOPSSummary2032{TotalPVCs: 20, BoundPVCs: 18, HighIOPS: 3, SharedPVCs: 3}}
	if r.Summary.SharedPVCs != 3 {
		t.Errorf("expected 3")
	}
}
func TestVolIOPSEntry2032(t *testing.T) {
	e := VolIOPSEntry2032{PVC: "data", Namespace: "prod", Size: "100Gi", MountCount: 5}
	if e.MountCount != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodeAllocResult2032(t *testing.T) {
	r := NodeAllocResult2032{Summary: NodeAllocSummary2032{TotalNodes: 5, OverAllocated: 1, TightNodes: 2}}
	if r.Summary.OverAllocated != 1 {
		t.Errorf("expected 1")
	}
}
func TestNodeAllocEntry2032(t *testing.T) {
	e := NodeAllocEntry2032{Node: "node-1", AllocPct: 70, ReservedPct: 30}
	if e.ReservedPct != 30 {
		t.Errorf("expected 30")
	}
}
