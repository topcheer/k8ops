package dashboard

import "testing"

func TestRegDivResult2055(t *testing.T) {
	r := RegDivResult2055{Summary: RegDivSummary2055{TotalImages: 50, UniqueRegistries: 3, UnknownRegistries: 5}}
	if r.Summary.UniqueRegistries != 3 {
		t.Errorf("expected 3")
	}
}
func TestIngBackendResult2055(t *testing.T) {
	r := IngBackendResult2055{Summary: IngBackendSummary2055{TotalIngresses: 20, HealthyBackends: 18, DeadBackends: 2}}
	if r.Summary.DeadBackends != 2 {
		t.Errorf("expected 2")
	}
}
func TestPVCLifecycleResult2055(t *testing.T) {
	r := PVCLifecycleResult2055{Summary: PVCLifecycleSummary2055{TotalPVCs: 30, BoundPVCs: 25, PendingPVCs: 3, OldPVCs: 5}}
	if r.Summary.OldPVCs != 5 {
		t.Errorf("expected 5")
	}
}
func TestPVCLifecycleEntry2055(t *testing.T) {
	e := PVCLifecycleEntry2055{Name: "data", Namespace: "prod", AgeDays: 200, Phase: "Bound"}
	if e.AgeDays != 200 {
		t.Errorf("expected 200")
	}
}
