package dashboard

import (
	"testing"
)

func TestRolloutBlockerResultStruct1889(t *testing.T) {
	r := RolloutBlockerResult{
		Summary:     RolloutBlockerSummary{TotalDeployments: 59, Healthy: 55, Blocked: 4},
		HealthScore: 93,
	}
	if r.Summary.Blocked != 4 {
		t.Errorf("expected 4, got %d", r.Summary.Blocked)
	}
}

func TestTerminationGraceResultStruct1889(t *testing.T) {
	r := TerminationGraceResult{
		Summary:     TermGraceSummary{TotalWorkloads: 59, WithPreStop: 5, NoPreStop: 54},
		HealthScore: 10,
	}
	if r.Summary.NoPreStop != 54 {
		t.Errorf("expected 54, got %d", r.Summary.NoPreStop)
	}
}

func TestMaxSurgeAuditResultStruct1889(t *testing.T) {
	r := MaxSurgeAuditResult{
		Summary:     MaxSurgeSummary{TotalDeployments: 59, RollingUpdate: 43, Recreate: 5},
		HealthScore: 75,
	}
	if r.Summary.Recreate != 5 {
		t.Errorf("expected 5, got %d", r.Summary.Recreate)
	}
}
