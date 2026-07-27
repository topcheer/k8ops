package dashboard

import "testing"

func TestTopoSpreadResult1998(t *testing.T) {
	r := TopoSpreadResult1998{Summary: TopoSpreadSummary1998{TotalPods: 80, WithSpread: 10, ZoneSpread: 5}}
	if r.Summary.ZoneSpread != 5 {
		t.Errorf("expected 5")
	}
}
func TestTopoSpreadEntry1998(t *testing.T) {
	e := TopoSpreadEntry1998{Pod: "app", Namespace: "prod", TopologyKey: "zone", MaxSkew: 1, WhenUnsatisfiable: "DoNotSchedule"}
	if e.TopologyKey != "zone" {
		t.Errorf("expected zone")
	}
}
func TestLimitRangeCatResult1998(t *testing.T) {
	r := LimitRangeCatResult1998{Summary: LimitRangeCatSummary1998{TotalLimitRanges: 5, NamespacesWith: 3, WithCPULimit: 4}}
	if r.Summary.WithCPULimit != 4 {
		t.Errorf("expected 4")
	}
}
func TestLimitRangeCatEntry1998(t *testing.T) {
	e := LimitRangeCatEntry1998{Name: "limits", Namespace: "prod", RuleCount: 3}
	if e.RuleCount != 3 {
		t.Errorf("expected 3")
	}
}
func TestLeaseResult1998(t *testing.T) {
	r := LeaseResult1998{Summary: LeaseSummary1998{TotalLeases: 10, ActiveLeases: 8, ExpiredLeases: 2}}
	if r.Summary.ExpiredLeases != 2 {
		t.Errorf("expected 2")
	}
}
func TestLeaseEntry1998(t *testing.T) {
	e := LeaseEntry1998{Name: "leader", Namespace: "kube-system", Holder: "controller-0", AgeSeconds: 5.0}
	if e.Holder != "controller-0" {
		t.Errorf("expected controller-0")
	}
}
func TestTopoSpreadSummary1998(t *testing.T) {
	s := TopoSpreadSummary1998{HostSpread: 8}
	if s.HostSpread != 8 {
		t.Errorf("expected 8")
	}
}
func TestLeaseSummary1998(t *testing.T) {
	s := LeaseSummary1998{AvgRenewSec: 3.5}
	if s.AvgRenewSec != 3.5 {
		t.Errorf("expected 3.5")
	}
}
