package dashboard

import "testing"

func TestBackupScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  BackupSummary
		minScore int
		maxScore int
	}{
		{"no PVCs", BackupSummary{TotalPVCs: 0}, 95, 100},
		{"all protected", BackupSummary{TotalPVCs: 10, ProtectedPVCs: 10, HasVelero: true}, 90, 100},
		{"half unprotected", BackupSummary{TotalPVCs: 10, UnprotectedPVCs: 5, ProtectedPVCs: 5}, 60, 80},
		{"critical unprotected", BackupSummary{TotalPVCs: 10, UnprotectedPVCs: 5, CriticalPVCs: 3}, 30, 60},
		{"no velero many PVCs", BackupSummary{TotalPVCs: 20, ProtectedPVCs: 15, UnprotectedPVCs: 5, HasVelero: false}, 70, 85},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := backupScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestBackupRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &BackupResult{Summary: BackupSummary{TotalPVCs: 10, ProtectedPVCs: 10, HasVelero: true}}
		recs := backupRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &BackupResult{Summary: BackupSummary{
			TotalPVCs: 20, UnprotectedPVCs: 10, ProtectedPVCs: 10,
			CriticalPVCs: 3, HasVelero: false,
		}}
		recs := backupRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
