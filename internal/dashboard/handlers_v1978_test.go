package dashboard

import "testing"

func TestExitCodeResult1978(t *testing.T) {
	r := ExitCodeResult1978{Summary: ExitCodeSummary1978{TotalContainers: 100, OOMKilled: 5, ExitError: 10}}
	if r.Summary.OOMKilled != 5 {
		t.Errorf("expected 5")
	}
}
func TestExitCodeEntry1978(t *testing.T) {
	e := ExitCodeEntry1978{Pod: "app", Namespace: "prod", Container: "web", ExitCode: 137, Reason: "OOMKilled"}
	if e.Reason != "OOMKilled" {
		t.Errorf("expected OOMKilled")
	}
}
func TestPodQoSResult1978(t *testing.T) {
	r := PodQoSResult1978{Summary: PodQoSSummary1978{TotalPods: 80, Guaranteed: 30, Burstable: 40, BestEffort: 10}}
	if r.Summary.Guaranteed != 30 {
		t.Errorf("expected 30")
	}
}
func TestPodQoSNSEntry1978(t *testing.T) {
	e := PodQoSNSEntry1978{Namespace: "prod", Guaranteed: 10, Burstable: 5, BestEffort: 2}
	if e.BestEffort != 2 {
		t.Errorf("expected 2")
	}
}
func TestNSPressureResult1978(t *testing.T) {
	r := NSPressureResult1978{Summary: NSPressureSummary1978{TotalNamespaces: 10, HighPressure: 2, MediumPressure: 3}}
	if r.Summary.HighPressure != 2 {
		t.Errorf("expected 2")
	}
}
func TestNSPressureEntry1978(t *testing.T) {
	e := NSPressureEntry1978{Namespace: "prod", CPUReq: 8.0, MemReq: 16.0, PodCount: 20, PressureLevel: "high"}
	if e.PressureLevel != "high" {
		t.Errorf("expected high")
	}
}
func TestExitCodeSummary1978(t *testing.T) {
	s := ExitCodeSummary1978{WithExitInfo: 25, SignalKilled: 3}
	if s.SignalKilled != 3 {
		t.Errorf("expected 3")
	}
}
func TestPodQoSSummary1978(t *testing.T) {
	s := PodQoSSummary1978{GuaranteedPct: 37.5, BestEffortPct: 12.5}
	if s.GuaranteedPct != 37.5 {
		t.Errorf("expected 37.5")
	}
}
