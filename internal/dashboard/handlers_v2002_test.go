package dashboard

import "testing"

func TestPodInitResult2002(t *testing.T) {
	r := PodInitResult2002{Summary: PodInitSummary2002{TotalPods: 80, AvgInitSec: 15.5, SlowPods: 3}}
	if r.Summary.SlowPods != 3 {
		t.Errorf("expected 3")
	}
}
func TestPodInitEntry2002(t *testing.T) {
	e := PodInitEntry2002{Name: "app", Namespace: "prod", InitSec: 90.0}
	if e.InitSec != 90.0 {
		t.Errorf("expected 90")
	}
}
func TestKubeletCertResult2002(t *testing.T) {
	r := KubeletCertResult2002{Summary: KubeletCertSummary2002{TotalNodes: 5, ExpiringSoon: 1, Expired: 0}}
	if r.Summary.ExpiringSoon != 1 {
		t.Errorf("expected 1")
	}
}
func TestKubeletCertEntry2002(t *testing.T) {
	e := KubeletCertEntry2002{Name: "node-1", Ready: true, CertAge: "120d"}
	if e.CertAge != "120d" {
		t.Errorf("expected 120d")
	}
}
func TestNSEventNoiseResult2002(t *testing.T) {
	r := NSEventNoiseResult2002{Summary: NSEventNoiseSummary2002{TotalEvents: 500, TotalNS: 10, NoisyNS: 2}}
	if r.Summary.NoisyNS != 2 {
		t.Errorf("expected 2")
	}
}
func TestNSEventNoiseEntry2002(t *testing.T) {
	e := NSEventNoiseEntry2002{Namespace: "prod", Events: 100, Warnings: 30}
	if e.Warnings != 30 {
		t.Errorf("expected 30")
	}
}
func TestPodInitSummary2002(t *testing.T) {
	s := PodInitSummary2002{MaxInitSec: 120.0, FastPods: 50}
	if s.FastPods != 50 {
		t.Errorf("expected 50")
	}
}
func TestKubeletCertSummary2002(t *testing.T) {
	s := KubeletCertSummary2002{WithCertInfo: 5}
	if s.WithCertInfo != 5 {
		t.Errorf("expected 5")
	}
}
