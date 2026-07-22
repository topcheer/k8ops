package dashboard

import "testing"

func TestStorageLatencyResult1918(t *testing.T) {
	r := StorageLatencyResult{
		Summary:     StorageLatSummary{TotalPVCs: 15, LocalStorageGB: 0, NetworkStorageGB: 50, HighRiskCount: 3},
		HealthScore: 80,
	}
	if r.Summary.HighRiskCount != 3 {
		t.Errorf("expected 3, got %d", r.Summary.HighRiskCount)
	}
}

func TestPacketLossResult1918(t *testing.T) {
	r := PacketLossResult{
		Summary:     PacketLossSummary{TotalNodes: 1, ReadyNodes: 1, PodsNotReady: 2, HighRiskNodes: 0, SvcWithoutEP: 0},
		HealthScore: 100,
	}
	if r.Summary.PodsNotReady != 2 {
		t.Errorf("expected 2, got %d", r.Summary.PodsNotReady)
	}
}

func TestCgroupPressureResult1918(t *testing.T) {
	r := CgroupPressureResult{
		Summary:     CgroupPressureSummary{TotalPods: 80, WithoutLimits: 48, ThrottleRisk: 3, OOMRiskCount: 5},
		HealthScore: 90,
	}
	if r.Summary.WithoutLimits != 48 {
		t.Errorf("expected 48, got %d", r.Summary.WithoutLimits)
	}
}

func TestBuildStorageLatRecs1918(t *testing.T) {
	r := &StorageLatencyResult{Summary: StorageLatSummary{TotalPVCs: 15, LocalStorageGB: 0, NetworkStorageGB: 50, HighRiskCount: 3}}
	recs := buildStorageLatRecs1918(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildPacketLossRecs1918(t *testing.T) {
	r := &PacketLossResult{Summary: PacketLossSummary{TotalNodes: 1, ReadyNodes: 1, PodsNotReady: 5, HighRiskNodes: 0, SvcWithoutEP: 1}}
	recs := buildPacketLossRecs1918(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildCgroupRecs1918(t *testing.T) {
	r := &CgroupPressureResult{Summary: CgroupPressureSummary{TotalPods: 80, WithoutLimits: 48, ThrottleRisk: 3, OOMRiskCount: 5}}
	recs := buildCgroupRecs1918(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}
