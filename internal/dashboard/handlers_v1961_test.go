package dashboard

import "testing"

func TestLinuxCapResult1961(t *testing.T) {
	r := LinuxCapResult1961{Summary: LinuxCapSummary1961{TotalContainers: 50, WithCapabilities: 10, DangerousCapCount: 3, PrivilegedCount: 2}}
	if r.Summary.DangerousCapCount != 3 {
		t.Errorf("expected 3")
	}
	if r.Summary.PrivilegedCount != 2 {
		t.Errorf("expected 2")
	}
}
func TestLinuxCapEntry1961(t *testing.T) {
	e := LinuxCapEntry1961{Container: "app", Pod: "app-xyz", Caps: []string{"SYS_ADMIN"}, Severity: "high"}
	if e.Severity != "high" {
		t.Errorf("expected high")
	}
	if len(e.Caps) != 1 {
		t.Errorf("expected 1 cap")
	}
}
func TestLinuxCapContainer1961(t *testing.T) {
	e := LinuxCapContainer1961{Container: "sidecar", DropsAll: true, Added: []string{}, Dropped: []string{"ALL"}}
	if !e.DropsAll {
		t.Errorf("expected true")
	}
}
func TestEgressAuditResult1961(t *testing.T) {
	r := EgressAuditResult1961{Summary: EgressAuditSummary1961{TotalNamespaces: 20, WithEgressPolicy: 5, WithoutEgress: 15, LoadBalancerSvc: 3, NodePortSvc: 7}}
	if r.Summary.WithoutEgress != 15 {
		t.Errorf("expected 15, got %d", r.Summary.WithoutEgress)
	}
}
func TestEgressTarget1961(t *testing.T) {
	e := EgressTarget1961{Namespace: "prod", Service: "api-lb", Type: "LoadBalancer", Exposed: true}
	if !e.Exposed {
		t.Errorf("expected true")
	}
}
func TestNodeHardenResult1961(t *testing.T) {
	r := NodeHardenResult1961{Summary: NodeHardenSummary1961{TotalNodes: 5, ReadyNodes: 5, AvgHardening: 92.5}}
	if r.Summary.AvgHardening != 92.5 {
		t.Errorf("expected 92.5")
	}
}
func TestNodeHardenEntry1961(t *testing.T) {
	e := NodeHardenEntry1961{Name: "node-1", Ready: true, HardeningScore: 88.0, Issues: 2, Version: "v1.30.0"}
	if e.HardeningScore != 88.0 {
		t.Errorf("expected 88.0")
	}
}
func TestNodeHardenCheck1961(t *testing.T) {
	e := NodeHardenCheck1961{Check: "container-runtime", Status: "pass", Severity: "info"}
	if e.Status != "pass" {
		t.Errorf("expected pass")
	}
}
func TestContainsStr1961v(t *testing.T) {
	if !containsStr1961("Ingress Egress", "Egress") {
		t.Errorf("expected true")
	}
	if containsStr1961("hello", "world") {
		t.Errorf("expected false")
	}
}
