package dashboard

import (
	"testing"
)

func TestPlatformRiskHeatmapResultStruct1872(t *testing.T) {
	r := PlatformRiskHeatmapResult{
		Summary: PlatformHeatmapSummary{
			TotalNamespaces: 20,
			CriticalNS:      3,
			HighRiskNS:      5,
			LowRiskNS:       8,
		},
		OverallScore: 55,
	}
	if r.Summary.CriticalNS != 3 {
		t.Errorf("expected 3, got %d", r.Summary.CriticalNS)
	}
}

func TestWorkloadMaturityMatrixResultStruct1872(t *testing.T) {
	r := WorkloadMaturityMatrixResult{
		Summary: MaturityMatrixSummary{
			TotalWorkloads:    30,
			EliteWorkloads:    2,
			AdvancedWorkloads: 5,
			MidWorkloads:      10,
			BasicWorkloads:    13,
		},
		OverallScore: 35,
	}
	if r.Summary.EliteWorkloads != 2 {
		t.Errorf("expected 2, got %d", r.Summary.EliteWorkloads)
	}
}

func TestIncidentPlaybookResultStruct1872(t *testing.T) {
	r := IncidentPlaybookResult{
		Summary: PlaybookSummary{
			TotalScenarios:  6,
			ReadyScenarios:  3,
			GappedScenarios: 3,
			CoveragePct:     50,
		},
		ReadinessScore: 50,
	}
	if r.Summary.TotalScenarios != 6 {
		t.Errorf("expected 6, got %d", r.Summary.TotalScenarios)
	}
}

func TestMinInt(t *testing.T) {
	if minInt1872(3, 5) != 3 {
		t.Error("expected 3")
	}
	if minInt1872(5, 3) != 3 {
		t.Error("expected 3")
	}
}
