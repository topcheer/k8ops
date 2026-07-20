package dashboard

import (
	"testing"
)

func TestCostOptimizationRoadmapResultStruct1884(t *testing.T) {
	r := CostOptimizationRoadmapResult{
		Summary:      CostRoadmapSummary{TotalActions: 6, QuickWinCount: 2, EstSavingsMo: 150.0},
		TotalSavings: 150.0,
	}
	if r.Summary.QuickWinCount != 2 {
		t.Errorf("expected 2, got %d", r.Summary.QuickWinCount)
	}
}

func TestSecurityPostureTrendResultStruct1884(t *testing.T) {
	r := SecurityPostureTrendResult{
		Summary:     SecPostureTrendSummary{TotalPods: 83, RunningAsRoot: 70, PrivilegedCount: 3},
		HealthScore: 15,
	}
	if r.Summary.PrivilegedCount != 3 {
		t.Errorf("expected 3, got %d", r.Summary.PrivilegedCount)
	}
}

func TestCapacityPlanningReportResultStruct1884(t *testing.T) {
	r := CapacityPlanningReportResult1884{
		Summary:     CapacityPlanReportSummary{TotalPods: 97, NodeCount: 1, NodesNeeded6Mo1884: 2},
		HealthScore: 0,
	}
	if r.Summary.NodesNeeded6Mo1884 != 2 {
		t.Errorf("expected 2, got %d", r.Summary.NodesNeeded6Mo1884)
	}
}
