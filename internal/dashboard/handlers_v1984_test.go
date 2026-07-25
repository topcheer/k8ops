package dashboard

import "testing"

func TestProbeLatResult1984(t *testing.T) {
	r := ProbeLatResult1984{Summary: ProbeLatSummary1984{TotalContainers: 100, WithLiveness: 80, WithoutProbes: 15}}
	if r.Summary.WithoutProbes != 15 {
		t.Errorf("expected 15")
	}
}
func TestProbeLatEntry1984(t *testing.T) {
	e := ProbeLatEntry1984{Pod: "app", Namespace: "prod", Container: "web", Issue: "no probe"}
	if e.Issue != "no probe" {
		t.Errorf("expected issue")
	}
}
func TestImgPullResult1984(t *testing.T) {
	r := ImgPullResult1984{Summary: ImgPullSummary1984{TotalImages: 30, AvgPullTimeSec: 12.5, UsingAlways: 5}}
	if r.Summary.AvgPullTimeSec != 12.5 {
		t.Errorf("expected 12.5")
	}
}
func TestImgPullEntry1984(t *testing.T) {
	e := ImgPullEntry1984{Image: "nginx:1.25", EstPullSec: 8.0, PullPolicy: "IfNotPresent"}
	if e.PullPolicy != "IfNotPresent" {
		t.Errorf("expected IfNotPresent")
	}
}
func TestConfigReloadResult1984(t *testing.T) {
	r := ConfigReloadResult1984{Summary: ConfigReloadSummary1984{TotalPods: 50, PodsWithCMRef: 20, WithReloader: 5}}
	if r.Summary.WithReloader != 5 {
		t.Errorf("expected 5")
	}
}
func TestConfigReloadEntry1984(t *testing.T) {
	e := ConfigReloadEntry1984{Pod: "app", Namespace: "prod", HasCM: true, HasSecret: false, HasReloader: true}
	if !e.HasCM {
		t.Errorf("expected true")
	}
}
func TestProbeLatSummary1984(t *testing.T) {
	s := ProbeLatSummary1984{WithReadiness: 75, WithStartup: 10}
	if s.WithReadiness != 75 {
		t.Errorf("expected 75")
	}
}
func TestConfigReloadSummary1984(t *testing.T) {
	s := ConfigReloadSummary1984{PodsWithSecretRef: 15, WithoutReloader: 30}
	if s.WithoutReloader != 30 {
		t.Errorf("expected 30")
	}
}
