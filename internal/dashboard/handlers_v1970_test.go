package dashboard

import "testing"

func TestConntrackResult1970(t *testing.T) {
	r := ConntrackResult1970{Summary: ConntrackSummary1970{TotalNodes: 5, EstConnections: 50000, ConntrackLimit: 655360, UtilizationPct: 7.6, PressureLevel: "low"}}
	if r.Summary.PressureLevel != "low" {
		t.Errorf("expected low")
	}
}
func TestConntrackNodeEntry1970(t *testing.T) {
	e := ConntrackNodeEntry1970{Name: "node-1", EstConns: 10000, PodCount: 100, Utilization: 7.6}
	if e.PodCount != 100 {
		t.Errorf("expected 100")
	}
}
func TestIPPoolResult1970(t *testing.T) {
	r := IPPoolResult1970{Summary: IPPoolSummary1970{TotalNodes: 5, TotalPods: 200, ClusterIPUtilPct: 15.7, ServiceIPUtilPct: 0.3}}
	if r.Summary.ClusterIPUtilPct != 15.7 {
		t.Errorf("expected 15.7")
	}
}
func TestIPPoolEntry1970(t *testing.T) {
	e := IPPoolEntry1970{Node: "node-1", PodCIDR: "10.42.1.0/24", PodCount: 45}
	if e.PodCIDR != "10.42.1.0/24" {
		t.Errorf("expected CIDR")
	}
}
func TestResVersionResult1970(t *testing.T) {
	r := ResVersionResult1970{Summary: ResVersionSummary1970{TotalPods: 100, AvgAge: 48.5, StalePods: 3, CacheFreshness: "stable"}}
	if r.Summary.CacheFreshness != "stable" {
		t.Errorf("expected stable")
	}
}
func TestResVersionEntry1970(t *testing.T) {
	e := ResVersionEntry1970{Name: "app-1", Namespace: "prod", AgeHours: 800, IsStale: true}
	if !e.IsStale {
		t.Errorf("expected true")
	}
}
func TestResVersionSummary1970(t *testing.T) {
	s := ResVersionSummary1970{NewestAgeMin: 5.2, OldestAgeDays: 120.5}
	if s.OldestAgeDays != 120.5 {
		t.Errorf("expected 120.5")
	}
}
func TestConntrackSummary1970(t *testing.T) {
	s := ConntrackSummary1970{EstConnections: 30000, ConntrackLimit: 131072}
	if s.ConntrackLimit != 131072 {
		t.Errorf("expected 131072")
	}
}
