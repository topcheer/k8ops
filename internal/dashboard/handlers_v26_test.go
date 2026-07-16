package dashboard

import "testing"

func TestComputeVolBudgetScore(t *testing.T) {
	if s := computeVolBudgetScore(VolBudgetSummary{}); s != 100 {
		t.Errorf("expected 100, got %d", s)
	}
	s2 := VolBudgetSummary{TotalPVCs: 10, OrphanedCount: 3, PendingPVCs: 2}
	if s := computeVolBudgetScore(s2); s > 85 {
		t.Errorf("expected <= 85, got %d", s)
	}
}

func TestComputeRestartScore(t *testing.T) {
	if s := computeRestartScore(RestartPatSummary{}); s != 100 {
		t.Errorf("expected 100, got %d", s)
	}
	s2 := RestartPatSummary{ChronicCount: 2, CrashLoops: 1, OOMKills: 3}
	if s := computeRestartScore(s2); s > 70 {
		t.Errorf("expected <= 70, got %d", s)
	}
}

func TestComputeCertInvScore(t *testing.T) {
	if s := computeCertInvScore(CertInvSummary{}); s != 100 {
		t.Errorf("expected 100, got %d", s)
	}
	s2 := CertInvSummary{TotalCerts: 10, ExpiredCount: 2, Expiring7d: 1}
	if s := computeCertInvScore(s2); s > 60 {
		t.Errorf("expected <= 60, got %d", s)
	}
}
