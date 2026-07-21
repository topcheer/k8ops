package dashboard

import "testing"

func TestGracefulShutdownResult1900(t *testing.T) {
	r := GracefulShutdownResult{
		Summary:     ShutdownSummary{TotalWorkloads: 60, WithPreStop: 5, WithoutPreStop: 55},
		HealthScore: 8,
	}
	if r.Summary.WithoutPreStop != 55 {
		t.Errorf("expected 55, got %d", r.Summary.WithoutPreStop)
	}
}

func TestRolloutSpeedResult1900(t *testing.T) {
	r := RolloutSpeedResult{
		Summary:     RolloutSpeedSummary{TotalDeployments: 60, FastRollout: 55, SlowRollout: 5, RecreateCount: 6},
		HealthScore: 91,
	}
	if r.Summary.RecreateCount != 6 {
		t.Errorf("expected 6, got %d", r.Summary.RecreateCount)
	}
}

func TestDeployConflictResult1900(t *testing.T) {
	r := DeployConflictResult{
		Summary:     ConflictSummary{TotalWorkloads: 60, ConflictingPairs: 3, NameConflicts: 1, ConcurrentDeploys: 2},
		HealthScore: 95,
	}
	if r.Summary.NameConflicts != 1 {
		t.Errorf("expected 1, got %d", r.Summary.NameConflicts)
	}
}

func TestBuildShutdownRecs1900(t *testing.T) {
	r := &GracefulShutdownResult{Summary: ShutdownSummary{TotalWorkloads: 50, WithPreStop: 10, WithoutPreStop: 40}}
	recs := buildShutdownRecs1900(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildRolloutSpeedRecs1900(t *testing.T) {
	r := &RolloutSpeedResult{Summary: RolloutSpeedSummary{TotalDeployments: 50, FastRollout: 40, RecreateCount: 5}}
	recs := buildRolloutSpeedRecs1900(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildConflictRecs1900(t *testing.T) {
	r := &DeployConflictResult{Summary: ConflictSummary{TotalWorkloads: 50, ConflictingPairs: 5, ConcurrentDeploys: 2}}
	recs := buildConflictRecs1900(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
