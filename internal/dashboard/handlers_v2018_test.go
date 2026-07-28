package dashboard

import "testing"

func TestPodDensResult2018(t *testing.T) {
	r := PodDensResult2018{Summary: PodDensSummary2018{TotalNodes: 5, TotalPods: 150, AvgDensity: 30.0, DensityLevel: "low"}}
	if r.Summary.AvgDensity != 30.0 {
		t.Errorf("expected 30")
	}
}
func TestPodDensEntry2018(t *testing.T) {
	e := PodDensEntry2018{Node: "node-1", PodCount: 50, DensityPct: 45.5}
	if e.DensityPct != 45.5 {
		t.Errorf("expected 45.5")
	}
}
func TestSMEPResult2018(t *testing.T) {
	r := SMEPResult2018{Summary: SMEPSummary2018{TotalServices: 50, TotalEndpoints: 120, AvgEPPerSvc: 2.4}}
	if r.Summary.AvgEPPerSvc != 2.4 {
		t.Errorf("expected 2.4")
	}
}
func TestSMEPEntry2018(t *testing.T) {
	e := SMEPEntry2018{Namespace: "prod", SvcCount: 10, EPCount: 25}
	if e.EPCount != 25 {
		t.Errorf("expected 25")
	}
}
func TestAllocHeadResult2018(t *testing.T) {
	r := AllocHeadResult2018{Summary: AllocHeadSummary2018{TotalNodes: 3, AvgAllocRatio: 0.85, WorstNode: "node-2"}}
	if r.Summary.WorstNode != "node-2" {
		t.Errorf("expected node-2")
	}
}
func TestAllocHeadEntry2018(t *testing.T) {
	e := AllocHeadEntry2018{Node: "node-1", CapacityCPU: 16.0, AllocatableCPU: 15.0, Ratio: 0.9375}
	if e.Ratio != 0.9375 {
		t.Errorf("expected 0.9375")
	}
}
func TestPodDensSummary2018(t *testing.T) {
	s := PodDensSummary2018{MaxDensity: 80, DensityLevel: "high"}
	if s.MaxDensity != 80 {
		t.Errorf("expected 80")
	}
}
func TestAllocHeadSummary2018(t *testing.T) {
	s := AllocHeadSummary2018{AvgAllocRatio: 0.9}
	if s.AvgAllocRatio != 0.9 {
		t.Errorf("expected 0.9")
	}
}
