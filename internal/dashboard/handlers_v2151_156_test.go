package dashboard

import "testing"

func TestShareProcResult2151(t *testing.T) {
	r := ShareProcResult2151{Summary: ShareProcSummary2151{TotalPods: 100, SharedProc: 3}}
	if r.Summary.SharedProc != 3 {
		t.Errorf("expected 3")
	}
}
func TestNodeSelReqResult2152(t *testing.T) {
	r := NodeSelReqResult2152{Summary: NodeSelReqSummary2152{TotalDeploys: 50, WithRequired: 10}}
	if r.Summary.WithRequired != 10 {
		t.Errorf("expected 10")
	}
}
func TestSysInfoResult2153(t *testing.T) {
	r := SysInfoResult2153{Summary: SysInfoSummary2153{TotalNodes: 1, KubeletVersion: "v1.29"}}
	if r.Summary.KubeletVersion != "v1.29" {
		t.Errorf("expected v1.29")
	}
}
func TestSeccompTypeResult2154(t *testing.T) {
	r := SeccompTypeResult2154{Summary: SeccompTypeSummary2154{TotalPods: 100, ByType: map[string]int{"Unconfined": 80}}}
	if r.Summary.TotalPods != 100 {
		t.Errorf("expected 100")
	}
}
func TestTaintKeyResult2155(t *testing.T) {
	r := TaintKeyResult2155{Summary: TaintKeySummary2155{TotalNodes: 1, TaintKeyCount: map[string]int{"node.kubernetes.io/not-ready": 0}}}
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestCPUConcResult2156(t *testing.T) {
	r := CPUConcResult2156{Summary: CPUConcSummary2156{TotalNodes: 1}}
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestNSMultiHAResult2156(t *testing.T) {
	r := NSMultiHAResult2156{Summary: NSMultiHASummary2156{TotalNS: 5}}
	if r.Summary.TotalNS != 5 {
		t.Errorf("expected 5")
	}
}
