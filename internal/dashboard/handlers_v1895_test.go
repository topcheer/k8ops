package dashboard

import (
	"testing"
)

func TestPodPhaseTimelineResultStruct1895(t *testing.T) {
	r := PodPhaseTimelineResult{
		Summary: PodPhaseSummary{
			TotalPods:   104,
			Running:     100,
			Pending:     2,
			Failed:      1,
			StalePods:   3,
			LongPending: 2,
		},
		HealthScore: 96,
	}
	if r.Summary.Running != 100 {
		t.Errorf("expected 100 running, got %d", r.Summary.Running)
	}
}

func TestImageGCResultStruct1895(t *testing.T) {
	r := ImageGCResult{
		Summary: ImageGCSummary{
			TotalImages:    93,
			UniqueImages:   80,
			DuplicateCount: 13,
			UnusedCount:    5,
		},
		HealthScore: 86,
	}
	if r.Summary.DuplicateCount != 13 {
		t.Errorf("expected 13 duplicates, got %d", r.Summary.DuplicateCount)
	}
}

func TestControllerReconcileResultStruct1895(t *testing.T) {
	r := ControllerReconcileResult{
		Summary: ReconcileSummary{
			TotalWorkloads:       60,
			HealthyControllers:   55,
			UnhealthyControllers: 5,
			Mismatch:             3,
			OrphanedWorkloads:    2,
		},
		HealthScore: 80,
	}
	if r.Summary.UnhealthyControllers != 5 {
		t.Errorf("expected 5 unhealthy, got %d", r.Summary.UnhealthyControllers)
	}
}

func TestBuildPodPhaseRecs1895(t *testing.T) {
	result := &PodPhaseTimelineResult{
		Summary: PodPhaseSummary{
			TotalPods: 100, Running: 95, Pending: 3, Failed: 1,
			LongPending: 2, StalePods: 1,
		},
	}
	recs := buildPodPhaseRecs1895(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}

func TestBuildImageGCRecs1895(t *testing.T) {
	result := &ImageGCResult{
		Summary: ImageGCSummary{
			TotalImages: 90, UniqueImages: 75, DuplicateCount: 15,
			EstimatedSavingsMB: 3000,
		},
	}
	recs := buildImageGCRecs1895(result)
	if len(recs) < 1 {
		t.Errorf("expected recs, got %d", len(recs))
	}
}

func TestBuildReconcileRecs1895(t *testing.T) {
	result := &ControllerReconcileResult{
		Summary: ReconcileSummary{
			TotalWorkloads: 50, HealthyControllers: 40,
			UnhealthyControllers: 10, Mismatch: 5, OrphanedWorkloads: 3,
		},
	}
	recs := buildReconcileRecs1895(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}
