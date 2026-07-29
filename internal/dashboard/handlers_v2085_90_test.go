package dashboard

import "testing"

func TestGraceShutdownResult2085(t *testing.T) {
	r := GraceShutdownResult2085{Summary: GraceShutdownSummary2085{TotalPods: 100, ShortGrace: 5, NoGrace: 10}}
	if r.Summary.ShortGrace != 5 {
		t.Errorf("expected 5")
	}
}
func TestMeshReadyResult2085(t *testing.T) {
	r := MeshReadyResult2085{Summary: MeshReadySummary2085{TotalPods: 100, SidecarPods: 30, MeshDetected: true}}
	if !r.Summary.MeshDetected {
		t.Errorf("expected true")
	}
}
func TestResGapWideResult2086(t *testing.T) {
	r := ResGapWideResult2086{Summary: ResGapWideSummary2086{TotalContainers: 200, WideGap: 15}}
	if r.Summary.WideGap != 15 {
		t.Errorf("expected 15")
	}
}
func TestQoSDistResult2086(t *testing.T) {
	r := QoSDistResult2086{Summary: QoSDistSummary2086{TotalPods: 100, Guaranteed: 20, Burstable: 60, BestEffort: 20}}
	if r.Summary.BestEffort != 20 {
		t.Errorf("expected 20")
	}
}
func TestRestartTrendResult2087(t *testing.T) {
	r := RestartTrendResult2087{Summary: RestartTrendSummary2087{TotalPods: 100, HighRestart: 3, TotalRestarts: 50}}
	if r.Summary.HighRestart != 3 {
		t.Errorf("expected 3")
	}
}
func TestSATokenAgeResult2088(t *testing.T) {
	r := SATokenAgeResult2088{Summary: SATokenAgeSummary2088{TotalTokens: 30, OldTokens: 5}}
	if r.Summary.OldTokens != 5 {
		t.Errorf("expected 5")
	}
}
func TestNodeCapSumResult2089(t *testing.T) {
	r := NodeCapSumResult2089{Summary: NodeCapSumSummary2089{TotalNodes: 1, TotalCPU: 8, TotalMem: 16, TotalPods: 110}}
	if r.Summary.TotalPods != 110 {
		t.Errorf("expected 110")
	}
}
func TestSchedLatResult2090(t *testing.T) {
	r := SchedLatResult2090{Summary: SchedLatSummary2090{TotalPods: 100, PendingPods: 2}}
	if r.Summary.PendingPods != 2 {
		t.Errorf("expected 2")
	}
}
func TestOvercommitResult2090(t *testing.T) {
	r := OvercommitResult2090{Summary: OvercommitSummary2090{CPUOvercommit: 80, MemOvercommit: 60}}
	if r.Summary.CPUOvercommit != 80 {
		t.Errorf("expected 80")
	}
}
func TestPodDensResult2090(t *testing.T) {
	r := PodDensResult2090{Summary: PodDensSummary2090{TotalNodes: 1, TotalPods: 50, AvgPerNode: 50}}
	if r.Summary.AvgPerNode != 50 {
		t.Errorf("expected 50")
	}
}
