package dashboard

import "testing"

func TestFSGroupResult2003(t *testing.T) {
	r := FSGroupResult2003{Summary: FSGroupSummary2003{TotalPods: 80, WithFSGroup: 10, Without: 70}}
	if r.Summary.Without != 70 {
		t.Errorf("expected 70")
	}
}
func TestFSGroupEntry2003(t *testing.T) {
	gid := int64(1000)
	e := FSGroupEntry2003{Pod: "app", Namespace: "prod", FSGroup: &gid}
	if *e.FSGroup != 1000 {
		t.Errorf("expected 1000")
	}
}
func TestProcMountResult2003(t *testing.T) {
	r := ProcMountResult2003{Summary: ProcMountSummary2003{TotalPods: 50, WithProcMount: 2, Unmasked: 1}}
	if r.Summary.Unmasked != 1 {
		t.Errorf("expected 1")
	}
}
func TestProcMountEntry2003(t *testing.T) {
	e := ProcMountEntry2003{Pod: "app", Namespace: "prod", Type: "Unmasked"}
	if e.Type != "Unmasked" {
		t.Errorf("expected Unmasked")
	}
}
func TestKernelParamResult2003(t *testing.T) {
	r := KernelParamResult2003{Summary: KernelParamSummary2003{TotalPods: 80, WithSysctl: 5, Dangerous: 2}}
	if r.Summary.Dangerous != 2 {
		t.Errorf("expected 2")
	}
}
func TestKernelParamEntry2003(t *testing.T) {
	e := KernelParamEntry2003{Pod: "app", Namespace: "prod", Sysctls: []string{"net.core.somaxconn"}}
	if len(e.Sysctls) != 1 {
		t.Errorf("expected 1")
	}
}
func TestDangerousSysctls2003(t *testing.T) {
	if !dangerousSysctls2003["kernel.core_pattern"] {
		t.Errorf("expected kernel.core_pattern to be dangerous")
	}
	if dangerousSysctls2003["net.ipv4.ip_forward"] {
		t.Errorf("net.ipv4.ip_forward should not be in dangerous list")
	}
}
func TestFSGroupSummary2003(t *testing.T) {
	s := FSGroupSummary2003{WithVolMount: 30}
	if s.WithVolMount != 30 {
		t.Errorf("expected 30")
	}
}
