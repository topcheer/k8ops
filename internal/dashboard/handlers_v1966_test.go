package dashboard

import "testing"

func TestRestartCostResult1966(t *testing.T) {
	r := RestartCostResult1966{Summary: RestartCostSummary1966{TotalPods: 80, TotalRestarts: 25, HighRestartPods: 5, EstimatedDowntimeMin: 12.5, TotalCostImpactUSD: 0.15}}
	if r.Summary.TotalRestarts != 25 {
		t.Errorf("expected 25")
	}
	if r.Summary.HighRestartPods != 5 {
		t.Errorf("expected 5")
	}
}
func TestRestartCostEntry1966(t *testing.T) {
	e := RestartCostEntry1966{Name: "app-1", Namespace: "prod", Restarts: 10, DowntimeMin: 5.0, CostUSD: 0.03}
	if e.Restarts != 10 {
		t.Errorf("expected 10")
	}
	if e.CostUSD != 0.03 {
		t.Errorf("expected 0.03")
	}
}
func TestNodeDiskIOResult1966(t *testing.T) {
	r := NodeDiskIOResult1966{Summary: NodeDiskIOSummary1966{TotalNodes: 5, HealthyNodes: 4, DiskPressureNodes: 1}}
	if r.Summary.DiskPressureNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestNodeDiskIOEntry1966(t *testing.T) {
	e := NodeDiskIOEntry1966{Name: "node-1", HasDiskPressure: true, ImagesOnNode: 65, Status: "disk-pressure"}
	if !e.HasDiskPressure {
		t.Errorf("expected true")
	}
	if e.ImagesOnNode != 65 {
		t.Errorf("expected 65")
	}
}
func TestEventQPSResult1966(t *testing.T) {
	r := EventQPSResult1966{Summary: EventQPSSummary1966{TotalEvents: 500, EventsPerMin: 8.3, PressureLevel: "low", WarningEvents: 100}}
	if r.Summary.EventsPerMin != 8.3 {
		t.Errorf("expected 8.3")
	}
	if r.Summary.PressureLevel != "low" {
		t.Errorf("expected low")
	}
}
func TestEventQPSEntry1966(t *testing.T) {
	e := EventQPSEntry1966{Source: "kubelet", Reason: "Pulled", Count: 250}
	if e.Count != 250 {
		t.Errorf("expected 250")
	}
}
func TestEventQPSNSEntry1966(t *testing.T) {
	e := EventQPSNSEntry1966{Namespace: "default", EventCount: 300, EventsPerMin: 5.0}
	if e.EventCount != 300 {
		t.Errorf("expected 300")
	}
}
func TestRestartCostSummary1966(t *testing.T) {
	s := RestartCostSummary1966{EstimatedWasteCPU: 2.5, EstimatedWasteMemGB: 10.0}
	if s.EstimatedWasteCPU != 2.5 {
		t.Errorf("expected 2.5")
	}
}
