package dashboard

import "testing"

func TestCPUThrotResult2026(t *testing.T) {
	r := CPUThrotResult2026{Summary: CPUThrotSummary2026{TotalContainers: 100, WithCPULimit: 60, AtRiskThrottle: 10}}
	if r.Summary.AtRiskThrottle != 10 {
		t.Errorf("expected 10")
	}
}
func TestCPUThrotEntry2026(t *testing.T) {
	e := CPUThrotEntry2026{Pod: "app", Namespace: "prod", Container: "web", CPULimit: 0.05}
	if e.CPULimit != 0.05 {
		t.Errorf("expected 0.05")
	}
}
func TestNSPressureResult2026(t *testing.T) {
	r := NSPressureResult2026{Summary: NSPressureSummary2026{TotalNS: 10, HighPressureNS: 2, TotalPods: 90}}
	if r.Summary.HighPressureNS != 2 {
		t.Errorf("expected 2")
	}
}
func TestNSPressureEntry2026(t *testing.T) {
	e := NSPressureEntry2026{Namespace: "prod", PodCount: 30, CPURequest: 5.0, MemRequest: 10.0, PressureScore: 75.0}
	if e.PressureScore != 75.0 {
		t.Errorf("expected 75")
	}
}
func TestEvtAgeResult2026(t *testing.T) {
	r := EvtAgeResult2026{Summary: EvtAgeSummary2026{TotalEvents: 300, StaleEvents: 50, OldEvents: 20}}
	if r.Summary.StaleEvents != 50 {
		t.Errorf("expected 50")
	}
}
func TestEvtAgeBucket2026(t *testing.T) {
	e := EvtAgeBucket2026{Bucket: "1-10m", Count: 80}
	if e.Count != 80 {
		t.Errorf("expected 80")
	}
}
func TestCPUThrotSummary2026(t *testing.T) {
	s := CPUThrotSummary2026{AvgCPULimit: 2.5}
	if s.AvgCPULimit != 2.5 {
		t.Errorf("expected 2.5")
	}
}
func TestNSPressureSummary2026(t *testing.T) {
	s := NSPressureSummary2026{TotalPods: 90}
	if s.TotalPods != 90 {
		t.Errorf("expected 90")
	}
}
