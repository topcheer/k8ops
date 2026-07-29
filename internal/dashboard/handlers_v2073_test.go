package dashboard

import "testing"

func TestAffinityCatResult2073(t *testing.T) {
	r := AffinityCatResult2073{Summary: AffinityCatSummary2073{TotalDeploys: 50, WithAffinity: 10, WithAntiAff: 5}}
	if r.Summary.WithAntiAff != 5 {
		t.Errorf("expected 5")
	}
}
func TestIngTLSResult2073(t *testing.T) {
	r := IngTLSResult2073{Summary: IngTLSSummary2073{TotalIngresses: 20, WithTLS: 15, NoTLS: 5}}
	if r.Summary.NoTLS != 5 {
		t.Errorf("expected 5")
	}
}
func TestCMRotResult2073(t *testing.T) {
	r := CMRotResult2073{Summary: CMRotSummary2073{TotalCMs: 100, StaleCMs: 30, FreshCMs: 70}}
	if r.Summary.StaleCMs != 30 {
		t.Errorf("expected 30")
	}
}
