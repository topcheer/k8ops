package dashboard

import (
	"testing"
)

func TestPDBGapAnalysisResultStruct1887(t *testing.T) {
	r := PDBGapAnalysisResult{
		Summary:     PDBGapSummary{TotalDeployments: 59, MultiReplicaDeps: 2, WithoutPDB: 2},
		HealthScore: 0,
	}
	if r.Summary.WithoutPDB != 2 {
		t.Errorf("expected 2, got %d", r.Summary.WithoutPDB)
	}
}

func TestTopologySpreadViolationResultStruct1887(t *testing.T) {
	r := TopoViolationResult{
		Summary:     TopoViolationSummary{TotalWorkloads: 59, MultiReplica: 2, SingleNode: 2},
		HealthScore: 0,
	}
	if r.Summary.SingleNode != 2 {
		t.Errorf("expected 2, got %d", r.Summary.SingleNode)
	}
}

func TestOvercommitDeepResultStruct1887(t *testing.T) {
	r := OvercommitDeepResult{
		Summary:     OvercommitDeepSummary{TotalWorkloads: 59, CPUReqVsLimit: 2.5, OvercommitRatio: 2.0},
		HealthScore: 80,
	}
	if r.Summary.OvercommitRatio != 2.0 {
		t.Errorf("expected 2.0, got %.1f", r.Summary.OvercommitRatio)
	}
}
