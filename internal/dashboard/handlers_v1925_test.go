package dashboard

import "testing"

func TestTokenProjectionResult1925(t *testing.T) {
	r := TokenProjectionResult1925{
		Summary: TokenProjectionSummary1925{TotalPods: 80, ProjectedTokens: 20, LegacyAutoMount: 50, DefaultSAUsed: 15},
	}
	if r.Summary.ProjectedTokens != 20 {
		t.Errorf("expected 20, got %d", r.Summary.ProjectedTokens)
	}
}

func TestTokenProjectionEntry1925(t *testing.T) {
	e := TokenProjectionEntry1925{PodName: "web-1", Namespace: "prod", ServiceAccount: "web-sa", IsProjected: true}
	if !e.IsProjected {
		t.Errorf("expected projected=true")
	}
}

func TestSysctlRiskResult1925(t *testing.T) {
	r := SysctlRiskResult1925{
		Summary: SysctlSummary1925{TotalPods: 80, WithSysctls: 5, DangerousCount: 2, UnsafeAllowed: 4},
	}
	if r.Summary.DangerousCount != 2 {
		t.Errorf("expected 2, got %d", r.Summary.DangerousCount)
	}
}

func TestSysctlDanger1925(t *testing.T) {
	d := SysctlDanger1925{Sysctl: "net.ipv4.ip_forward", Value: "1", Severity: "medium"}
	if d.Severity != "medium" {
		t.Errorf("expected medium")
	}
}

func TestHostPortResult1925(t *testing.T) {
	r := HostPortResult1925{
		Summary: HostPortSummary1925{TotalPods: 80, PodsWithHostPort: 8, TotalHostPorts: 12, PrivilegedPorts: 2},
	}
	if r.Summary.PrivilegedPorts != 2 {
		t.Errorf("expected 2, got %d", r.Summary.PrivilegedPorts)
	}
}

func TestHostPortEntry1925(t *testing.T) {
	e := HostPortEntry1925{PodName: "lb-1", Port: 80, IsPrivileged: true}
	if !e.IsPrivileged {
		t.Errorf("expected privileged")
	}
}

func TestHostPortConflict1925(t *testing.T) {
	c := HostPortConflict1925{Port: 8080, Pod1: "api-1", Pod2: "api-2"}
	if c.Pod1 != "api-1" {
		t.Errorf("expected api-1")
	}
}

func TestHostPortRisk1925(t *testing.T) {
	r := HostPortRisk1925{RiskType: "bypass-netpol", Severity: "medium"}
	if r.RiskType != "bypass-netpol" {
		t.Errorf("expected bypass-netpol")
	}
}
