package dashboard

import "testing"

func TestHPAThrashResult2066(t *testing.T) {
	r := HPAThrashResult2066{Summary: HPAThrashSummary2066{TotalHPAs: 10, AtRisk: 3, NoBehavior: 5}}
	if r.Summary.NoBehavior != 5 {
		t.Errorf("expected 5")
	}
}
func TestEvictReadyResult2066(t *testing.T) {
	r := EvictReadyResult2066{Summary: EvictReadySummary2066{TotalMultiReplica: 30, WithPDB: 10, UnsafeEviction: 20}}
	if r.Summary.UnsafeEviction != 20 {
		t.Errorf("expected 20")
	}
}
func TestClusterScaleHRResult2066(t *testing.T) {
	r := ClusterScaleHRResult2066{Summary: ClusterScaleHRSummary2066{TotalPodCapacity: 110, RunningPods: 92, HeadroomPods: 18, HeadroomPct: 16}}
	if r.Summary.HeadroomPct != 16 {
		t.Errorf("expected 16")
	}
}
