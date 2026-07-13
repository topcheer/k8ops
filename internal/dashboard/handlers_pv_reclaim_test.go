package dashboard

import (
	"testing"
)

func TestComputePVReclaimScore(t *testing.T) {
	// No PVs → perfect
	score := computePVReclaimScore(PVReclaimSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// Failed PVs have highest impact
	score = computePVReclaimScore(PVReclaimSummary{
		TotalPVs:    20,
		FailedPVs:   2,
		PendingPVCs: 3,
		OrphanedPVs: 1,
	}, 5)
	if score > 55 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-55", score)
	}

	// All healthy
	score = computePVReclaimScore(PVReclaimSummary{
		TotalPVs:  20,
		BoundPVs:  20,
		TotalPVCs: 20,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Orphaned PVs have moderate impact
	score = computePVReclaimScore(PVReclaimSummary{
		TotalPVs:    10,
		OrphanedPVs: 3,
	}, 3)
	if score > 90 || score < 0 {
		t.Fatalf("orphaned score = %d, expected 0-90", score)
	}
}
