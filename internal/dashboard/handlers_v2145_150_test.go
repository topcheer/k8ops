package dashboard

import "testing"

func TestOverheadResult2145(t *testing.T) {
	r := OverheadResult2145{Summary: OverheadSummary2145{TotalPods: 100, WithOverhead: 5}}
	if r.Summary.WithOverhead != 5 {
		t.Errorf("expected 5")
	}
}
func TestEphemeralResult2146(t *testing.T) {
	r := EphemeralResult2146{Summary: EphemeralSummary2146{TotalPods: 100, WithEphemeral: 2}}
	if r.Summary.WithEphemeral != 2 {
		t.Errorf("expected 2")
	}
}
func TestUnschedResult2147(t *testing.T) {
	r := UnschedResult2147{Summary: UnschedSummary2147{TotalNodes: 1, Schedulable: 1, Unschedulable: 0}}
	if r.Summary.Schedulable != 1 {
		t.Errorf("expected 1")
	}
}
func TestROFSResult2148(t *testing.T) {
	r := ROFSResult2148{Summary: ROFSSummary2148{TotalContainers: 200, ReadOnlyRoot: 50}}
	if r.Summary.ReadOnlyRoot != 50 {
		t.Errorf("expected 50")
	}
}
func TestCapGapResult2149(t *testing.T) {
	r := CapGapResult2149{Summary: CapGapSummary2149{TotalNodes: 1, GapPct: 10}}
	if r.Summary.GapPct != 10 {
		t.Errorf("expected 10")
	}
}
func TestMemForecastResult2150(t *testing.T) {
	r := MemForecastResult2150{Summary: MemForecastSummary2150{ForecastPct: 50}}
	if r.Summary.ForecastPct != 50 {
		t.Errorf("expected 50")
	}
}
func TestNSCPUQuotaResult2150(t *testing.T) {
	r := NSCPUQuotaResult2150{Summary: NSCPUQuotaSummary2150{TotalNS: 10}}
	if r.Summary.TotalNS != 10 {
		t.Errorf("expected 10")
	}
}
