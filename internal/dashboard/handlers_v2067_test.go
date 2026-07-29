package dashboard

import "testing"

func TestWkldDensResult2067(t *testing.T) {
	r := WkldDensResult2067{Summary: WkldDensSummary2067{TotalNS: 30, DenseNS: 3, TotalPods: 200}}
	if r.Summary.DenseNS != 3 {
		t.Errorf("expected 3")
	}
}
func TestImgVintageResult2067(t *testing.T) {
	r := ImgVintageResult2067{Summary: ImgVintageSummary2067{TotalImages: 50, StaleImages: 10}}
	if r.Summary.StaleImages != 10 {
		t.Errorf("expected 10")
	}
}
func TestSvcProtoResult2067(t *testing.T) {
	r := SvcProtoResult2067{Summary: SvcProtoSummary2067{TotalServices: 80, TCPCount: 70, UDPCount: 8, SCTPCount: 2}}
	if r.Summary.TCPCount != 70 {
		t.Errorf("expected 70")
	}
}
