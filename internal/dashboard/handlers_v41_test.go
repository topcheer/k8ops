package dashboard

import (
	"testing"
)

func TestBuildLabelScoreRecs(t *testing.T) {
	r := &LabelScoreResult{
		Summary: LabelScoreSummary{NoLabels: 3, WithTeam: 0},
		StandardLabels: []LabelStandardStat{
			{Label: "app", Coverage: 30},
			{Label: "version", Coverage: 20},
		},
	}
	recs := buildLabelScoreRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestHasLabel(t *testing.T) {
	labels := map[string]string{"app": "web", "team": "dev"}
	if !hasLabel(labels, "app") {
		t.Error("expected true for existing label")
	}
	if hasLabel(labels, "owner") {
		t.Error("expected false for missing label")
	}
}

func TestBuildStorageTierRecs(t *testing.T) {
	r := &StorageTierResult{
		Summary: StorageTierSummary{PendingPVCs: 2, OrphanedPVCs: 1, TotalSizeGB: 600, StorageClasses: 1},
	}
	recs := buildStorageTierRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestBuildTrustChainRecs(t *testing.T) {
	r := &TrustChainResult{
		Summary: TrustChainSummary{ExpiredCerts: 1, ExpiringSoon: 2, OldTokens: 3, WebhookNoTLS: 1},
	}
	recs := buildTrustChainRecs(r)
	if len(recs) < 4 {
		t.Errorf("expected >= 4 recs, got %d", len(recs))
	}
}
