package dashboard

import (
	"testing"
)

func TestBuildCapacityGapRecs(t *testing.T) {
	r := &CapacityGapResult{
		Summary:        CapacityGapSummary{CPUHeadroom: 15, MemHeadroom: 10, PodHeadroom: 5},
		RiskAssessment: CapacityRisk{NodeLossSurvivable: false},
		Scenarios:      []CapacityScenario{{Name: "test", Survivable: false, Impact: "down"}},
	}
	recs := buildCapacityGapRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestEstimateExhaustion(t *testing.T) {
	v := estimateExhaustion(5, 20)
	if v == "unknown" {
		t.Error("expected a time estimate")
	}
	v2 := estimateExhaustion(0, 0)
	if v2 != "unknown" {
		t.Errorf("expected unknown for 0/0, got %s", v2)
	}
}

func TestSevFromBool(t *testing.T) {
	if sevFromBool(true) != "low" {
		t.Error("expected low")
	}
	if sevFromBool(false) != "critical" {
		t.Error("expected critical")
	}
}

func TestBuildDriftRecs(t *testing.T) {
	r := &RevisionDriftResult{
		Summary: DriftSummary{DriftDetected: 2, OldReplicaSets: 30, PodsInOldRevs: 5, MaxRevisions: 20},
	}
	recs := buildDriftRecs(r)
	if len(recs) < 4 {
		t.Errorf("expected >= 4 recs, got %d", len(recs))
	}
}
