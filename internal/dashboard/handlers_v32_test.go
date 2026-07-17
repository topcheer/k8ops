package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBuildOrphanRecs(t *testing.T) {
	r := &OrphanCleanupResult{
		Summary: OrphanCleanupSummary{
			OrphanedCount: 10, ConfigMaps: 5, Secrets: 3, PVCs: 2, SafeToDelete: 5,
		},
		PotentialSave: OrphanSave{EstimatedStorageGB: 50, EstimatedMonthlyCost: 5.0},
	}
	recs := buildOrphanRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestBuildOrphanRecsEmpty(t *testing.T) {
	r := &OrphanCleanupResult{Summary: OrphanCleanupSummary{OrphanedCount: 0}}
	recs := buildOrphanRecs(r)
	if len(recs) != 1 {
		t.Errorf("expected 1 rec for empty, got %d", len(recs))
	}
}

func TestBuildCostAnomalyRecs(t *testing.T) {
	r := &CostAnomalyResult{
		Summary: CostAnomalySummary{
			AnomalyCount: 5, OversizedCount: 3, IdleCount: 2, TotalMonthlyCost: 500,
		},
		EstimatedWaste: CostWasteEstimate{OversizedWaste: 50, WastePct: 25},
		TopSpenders: []CostSpender{
			{Name: "big-app", Namespace: "prod", MonthlyCost: 100},
		},
	}
	recs := buildCostAnomalyRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestShortHash(t *testing.T) {
	h1 := shortHash("test-data-here")
	h2 := shortHash("test-data-here")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	h3 := shortHash("different-data")
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
}

func TestGetImageList(t *testing.T) {
	containers := []corev1.Container{
		{Image: "nginx:1.21"},
		{Image: "redis:6"},
	}
	result := getImageList(containers)
	if result != "nginx:1.21,redis:6" {
		t.Errorf("expected comma-separated images, got %s", result)
	}
}
