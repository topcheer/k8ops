package dashboard

import "testing"

func TestCapDropResult2064(t *testing.T) {
	r := CapDropResult2064{Summary: CapDropSummary2064{TotalContainers: 200, WithCapDrop: 50, NoCapDrop: 150}}
	if r.Summary.NoCapDrop != 150 {
		t.Errorf("expected 150")
	}
}
func TestSeccompResult2064(t *testing.T) {
	r := SeccompResult2064{Summary: SeccompSummary2064{TotalPods: 100, WithSeccomp: 20, NoSeccomp: 80}}
	if r.Summary.NoSeccomp != 80 {
		t.Errorf("expected 80")
	}
}
func TestNSIsoResult2064(t *testing.T) {
	r := NSIsoResult2064{Summary: NSIsoSummary2064{TotalNS: 20, IsolatedNS: 5, OpenNS: 15}}
	if r.Summary.OpenNS != 15 {
		t.Errorf("expected 15")
	}
}
