package dashboard

import "testing"

func TestCPUThrottleResult1993(t *testing.T) {
	r := CPUThrottleResult1993{Summary: CPUThrottleSummary1993{TotalContainers: 50, WithCPULimit: 30, AtRiskCount: 3}}
	if r.Summary.AtRiskCount != 3 {
		t.Errorf("expected 3")
	}
}
func TestCPUThrottleEntry1993(t *testing.T) {
	e := CPUThrottleEntry1993{Pod: "app", Namespace: "prod", Container: "web", LimitCPU: 0.05, RiskLevel: "critical"}
	if e.RiskLevel != "critical" {
		t.Errorf("expected critical")
	}
}
func TestImgDedupResult1993(t *testing.T) {
	r := ImgDedupResult1993{Summary: ImgDedupSummary1993{TotalImages: 30, BaseImageCount: 15, DedupPotential: 45.0}}
	if r.Summary.DedupPotential != 45.0 {
		t.Errorf("expected 45")
	}
}
func TestImgDedupEntry1993(t *testing.T) {
	e := ImgDedupEntry1993{BaseImage: "nginx", RefCount: 10}
	if e.RefCount != 10 {
		t.Errorf("expected 10")
	}
}
func TestSchedLatResult1993(t *testing.T) {
	r := SchedLatResult1993{Summary: SchedLatSummary1993{TotalPods: 80, AvgLatencySec: 2.5, SlowPods: 1}}
	if r.Summary.AvgLatencySec != 2.5 {
		t.Errorf("expected 2.5")
	}
}
func TestSchedLatEntry1993(t *testing.T) {
	e := SchedLatEntry1993{Name: "app", Namespace: "prod", LatencySec: 45.0}
	if e.LatencySec != 45.0 {
		t.Errorf("expected 45")
	}
}
func TestCPUThrottleSummary1993(t *testing.T) {
	s := CPUThrottleSummary1993{AvgLimitCPU: 1.5, ThrottleRisk: "low"}
	if s.ThrottleRisk != "low" {
		t.Errorf("expected low")
	}
}
func TestImgDedupSummary1993(t *testing.T) {
	s := ImgDedupSummary1993{TotalRefs: 120}
	if s.TotalRefs != 120 {
		t.Errorf("expected 120")
	}
}
