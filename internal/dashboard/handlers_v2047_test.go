package dashboard

import "testing"

func TestVSnapCatResult2047(t *testing.T) {
	r := VSnapCatResult2047{Summary: VSnapCatSummary2047{TotalSnapshots: 10, ReadySnapshots: 8, OldSnapshots: 3}}
	if r.Summary.OldSnapshots != 3 {
		t.Errorf("expected 3")
	}
}
func TestVSnapCatEntry2047(t *testing.T) {
	e := VSnapCatEntry2047{Name: "snap-1", Namespace: "prod", AgeDays: 45, Ready: true}
	if !e.Ready {
		t.Errorf("expected ready")
	}
}
func TestPriClassResult2047(t *testing.T) {
	r := PriClassResult2047{Summary: PriClassSummary2047{TotalClasses: 5, SystemCritical: 2, UsedClasses: 3}}
	if r.Summary.SystemCritical != 2 {
		t.Errorf("expected 2")
	}
}
func TestPriClassEntry2047(t *testing.T) {
	e := PriClassEntry2047{Name: "high-priority", Value: 1000, GlobalDefault: false}
	if e.Value != 1000 {
		t.Errorf("expected 1000")
	}
}
func TestEPSliceResult2047(t *testing.T) {
	r := EPSliceResult2047{Summary: EPSliceSummary2047{TotalSlices: 50, TotalEndpoints: 200, ServicesCovered: 30}}
	if r.Summary.ServicesCovered != 30 {
		t.Errorf("expected 30")
	}
}
func TestEPSliceEntry2047(t *testing.T) {
	e := EPSliceEntry2047{Service: "api", Namespace: "prod", Endpoints: 5, Slices: 2}
	if e.Endpoints != 5 {
		t.Errorf("expected 5")
	}
}
