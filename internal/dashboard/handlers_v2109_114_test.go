package dashboard

import "testing"

func TestSubresResult2109(t *testing.T) {
	r := SubresResult2109{Summary: SubresSummary2109{TotalPods: 100, TotalContainers: 150}}
	if r.Summary.TotalContainers != 150 {
		t.Errorf("expected 150")
	}
}
func TestReadyGateResult2110(t *testing.T) {
	r := ReadyGateResult2110{Summary: ReadyGateSummary2110{TotalPods: 100, WithReadyGt: 5}}
	if r.Summary.WithReadyGt != 5 {
		t.Errorf("expected 5")
	}
}
func TestTaintEffResult2111(t *testing.T) {
	r := TaintEffResult2111{Summary: TaintEffSummary2111{TotalNodes: 1, NodesNoTaints: 1, NodesTainted: 0}}
	if r.Summary.NodesTainted != 0 {
		t.Errorf("expected 0")
	}
}
func TestSAPullResult2112(t *testing.T) {
	r := SAPullResult2112{Summary: SAPullSummary2112{TotalSAs: 50, WithPullSecret: 10}}
	if r.Summary.WithPullSecret != 10 {
		t.Errorf("expected 10")
	}
}
func TestLabelDivResult2113(t *testing.T) {
	r := LabelDivResult2113{Summary: LabelDivSummary2113{TotalNodes: 1, LabelKeys: map[string]int{"kubernetes.io/hostname": 1}}}
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestBurstHRResult2114(t *testing.T) {
	r := BurstHRResult2114{Summary: BurstHRSummary2114{AllocatableCPU: 8, LimitedCPU: 4, BurstHeadroom: 4}}
	if r.Summary.BurstHeadroom != 4 {
		t.Errorf("expected 4")
	}
}
func TestNSFootprintResult2114(t *testing.T) {
	r := NSFootprintResult2114{Summary: NSFootprintSummary2114{TotalNS: 5}}
	if r.Summary.TotalNS != 5 {
		t.Errorf("expected 5")
	}
}
