package dashboard

import "testing"

func TestDRPlanResult1914(t *testing.T) {
	r := DRPlanResult{
		Summary:     DRPlanSummary{TotalWorkloads: 64, StatefulWorkloads: 5, TotalPVCs: 15, ProtectedPVCs: 0, RTOHours: 8, RPOHours: 24},
		HealthScore: 0,
	}
	if r.Summary.StatefulWorkloads != 5 {
		t.Errorf("expected 5, got %d", r.Summary.StatefulWorkloads)
	}
}

func TestADRResult1914(t *testing.T) {
	r := ADRResult{
		Summary:     ADRSummary{TotalADRs: 5, CriticalDecisions: 2, PendingReview: 1},
		HealthScore: 100,
	}
	if r.Summary.PendingReview != 1 {
		t.Errorf("expected 1, got %d", r.Summary.PendingReview)
	}
}

func TestMigrationCheckResult1914(t *testing.T) {
	r := MigrationCheckResult{
		Summary:     MigrationSummary{TotalItems: 12, Completed: 1, Pending: 11, EstimatedHrs: 21},
		HealthScore: 8,
	}
	if r.Summary.EstimatedHrs != 21 {
		t.Errorf("expected 21, got %d", r.Summary.EstimatedHrs)
	}
}

func TestBuildDRPlanRecs1914(t *testing.T) {
	r := &DRPlanResult{
		Summary:       DRPlanSummary{TotalWorkloads: 64, StatefulWorkloads: 5, TotalPVCs: 15, ProtectedPVCs: 0, RTOHours: 8, RPOHours: 24},
		RTOAssessment: DRPlanRTO{Target: 4, Estimated: 8, Met: false},
	}
	recs := buildDRPlanRecs1914(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildADRRecs1914(t *testing.T) {
	r := &ADRResult{Summary: ADRSummary{TotalADRs: 5, CriticalDecisions: 2, PendingReview: 1}}
	recs := buildADRRecs1914(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildMigrationRecs1914(t *testing.T) {
	r := &MigrationCheckResult{Summary: MigrationSummary{TotalItems: 12, Completed: 1, Pending: 11, EstimatedHrs: 21}}
	recs := buildMigrationRecs1914(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
