package dashboard

import "testing"

func TestPodNetPolResult2017(t *testing.T) {
	r := PodNetPolResult2017{Summary: PodNetPolSummary2017{TotalPods: 90, CoveredPods: 60, UncoveredNS: 3}}
	if r.Summary.UncoveredNS != 3 {
		t.Errorf("expected 3")
	}
}
func TestPodNetPolEntry2017(t *testing.T) {
	e := PodNetPolEntry2017{Namespace: "prod", PodCount: 30, HasNetPol: true}
	if !e.HasNetPol {
		t.Errorf("expected true")
	}
}
func TestMaxUnavailResult2017(t *testing.T) {
	r := MaxUnavailResult2017{Summary: MaxUnavailSummary2017{TotalDeployments: 30, WithCustom: 15, UsingDefault: 15}}
	if r.Summary.WithCustom != 15 {
		t.Errorf("expected 15")
	}
}
func TestMaxUnavailEntry2017(t *testing.T) {
	e := MaxUnavailEntry2017{Name: "api", Namespace: "prod", MaxUnavailable: "25%", MaxSurge: "25%"}
	if e.MaxSurge != "25%" {
		t.Errorf("expected 25pct")
	}
}
func TestPullPolResult2017(t *testing.T) {
	r := PullPolResult2017{Summary: PullPolSummary2017{TotalContainers: 100, Always: 30, IfNotPresent: 60, Never: 5, NotSet: 5}}
	if r.Summary.Never != 5 {
		t.Errorf("expected 5")
	}
}
func TestPullPolEntry2017(t *testing.T) {
	e := PullPolEntry2017{Pod: "app", Namespace: "prod", Container: "web", Policy: "Never"}
	if e.Policy != "Never" {
		t.Errorf("expected Never")
	}
}
func TestPodNetPolSummary2017(t *testing.T) {
	s := PodNetPolSummary2017{CoveredPods: 60}
	if s.CoveredPods != 60 {
		t.Errorf("expected 60")
	}
}
func TestPullPolSummary2017(t *testing.T) {
	s := PullPolSummary2017{IfNotPresent: 60}
	if s.IfNotPresent != 60 {
		t.Errorf("expected 60")
	}
}
