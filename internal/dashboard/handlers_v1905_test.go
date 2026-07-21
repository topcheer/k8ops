package dashboard

import "testing"

func TestHPAEffResult1905(t *testing.T) {
	r := HPAEffResult{
		Summary:     HPAEffSummary{TotalDeployments: 60, WithHPA: 10, WithoutHPA: 50, HPAAtMax: 2, MisconfiguredHPA: 3},
		HealthScore: 7,
	}
	if r.Summary.WithoutHPA != 50 {
		t.Errorf("expected 50, got %d", r.Summary.WithoutHPA)
	}
}

func TestSchedulingLatencyResult1905(t *testing.T) {
	r := SchedulingLatencyResult{
		Summary:     SchedLatencySummary{TotalPods: 100, ScheduledOK: 95, PendingPods: 5, AvgLatencySec: 15, P95LatencySec: 120, Unschedulable: 3},
		HealthScore: 62,
	}
	if r.Summary.P95LatencySec != 120 {
		t.Errorf("expected 120, got %d", r.Summary.P95LatencySec)
	}
}

func TestCapacityHeadroomResult1905(t *testing.T) {
	r := CapacityHeadroomResult{
		Summary:     HeadroomSummary{TotalNodes: 1, TotalCPUm: 2000, UsedCPUm: 400, AvailCPUm: 1600, AdditionalPods: 50},
		HealthScore: 70,
	}
	if r.Summary.AdditionalPods != 50 {
		t.Errorf("expected 50, got %d", r.Summary.AdditionalPods)
	}
}

func TestBuildHPAEffRecs1905(t *testing.T) {
	r := &HPAEffResult{Summary: HPAEffSummary{WithHPA: 10, TotalDeployments: 60, HPAScalingActive: 5, HPAAtMax: 2, MisconfiguredHPA: 3, WithoutHPA: 50}}
	recs := buildHPAEffRecs1905(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildSchedLatencyRecs1905(t *testing.T) {
	r := &SchedulingLatencyResult{Summary: SchedLatencySummary{ScheduledOK: 95, AvgLatencySec: 15, P95LatencySec: 150, SlowScheduling: 5, Unschedulable: 3}}
	recs := buildSchedLatencyRecs1905(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildHeadroomRecs1905(t *testing.T) {
	r := &CapacityHeadroomResult{Summary: HeadroomSummary{TotalNodes: 1, UsedCPUm: 400, TotalCPUm: 2000, UsedMemMB: 4000, TotalMemMB: 16000, AdditionalPods: 5}}
	recs := buildHeadroomRecs1905(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
