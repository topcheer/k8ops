package dashboard

import "testing"

func TestPriorityClassResult2115(t *testing.T) {
	r := PriorityClassResult2115{Summary: PriorityClassSummary2115{TotalPods: 100, ByPriority: map[string]int{"none": 80, "system": 20}}}
	if r.Summary.TotalPods != 100 {
		t.Errorf("expected 100")
	}
}
func TestProbeTimeoutResult2116(t *testing.T) {
	r := ProbeTimeoutResult2116{Summary: ProbeTimeoutSummary2116{TotalContainers: 200, WithLiveness: 150, WithReadiness: 160}}
	if r.Summary.WithLiveness != 150 {
		t.Errorf("expected 150")
	}
}
func TestMachineInfoResult2117(t *testing.T) {
	r := MachineInfoResult2117{Summary: MachineInfoSummary2117{TotalNodes: 1, ByMachineID: map[string]int{"abc12345": 1}}}
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestSuppGroupResult2118(t *testing.T) {
	r := SuppGroupResult2118{Summary: SuppGroupSummary2118{TotalPods: 100, WithSuppGrp: 10}}
	if r.Summary.WithSuppGrp != 10 {
		t.Errorf("expected 10")
	}
}
func TestNodeAllocResult2119(t *testing.T) {
	r := NodeAllocResult2119{Summary: NodeAllocSummary2119{TotalNodes: 1, TotalPodCap: 110}}
	if r.Summary.TotalPodCap != 110 {
		t.Errorf("expected 110")
	}
}
func TestMemLimitAllocResult2120(t *testing.T) {
	r := MemLimitAllocResult2120{Summary: MemLimitAllocSummary2120{AllocatableMem: 16, LimitedMem: 8, RatioPct: 50}}
	if r.Summary.RatioPct != 50 {
		t.Errorf("expected 50")
	}
}
func TestBlastRadiusResult2120(t *testing.T) {
	r := BlastRadiusResult2120{Summary: BlastRadiusSummary2120{TotalNodes: 1, MaxPodsOnNode: 50}}
	if r.Summary.MaxPodsOnNode != 50 {
		t.Errorf("expected 50")
	}
}
