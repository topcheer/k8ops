package dashboard

import "testing"

func TestStartupProbeResult2103(t *testing.T) {
	r := StartupProbeResult2103{Summary: StartupProbeSummary2103{TotalContainers: 200, WithStartup: 30}}
	if r.Summary.WithStartup != 30 {
		t.Errorf("expected 30")
	}
}
func TestStrategyValResult2104(t *testing.T) {
	r := StrategyValResult2104{Summary: StrategyValSummary2104{TotalDeploys: 50, RollingUpdate: 45, Recreate: 5}}
	if r.Summary.Recreate != 5 {
		t.Errorf("expected 5")
	}
}
func TestAllocPodResult2105(t *testing.T) {
	r := AllocPodResult2105{Summary: AllocPodSummary2105{TotalNodes: 1, TotalPods: 50, AvgPodRatio: 45}}
	if r.Summary.AvgPodRatio != 45 {
		t.Errorf("expected 45")
	}
}
func TestSATokenMountResult2106(t *testing.T) {
	r := SATokenMountResult2106{Summary: SATokenMountSummary2106{TotalPods: 100, AutoMount: 90}}
	if r.Summary.AutoMount != 90 {
		t.Errorf("expected 90")
	}
}
func TestPVCSCResult2107(t *testing.T) {
	r := PVCSCResult2107{Summary: PVCSCSummary2107{TotalPVCs: 20, BySC: map[string]int{"local-path": 15, "fast": 5}}}
	if r.Summary.TotalPVCs != 20 {
		t.Errorf("expected 20")
	}
}
func TestMemEffResult2108(t *testing.T) {
	r := MemEffResult2108{Summary: MemEffSummary2108{AllocatableMem: 16, RequestedMem: 8, EfficiencyPct: 50}}
	if r.Summary.EfficiencyPct != 50 {
		t.Errorf("expected 50")
	}
}
func TestReplicaConcResult2108(t *testing.T) {
	r := ReplicaConcResult2108{Summary: ReplicaConcSummary2108{TotalDeploys: 50, TotalReplicas: 150}}
	if r.Summary.TotalReplicas != 150 {
		t.Errorf("expected 150")
	}
}
