package dashboard

import "testing"

func TestRollingWindowResult2068(t *testing.T) {
	r := RollingWindowResult2068{Summary: RollingWindowSummary2068{TotalDeploys: 50, DefaultConfig: 20, CustomConfig: 30}}
	if r.Summary.CustomConfig != 30 {
		t.Errorf("expected 30")
	}
}
func TestPreemptResult2068(t *testing.T) {
	r := PreemptResult2068{Summary: PreemptSummary2068{TotalPods: 100, HighPriority: 5, LowPriority: 3, NoPriority: 50}}
	if r.Summary.NoPriority != 50 {
		t.Errorf("expected 50")
	}
}
func TestSSPartitionResult2068(t *testing.T) {
	r := SSPartitionResult2068{Summary: SSPartitionSummary2068{TotalSTS: 10, Partitioned: 2, StuckRollout: 1}}
	if r.Summary.StuckRollout != 1 {
		t.Errorf("expected 1")
	}
}
