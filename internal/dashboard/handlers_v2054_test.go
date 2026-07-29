package dashboard

import "testing"

func TestHPAMetricResult2054(t *testing.T) {
	r := HPAMetricResult2054{Summary: HPAMetricSummary2054{TotalHPAs: 10, CPUMetrics: 8, MemMetrics: 3, CustomMetrics: 2}}
	if r.Summary.CPUMetrics != 8 {
		t.Errorf("expected 8")
	}
}
func TestAntiAffinityResult2054(t *testing.T) {
	r := AntiAffinityResult2054{Summary: AntiAffinitySummary2054{TotalMultiReplica: 30, WithAntiAffinity: 15, MissingAA: 15}}
	if r.Summary.MissingAA != 15 {
		t.Errorf("expected 15")
	}
}
func TestClusterCapResult2054(t *testing.T) {
	r := ClusterCapResult2054{Summary: ClusterCapSummary2054{TotalCapacityCPU: 8, AllocatableCPU: 6, RequestedCPU: 3, HeadroomCPUPct: 50}}
	if r.Summary.HeadroomCPUPct != 50 {
		t.Errorf("expected 50")
	}
}
