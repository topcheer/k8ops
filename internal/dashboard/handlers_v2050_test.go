package dashboard

import "testing"

func TestSTSUpdateResult2050(t *testing.T) {
	r := STSUpdateResult2050{Summary: STSUpdateSummary2050{TotalSTS: 10, RollingUpdate: 7, OnDelete: 3}}
	if r.Summary.OnDelete != 3 {
		t.Errorf("expected 3")
	}
}
func TestSTSUpdateEntry2050(t *testing.T) {
	e := STSUpdateEntry2050{Name: "db", Namespace: "prod", Replicas: 5}
	if e.Replicas != 5 {
		t.Errorf("expected 5")
	}
}
func TestDNSPolicyResult2050(t *testing.T) {
	r := DNSPolicyResult2050{Summary: DNSPolicySummary2050{TotalPods: 100, DefaultDNS: 10, ClusterFirst: 85, CustomDNS: 3, None: 2}}
	if r.Summary.None != 2 {
		t.Errorf("expected 2")
	}
}
func TestDNSPolicyEntry2050(t *testing.T) {
	e := DNSPolicyEntry2050{Pod: "app", Namespace: "prod", Policy: "None"}
	if e.Policy != "None" {
		t.Errorf("expected None")
	}
}
func TestCmdArgsResult2050(t *testing.T) {
	r := CmdArgsResult2050{Summary: CmdArgsSummary2050{TotalContainers: 100, WithCommand: 30, WithArgs: 40, NoCmd: 30}}
	if r.Summary.WithCommand != 30 {
		t.Errorf("expected 30")
	}
}
func TestCmdArgsEntry2050(t *testing.T) {
	e := CmdArgsEntry2050{Pod: "app", Namespace: "prod", Container: "web"}
	if e.Container != "web" {
		t.Errorf("expected web")
	}
}
