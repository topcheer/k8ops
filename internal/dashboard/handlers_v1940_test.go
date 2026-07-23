package dashboard

import "testing"

func TestResPressureResult1940(t *testing.T) {
	r := ResPressureResult1940{Summary: ResPressureSummary1940{TotalNodes: 3, HighPressureNodes: 1, AvgCPUPressure: 65.5}}
	if r.Summary.HighPressureNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestResPressureNode1940(t *testing.T) {
	e := ResPressureNode1940{Name: "node-1", CPUUsage: 80, PressureScore: 75, IsHigh: true}
	if !e.IsHigh {
		t.Errorf("expected high")
	}
}
func TestAntiAffinityResult1940(t *testing.T) {
	r := AntiAffinityResult1940{Summary: AntiAffinitySummary1940{TotalWorkloads: 30, WithAntiAffinity: 10, UncoveredMulti: 15}}
	if r.Summary.UncoveredMulti != 15 {
		t.Errorf("expected 15")
	}
}
func TestAntiAffinityEntry1940(t *testing.T) {
	e := AntiAffinityEntry1940{Name: "api", Replicas: 3, Type: "podAntiAffinity"}
	if e.Type != "podAntiAffinity" {
		t.Errorf("expected podAntiAffinity")
	}
}
func TestStartupLatencyResult1940(t *testing.T) {
	r := StartupLatencyResult1940{Summary: StartupLatencySummary1940{TotalPods: 80, AvgLatencySec: 45.5, SlowCount: 5}}
	if r.Summary.SlowCount != 5 {
		t.Errorf("expected 5")
	}
}
func TestStartupLatencyEntry1940(t *testing.T) {
	e := StartupLatencyEntry1940{Name: "web", LatencySec: 30, ImagePullSec: 15}
	if e.ImagePullSec != 15 {
		t.Errorf("expected 15")
	}
}
func TestStartupSlowEntry1940(t *testing.T) {
	e := StartupSlowEntry1940{Name: "worker", LatencySec: 180, Reason: "slow image pull"}
	if e.LatencySec != 180 {
		t.Errorf("expected 180")
	}
}
func TestResPressureNSEntry1940(t *testing.T) {
	e := ResPressureNSEntry1940{Namespace: "prod", CPUReq: 3.5, PodCount: 15}
	if e.CPUReq != 3.5 {
		t.Errorf("expected 3.5")
	}
}
