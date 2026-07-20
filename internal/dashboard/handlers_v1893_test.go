package dashboard

import (
	"testing"
)

func TestMemPressureResultStruct1893(t *testing.T) {
	r := MemPressureResult{
		Summary: MemPressureSummary{
			TotalNodes:         1,
			TotalAllocatableMB: 16384,
			TotalRequestedMB:   10440,
			OvercommitRatio:    0.64,
			CriticalNodes:      0,
			WarningNodes:       1,
			HealthyNodes:       0,
		},
		HealthScore: 80,
	}
	if r.Summary.TotalRequestedMB != 10440 {
		t.Errorf("expected 10440MB requested, got %d", r.Summary.TotalRequestedMB)
	}
}

func TestScaleConcurrencyResultStruct1893(t *testing.T) {
	r := ScaleConcurrencyResult{
		Summary: ScaleConcurrencySummary{
			MaxConcurrentScale: 15,
			TotalSpareCPUm:     2000,
			TotalSpareMemMB:    6000,
			TotalSparePods:     20,
			Bottleneck:         "cpu",
		},
		HealthScore: 65,
	}
	if r.Summary.MaxConcurrentScale != 15 {
		t.Errorf("expected 15, got %d", r.Summary.MaxConcurrentScale)
	}
	if r.Summary.Bottleneck != "cpu" {
		t.Errorf("expected cpu bottleneck, got %s", r.Summary.Bottleneck)
	}
}

func TestDisruptionWindowResultStruct1893(t *testing.T) {
	r := DisruptionWindowResult{
		Summary: DisruptionWindowSummary{
			TotalWorkloads:   60,
			WithPDB:          5,
			WithoutPDB:       55,
			MaxDisruptable:   3,
			SingleReplica:    40,
			DisruptionBudget: 3,
		},
		HealthScore: 5,
	}
	if r.Summary.DisruptionBudget != 3 {
		t.Errorf("expected budget 3, got %d", r.Summary.DisruptionBudget)
	}
	if r.Summary.SingleReplica != 40 {
		t.Errorf("expected 40 single-replica, got %d", r.Summary.SingleReplica)
	}
}

func TestMinInt1893(t *testing.T) {
	if minInt1893(5, 10) != 5 {
		t.Errorf("expected 5, got %d", minInt1893(5, 10))
	}
	if minInt1893(10, 5) != 5 {
		t.Errorf("expected 5, got %d", minInt1893(10, 5))
	}
	if minInt1893(-5, 5) != -5 {
		t.Errorf("expected -5, got %d", minInt1893(-5, 5))
	}
}

func TestBuildMemPressureRecs1893(t *testing.T) {
	result := &MemPressureResult{
		Summary: MemPressureSummary{
			TotalNodes:      1,
			CriticalNodes:   0,
			WarningNodes:    1,
			HealthyNodes:    0,
			OvercommitRatio: 0.64,
		},
	}
	recs := buildMemPressureRecs1893(result)
	if len(recs) == 0 {
		t.Error("expected recommendations, got 0")
	}
}

func TestBuildScaleConcurrencyRecs1893(t *testing.T) {
	result := &ScaleConcurrencyResult{
		Summary: ScaleConcurrencySummary{
			MaxConcurrentScale: 15,
			Bottleneck:         "cpu",
			TotalSpareCPUm:     2000,
			TotalSpareMemMB:    6000,
			TotalSparePods:     20,
		},
	}
	recs := buildScaleConcurrencyRecs1893(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(recs))
	}
}

func TestBuildDisruptionWindowRecs1893(t *testing.T) {
	result := &DisruptionWindowResult{
		Summary: DisruptionWindowSummary{
			TotalWorkloads:   40,
			WithPDB:          5,
			WithoutPDB:       35,
			SingleReplica:    25,
			DisruptionBudget: 2,
		},
	}
	recs := buildDisruptionWindowRecs1893(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(recs))
	}
}
