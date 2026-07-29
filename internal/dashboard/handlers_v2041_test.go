package dashboard

import "testing"

func TestNSCatalogResult2041(t *testing.T) {
	r := NSCatalogResult2041{Summary: NSCatalogSummary2041{TotalNamespaces: 30, ActiveNS: 20, EmptyNS: 5, SystemNS: 5}}
	if r.Summary.EmptyNS != 5 {
		t.Errorf("expected 5")
	}
}
func TestNSCatalogEntry2041(t *testing.T) {
	e := NSCatalogEntry2041{Name: "prod", Status: "Active", PodCount: 15, AgeDays: 100}
	if e.PodCount != 15 {
		t.Errorf("expected 15")
	}
}
func TestLimitRangeDocResult2041(t *testing.T) {
	r := LimitRangeDocResult2041{Summary: LimitRangeDocSummary2041{TotalNS: 20, NSWithLimits: 5, WithoutLimits: 15}}
	if r.Summary.WithoutLimits != 15 {
		t.Errorf("expected 15")
	}
}
func TestLimitRangeDocEntry2041(t *testing.T) {
	e := LimitRangeDocEntry2041{Name: "cpu-limits", Namespace: "prod", MaxCPU: "2", MaxMem: "4Gi"}
	if e.MaxCPU != "2" {
		t.Errorf("expected 2")
	}
}
func TestEventFreqResult2041(t *testing.T) {
	r := EventFreqResult2041{Summary: EventFreqSummary2041{TotalEvents: 500, UniqueReasons: 20, WarningEvents: 150}}
	if r.Summary.WarningEvents != 150 {
		t.Errorf("expected 150")
	}
}
func TestEventFreqEntry2041(t *testing.T) {
	e := EventFreqEntry2041{Reason: "FailedScheduling", Count: 50, Type: "Warning"}
	if e.Count != 50 {
		t.Errorf("expected 50")
	}
}
