package dashboard

import "testing"

func TestResWastageResult1939(t *testing.T) {
	r := ResWastageResult1939{Summary: ResWastageSummary1939{TotalContainers: 100, WithLimits: 60, HighRatioCount: 15}}
	if r.Summary.HighRatioCount != 15 {
		t.Errorf("expected 15")
	}
}
func TestResWastageEntry1939(t *testing.T) {
	e := ResWastageEntry1939{Container: "main", CPUReq: 0.5, CPULim: 4.0, CPURatio: 8.0}
	if e.CPURatio != 8.0 {
		t.Errorf("expected 8.0")
	}
}
func TestSAUsageResult1939(t *testing.T) {
	r := SAUsageResult1939{Summary: SAUsageSummary1939{TotalSAs: 30, UsedSAs: 15, OrphanedSAs: 15}}
	if r.Summary.OrphanedSAs != 15 {
		t.Errorf("expected 15")
	}
}
func TestSAOrphanEntry1939(t *testing.T) {
	e := SAOrphanEntry1939{Name: "old-sa", Age: "45d", Reason: "unused"}
	if e.Reason != "unused" {
		t.Errorf("expected unused")
	}
}
func TestEPSliceResult1939(t *testing.T) {
	r := EPSliceResult1939{Summary: EPSliceSummary1939{TotalSlices: 20, ReadyEndpoints: 50, NotReadyEPs: 3}}
	if r.Summary.NotReadyEPs != 3 {
		t.Errorf("expected 3")
	}
}
func TestEPSliceEntry1939(t *testing.T) {
	e := EPSliceEntry1939{ServiceName: "api", SliceCount: 2, Endpoints: 5, Ready: 4}
	if e.Ready != 4 {
		t.Errorf("expected 4")
	}
}
func TestMaxFloat1939(t *testing.T) {
	if maxFloat1939(3.0, 5.0) != 5.0 {
		t.Errorf("expected 5.0")
	}
	if maxFloat1939(10.0, 2.0) != 10.0 {
		t.Errorf("expected 10.0")
	}
}
func TestSplitKey1939(t *testing.T) {
	if splitKey("prod/api") != "api" {
		t.Errorf("expected api")
	}
}
