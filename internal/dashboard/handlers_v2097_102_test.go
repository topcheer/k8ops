package dashboard

import "testing"

func TestWaitReasonResult2097(t *testing.T) {
	r := WaitReasonResult2097{Summary: WaitReasonSummary2097{TotalPods: 100, WaitingPods: 3}}
	if r.Summary.WaitingPods != 3 {
		t.Errorf("expected 3")
	}
}
func TestPausedResult2098(t *testing.T) {
	r := PausedResult2098{Summary: PausedSummary2098{TotalDeploys: 50, Paused: 2}}
	if r.Summary.Paused != 2 {
		t.Errorf("expected 2")
	}
}
func TestCtnrStateResult2099(t *testing.T) {
	r := CtnrStateResult2099{Summary: CtnrStateSummary2099{TotalContainers: 200, Running: 190, Waiting: 5, Terminated: 5}}
	if r.Summary.Running != 190 {
		t.Errorf("expected 190")
	}
}
func TestNPEmptyResult2100(t *testing.T) {
	r := NPEmptyResult2100{Summary: NPEmptySummary2100{TotalNP: 20, BroadNP: 3}}
	if r.Summary.BroadNP != 3 {
		t.Errorf("expected 3")
	}
}
func TestBootIDResult2101(t *testing.T) {
	r := BootIDResult2101{Summary: BootIDSummary2101{TotalNodes: 1}}
	if r.Summary.TotalNodes != 1 {
		t.Errorf("expected 1")
	}
}
func TestCPUWasteResult2102(t *testing.T) {
	r := CPUWasteResult2102{Summary: CPUWasteSummary2102{TotalRequestedCPU: 2, TotalAllocatable: 8, WastePct: 75}}
	if r.Summary.WastePct != 75 {
		t.Errorf("expected 75")
	}
}
func TestNSQuotaHRResult2102(t *testing.T) {
	r := NSQuotaHRResult2102{Summary: NSQuotaHRSummary2102{TotalNS: 20, NearQuota: 3}}
	if r.Summary.NearQuota != 3 {
		t.Errorf("expected 3")
	}
}
