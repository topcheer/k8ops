package dashboard

import "testing"

func TestFragResult1952(t *testing.T) {
	r := FragResult1952{Summary: FragSummary1952{TotalNodes: 1, HighFragNodes: 0, StrandedCPU: 2.5}}
	if r.Summary.StrandedCPU != 2.5 {
		t.Errorf("expected 2.5")
	}
}
func TestFragNodeEntry1952(t *testing.T) {
	e := FragNodeEntry1952{Name: "node-1", CPUFrag: 15.5, MemFrag: 10.2, IsHighFrag: false}
	if e.CPUFrag != 15.5 {
		t.Errorf("expected 15.5")
	}
}
func TestCtrlQueueResult1952(t *testing.T) {
	r := CtrlQueueResult1952{Summary: CtrlQueueSummary1952{TotalOperators: 5, HighRestartOps: 1, TotalRestarts: 15}}
	if r.Summary.TotalRestarts != 15 {
		t.Errorf("expected 15")
	}
}
func TestCtrlQueueEntry1952(t *testing.T) {
	e := CtrlQueueEntry1952{Name: "cert-manager", Ready: true, Restarts: 3}
	if !e.Ready {
		t.Errorf("expected ready")
	}
}
func TestPodDensityOptResult1952(t *testing.T) {
	r := PodDensityOptResult1952{Summary: PodDensityOptSummary1952{TotalNodes: 1, TotalPods: 91, AvgPodsPerNode: 91}}
	if r.Summary.AvgPodsPerNode != 91 {
		t.Errorf("expected 91")
	}
}
func TestPodDensityNodeEntry1952(t *testing.T) {
	e := PodDensityNodeEntry1952{Node: "node-1", PodCount: 91, Capacity: 110, Density: 82.7}
	if e.Density != 82.7 {
		t.Errorf("expected 82.7")
	}
}
func TestContainsStr1952v(t *testing.T) {
	if !containsStr1952("cert-controller", "controller") {
		t.Errorf("expected true")
	}
	if containsStr1952("hello", "world") {
		t.Errorf("expected false")
	}
}
func TestCtrlQueueRisk1952(t *testing.T) {
	e := CtrlQueueRisk1952{RiskType: "high-restarts", Severity: "medium"}
	if e.Severity != "medium" {
		t.Errorf("expected medium")
	}
}
