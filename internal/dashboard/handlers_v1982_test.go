package dashboard

import "testing"

func TestTopoSkewResult1982(t *testing.T) {
	r := TopoSkewResult1982{Summary: TopoSkewSummary1982{TotalPods: 100, TotalZones: 3, MaxSkew: 15}}
	if r.Summary.MaxSkew != 15 {
		t.Errorf("expected 15")
	}
}
func TestTopoSkewZoneEntry1982(t *testing.T) {
	e := TopoSkewZoneEntry1982{Zone: "us-east-1a", PodCount: 40}
	if e.PodCount != 40 {
		t.Errorf("expected 40")
	}
}
func TestTopoSkewDepEntry1982(t *testing.T) {
	e := TopoSkewDepEntry1982{Name: "api", Namespace: "prod", Replicas: 5, Skew: 3}
	if e.Skew != 3 {
		t.Errorf("expected 3")
	}
}
func TestIPTablesResult1982(t *testing.T) {
	r := IPTablesResult1982{Summary: IPTablesSummary1982{TotalServices: 50, EstRulesPerNode: 600, PressureLevel: "low"}}
	if r.Summary.EstRulesPerNode != 600 {
		t.Errorf("expected 600")
	}
}
func TestIPTablesNodeEntry1982(t *testing.T) {
	e := IPTablesNodeEntry1982{Name: "node-1", EstRules: 600}
	if e.EstRules != 600 {
		t.Errorf("expected 600")
	}
}
func TestEtcdCompactResult1982(t *testing.T) {
	r := EtcdCompactResult1982{Summary: EtcdCompactSummary1982{EtcdCount: 3, EstDBSizeMB: 512.0, PressureLevel: "low"}}
	if r.Summary.EstDBSizeMB != 512.0 {
		t.Errorf("expected 512")
	}
}
func TestEtcdPodEntry1982(t *testing.T) {
	e := EtcdPodEntry1982{Name: "etcd-node-1", Namespace: "kube-system", Status: "Running"}
	if e.Status != "Running" {
		t.Errorf("expected Running")
	}
}
func TestContainsStr1982(t *testing.T) {
	if !containsStr1982("etcd-image", "etcd") {
		t.Errorf("expected true")
	}
	if containsStr1982("nginx", "etcd") {
		t.Errorf("expected false")
	}
}
