package dashboard

import "testing"

func TestSvcPortResult2059(t *testing.T) {
	r := SvcPortResult2059{Summary: SvcPortSummary2059{TotalServices: 50, TotalPorts: 80, PrivilegedPorts: 10}}
	if r.Summary.PrivilegedPorts != 10 {
		t.Errorf("expected 10")
	}
}
func TestTaintEffectResult2059(t *testing.T) {
	r := TaintEffectResult2059{Summary: TaintEffectSummary2059{TotalNodes: 5, NodesWithTaints: 1, TotalTaints: 2}}
	if r.Summary.TotalTaints != 2 {
		t.Errorf("expected 2")
	}
}
func TestCMKeyResult2059(t *testing.T) {
	r := CMKeyResult2059{Summary: CMKeySummary2059{TotalCMs: 50, TotalKeys: 200, LargeCMs: 3}}
	if r.Summary.LargeCMs != 3 {
		t.Errorf("expected 3")
	}
}
