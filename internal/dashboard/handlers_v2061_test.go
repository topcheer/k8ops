package dashboard

import "testing"

func TestImgCacheResult2061(t *testing.T) {
	r := ImgCacheResult2061{Summary: ImgCacheSummary2061{TotalContainers: 200, UniqueImages: 50, DuplicateImages: 30}}
	if r.Summary.DuplicateImages != 30 {
		t.Errorf("expected 30")
	}
}
func TestEPHealthResult2061(t *testing.T) {
	r := EPHealthResult2061{Summary: EPHealthSummary2061{TotalEPs: 80, HealthyEPs: 70, UnhealthyEPs: 10}}
	if r.Summary.UnhealthyEPs != 10 {
		t.Errorf("expected 10")
	}
}
func TestNSCostResult2061(t *testing.T) {
	r := NSCostResult2061{Summary: NSCostSummary2061{TotalNS: 30, TotalCost: 500.0, AvgCostPerNS: 16.67}}
	if r.Summary.TotalCost != 500.0 {
		t.Errorf("expected 500")
	}
}
