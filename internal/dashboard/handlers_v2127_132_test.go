package dashboard

import "testing"

func TestActiveDeadlineResult2127(t *testing.T) {
	r := ActiveDeadlineResult2127{Summary: ActiveDeadlineSummary2127{TotalPods: 100, WithDeadline: 5}}
	if r.Summary.WithDeadline != 5 {
		t.Errorf("expected 5")
	}
}
func TestResDeltaResult2128(t *testing.T) {
	r := ResDeltaResult2128{Summary: ResDeltaSummary2128{TotalContainers: 200, LargeDelta: 30}}
	if r.Summary.LargeDelta != 30 {
		t.Errorf("expected 30")
	}
}
func TestAllocEffRatioResult2129(t *testing.T) {
	r := AllocEffRatioResult2129{Summary: AllocEffRatioSummary2129{TotalNodes: 1, AvgCPUAllocPct: 90}}
	if r.Summary.AvgCPUAllocPct != 90 {
		t.Errorf("expected 90")
	}
}
func TestHostPIDResult2130(t *testing.T) {
	r := HostPIDResult2130{Summary: HostPIDSummary2130{TotalPods: 100, HostPID: 2, HostIPC: 1}}
	if r.Summary.HostPID != 2 {
		t.Errorf("expected 2")
	}
}
func TestEventReasonResult2131(t *testing.T) {
	r := EventReasonResult2131{Summary: EventReasonSummary2131{TotalEvents: 1000}}
	if r.Summary.TotalEvents != 1000 {
		t.Errorf("expected 1000")
	}
}
func TestMemEffNSResult2132(t *testing.T) {
	r := MemEffNSResult2132{Summary: MemEffNSSummary2132{TotalNS: 5}}
	if r.Summary.TotalNS != 5 {
		t.Errorf("expected 5")
	}
}
func TestHPATargetResult2132(t *testing.T) {
	r := HPATargetResult2132{Summary: HPATargetSummary2132{TotalHPAs: 10, Mismatched: 1}}
	if r.Summary.Mismatched != 1 {
		t.Errorf("expected 1")
	}
}
