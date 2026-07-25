package dashboard

import "testing"

func TestNodeAllocGapResult1976(t *testing.T) {
	r := NodeAllocGapResult1976{Summary: NodeAllocGapSummary1976{TotalNodes: 5, TotalCapacity: 80, TotalAllocatable: 72, ReservationPct: 10.0}}
	if r.Summary.ReservationPct != 10.0 {
		t.Errorf("expected 10")
	}
}
func TestNodeAllocGapEntry1976(t *testing.T) {
	e := NodeAllocGapEntry1976{Name: "node-1", Capacity: 16, Allocatable: 14.5, Reserved: 1.5, ReservedPct: 9.4}
	if e.Reserved != 1.5 {
		t.Errorf("expected 1.5")
	}
}
func TestPodOverheadResult1976(t *testing.T) {
	r := PodOverheadResult1976{Summary: PodOverheadSummary1976{TotalPods: 50, AvgOverheadPct: 8.5, HighOverheadCount: 5}}
	if r.Summary.HighOverheadCount != 5 {
		t.Errorf("expected 5")
	}
}
func TestPodOverheadEntry1976(t *testing.T) {
	e := PodOverheadEntry1976{Name: "app", Namespace: "prod", CPUReq: 2.0, MemReq: 4.0, ContainerCount: 3, OverheadPct: 15.0}
	if e.OverheadPct != 15.0 {
		t.Errorf("expected 15")
	}
}
func TestAPIQPSResult1976(t *testing.T) {
	r := APIQPSResult1976{Summary: APIQPSSummary1976{TotalPods: 100, EstQPS: 75.0, PressureLevel: "low", RecommendedLimit: 375}}
	if r.Summary.EstQPS != 75.0 {
		t.Errorf("expected 75")
	}
	if r.Summary.RecommendedLimit != 375 {
		t.Errorf("expected 375")
	}
}
func TestAPIQPENSEntry1976(t *testing.T) {
	e := APIQPENSEntry1976{Namespace: "prod", PodCount: 30, EstQPS: 15.0}
	if e.EstQPS != 15.0 {
		t.Errorf("expected 15")
	}
}
func TestNodeAllocGapSummary1976(t *testing.T) {
	s := NodeAllocGapSummary1976{TotalReserved: 8.0}
	if s.TotalReserved != 8.0 {
		t.Errorf("expected 8")
	}
}
func TestPodOverheadSummary1976(t *testing.T) {
	s := PodOverheadSummary1976{TotalRequestedCPU: 25.5, TotalRequestedMem: 80.0}
	if s.TotalRequestedCPU != 25.5 {
		t.Errorf("expected 25.5")
	}
}
