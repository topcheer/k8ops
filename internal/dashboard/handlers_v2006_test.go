package dashboard

import "testing"

func TestCordoneResult2006(t *testing.T) {
	r := CordoneResult2006{Summary: CordoneSummary2006{TotalNodes: 5, Schedulable: 4, Cordoned: 1}}
	if r.Summary.Cordoned != 1 {
		t.Errorf("expected 1")
	}
}
func TestCordoneEntry2006(t *testing.T) {
	e := CordoneEntry2006{Name: "node-1", Schedulable: true, Ready: true, Status: "ready"}
	if e.Status != "ready" {
		t.Errorf("expected ready")
	}
}
func TestPVReclaimResult2006(t *testing.T) {
	r := PVReclaimResult2006{Summary: PVReclaimSummary2006{TotalPVs: 20, ReleasedPVs: 3, FailedPVs: 1}}
	if r.Summary.ReleasedPVs != 3 {
		t.Errorf("expected 3")
	}
}
func TestPVReclaimEntry2006(t *testing.T) {
	e := PVReclaimEntry2006{Name: "pv-1", Phase: "Released", Reclaim: "Retain", Size: "10Gi"}
	if e.Reclaim != "Retain" {
		t.Errorf("expected Retain")
	}
}
func TestObjBudgetResult2006(t *testing.T) {
	r := ObjBudgetResult2006{Summary: ObjBudgetSummary2006{TotalObjects: 500, TopType: "Pods", ScalingRisk: "low"}}
	if r.Summary.TopType != "Pods" {
		t.Errorf("expected Pods")
	}
}
func TestObjBudgetEntry2006(t *testing.T) {
	e := ObjBudgetEntry2006{Type: "Services", Count: 50}
	if e.Count != 50 {
		t.Errorf("expected 50")
	}
}
func TestCordoneSummary2006(t *testing.T) {
	s := CordoneSummary2006{NotReady: 0}
	if s.NotReady != 0 {
		t.Errorf("expected 0")
	}
}
func TestPVReclaimSummary2006(t *testing.T) {
	s := PVReclaimSummary2006{BoundPVs: 15, AvailablePVs: 2}
	if s.BoundPVs != 15 {
		t.Errorf("expected 15")
	}
}
