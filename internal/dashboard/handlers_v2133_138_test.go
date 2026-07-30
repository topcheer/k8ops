package dashboard

import "testing"

func TestSAOverrideResult2133(t *testing.T) {
	r := SAOverrideResult2133{Summary: SAOverrideSummary2133{TotalPods: 100, Disabled: 10}}
	if r.Summary.Disabled != 10 {
		t.Errorf("expected 10")
	}
}
func TestSchedulerNameResult2134(t *testing.T) {
	r := SchedulerNameResult2134{Summary: SchedulerNameSummary2134{TotalPods: 100, ByScheduler: map[string]int{"default-scheduler": 100}}}
	if r.Summary.TotalPods != 100 {
		t.Errorf("expected 100")
	}
}
func TestNodeTransResult2135(t *testing.T) {
	r := NodeTransResult2135{Summary: NodeTransSummary2135{TotalNodes: 1, ReadyNodes: 1, NotReadyNodes: 0}}
	if r.Summary.ReadyNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestHostPathWriteResult2136(t *testing.T) {
	r := HostPathWriteResult2136{Summary: HostPathWriteSummary2136{TotalPods: 100, WithHostPath: 5, Writable: 3}}
	if r.Summary.Writable != 3 {
		t.Errorf("expected 3")
	}
}
func TestProviderIDResult2137(t *testing.T) {
	r := ProviderIDResult2137{Summary: ProviderIDSummary2137{TotalNodes: 1, ByProvider: map[string]int{"kind": 1}}}
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestCPUOvercommitNodeResult2138(t *testing.T) {
	r := CPUOvercommitNodeResult2138{Summary: CPUOvercommitNodeSummary2138{TotalNodes: 1, OverNodes: 0}}
	if r.Summary.OverNodes != 0 {
		t.Errorf("expected 0")
	}
}
func TestNSHAMultResult2138(t *testing.T) {
	r := NSHAMultResult2138{Summary: NSHAMultSummary2138{TotalNS: 5, LowHA: 2}}
	if r.Summary.LowHA != 2 {
		t.Errorf("expected 2")
	}
}
