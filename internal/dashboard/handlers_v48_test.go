package dashboard

import (
	"testing"
)

func TestBuildEventCorrRecs(t *testing.T) {
	r := &EventCorrelationDeepResult{
		Summary:    EventCorrSummary{CascadeChains: 5, RootCauseCount: 3, CorrelatedEvents: 50, TotalEvents: 100},
		RootCauses: []EventRootCause{{Kind: "Pod", Reason: "FailedScheduling", Count: 8, RootCause: "test", Fix: "fix"}},
	}
	recs := buildEventCorrRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestBuildRollbackSimRecs(t *testing.T) {
	r := &RollbackSimulatorResult{
		Summary:        RollbackSimSummary{SafeRollbacks: 10, RiskyRollbacks: 3, NoHistory: 5},
		RiskyRollbacks: []RollbackSimEntry{{Namespace: "prod", Workload: "db", RiskScore: 40}},
	}
	recs := buildRollbackSimRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
