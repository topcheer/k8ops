package dashboard

import (
	"testing"
)

func TestBuildPodSLORecs(t *testing.T) {
	r := &PodSLOResult{
		Summary: PodSLOSummary{MeetingSLO: 8, TotalWorkloads: 10, TargetAvailability: 99.9, BreachingSLO: 2, AvgAvailability: 98.5},
	}
	recs := buildPodSLORecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestBuildReadinessGateRecs(t *testing.T) {
	r := &DeployReadinessGateResult{
		Summary:    DeployGateSummary{ReadyToDeploy: 5, TotalWorkloads: 20, Blocked: 15},
		GateChecks: []DeployGateCheck{{Name: "Probes", PassRate: 30}, {Name: "HPA", PassRate: 20}},
	}
	recs := buildReadinessGateRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestBuildGovScoreRecs(t *testing.T) {
	r := &APIGovernanceScoreResult{
		Summary: GovSummary{StableAPIs: 40, BetaAPIs: 10, DeprecatedAPIs: 3},
	}
	recs := buildGovScoreRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}
