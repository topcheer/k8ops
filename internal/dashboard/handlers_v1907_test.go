package dashboard

import "testing"

func TestBackupSnapshotResult1907(t *testing.T) {
	r := BackupSnapshotResult{
		Summary:     BackupSnapSummary{TotalPVs: 15, ProtectedPVs: 5, UnprotectedPVs: 10},
		HealthScore: 33,
	}
	if r.Summary.UnprotectedPVs != 10 {
		t.Errorf("expected 10, got %d", r.Summary.UnprotectedPVs)
	}
}

func TestJobSuccessResult1907(t *testing.T) {
	r := JobSuccessResult{
		Summary:     JobSuccessSummary{TotalJobs: 50, SucceededJobs: 40, FailedJobs: 10, SuccessRate: 80.0},
		HealthScore: 80,
	}
	if r.Summary.SuccessRate != 80.0 {
		t.Errorf("expected 80.0, got %f", r.Summary.SuccessRate)
	}
}

func TestEventRetentionResult1907(t *testing.T) {
	r := EventRetentionResult{
		Summary:     EventRetentionSummary{TotalEvents: 5000, Events24h: 500, WarningEvents: 200, NoisyComponents: 3},
		HealthScore: 70,
	}
	if r.Summary.NoisyComponents != 3 {
		t.Errorf("expected 3, got %d", r.Summary.NoisyComponents)
	}
}

func TestBuildBackupSnapRecs1907(t *testing.T) {
	r := &BackupSnapshotResult{Summary: BackupSnapSummary{TotalPVs: 15, ProtectedPVs: 5, UnprotectedPVs: 10, VolumeSnapshots: 5}}
	r.UnprotectedNS = []string{"ns1", "ns2", "ns3"}
	recs := buildBackupSnapRecs1907(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildJobSuccessRecs1907(t *testing.T) {
	r := &JobSuccessResult{Summary: JobSuccessSummary{TotalJobs: 50, SuccessRate: 80.0, FailedJobs: 10, RunningJobs: 5, LongRunning: 3}}
	recs := buildJobSuccessRecs1907(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildEventRetentionRecs1907(t *testing.T) {
	r := &EventRetentionResult{Summary: EventRetentionSummary{TotalEvents: 5000, Events24h: 500, WarningEvents: 200, NoisyComponents: 3}}
	recs := buildEventRetentionRecs1907(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestMinInt1907(t *testing.T) {
	if minInt1907(3, 5) != 3 {
		t.Errorf("expected 3")
	}
	if minInt1907(7, 2) != 2 {
		t.Errorf("expected 2")
	}
}
