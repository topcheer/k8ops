package dashboard

import "testing"

func TestLabelCardResult2024(t *testing.T) {
	r := LabelCardResult2024{Summary: LabelCardSummary2024{TotalObjects: 90, UniqueLabelKeys: 25, HighCardinality: 3}}
	if r.Summary.HighCardinality != 3 {
		t.Errorf("expected 3")
	}
}
func TestLabelCardEntry2024(t *testing.T) {
	e := LabelCardEntry2024{LabelKey: "app", Count: 80}
	if e.Count != 80 {
		t.Errorf("expected 80")
	}
}
func TestCMSizeResult2024(t *testing.T) {
	r := CMSizeResult2024{Summary: CMSizeSummary2024{TotalCMs: 50, LargeCMs: 3, VeryLargeCMs: 1}}
	if r.Summary.VeryLargeCMs != 1 {
		t.Errorf("expected 1")
	}
}
func TestCMSizeEntry2024(t *testing.T) {
	e := CMSizeEntry2024{Name: "big-config", Namespace: "prod", DataKeys: 60, EstSizeKB: 150}
	if e.EstSizeKB != 150 {
		t.Errorf("expected 150")
	}
}
func TestEPSAddrResult2024(t *testing.T) {
	r := EPSAddrResult2024{Summary: EPSAddrSummary2024{TotalSlices: 100, TotalAddresses: 300, AvgPerSlice: 3.0, MaxPerSlice: 10}}
	if r.Summary.AvgPerSlice != 3.0 {
		t.Errorf("expected 3.0")
	}
}
func TestEPSAddrEntry2024(t *testing.T) {
	e := EPSAddrEntry2024{Namespace: "prod", SliceCount: 20, AddressCount: 50}
	if e.AddressCount != 50 {
		t.Errorf("expected 50")
	}
}
func TestLabelCardSummary2024(t *testing.T) {
	s := LabelCardSummary2024{UniqueLabelKeys: 25}
	if s.UniqueLabelKeys != 25 {
		t.Errorf("expected 25")
	}
}
func TestCMSizeSummary2024(t *testing.T) {
	s := CMSizeSummary2024{TotalDataKeys: 200}
	if s.TotalDataKeys != 200 {
		t.Errorf("expected 200")
	}
}
