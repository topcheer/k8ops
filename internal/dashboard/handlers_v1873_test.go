package dashboard

import (
	"testing"
)

func TestCanaryHealthResultStruct1873(t *testing.T) {
	r := CanaryHealthResult{
		Summary: CanaryHealthSummary{
			TotalDeployments: 49,
			WithCanary:       0,
			HealthyRollouts:  40,
			StalledRollouts:  3,
		},
		HealthScore: 81,
	}
	if r.Summary.HealthyRollouts != 40 {
		t.Errorf("expected 40, got %d", r.Summary.HealthyRollouts)
	}
}

func TestPVCIOHealthResultStruct1873(t *testing.T) {
	r := PVCIOHealthResult{
		Summary: PVCIOHealthSummary{
			TotalPVCs:    15,
			BoundPVCs:    12,
			OrphanedPVCs: 3,
			TotalSizeGB:  250.5,
		},
		HealthScore: 60,
	}
	if r.Summary.OrphanedPVCs != 3 {
		t.Errorf("expected 3, got %d", r.Summary.OrphanedPVCs)
	}
}

func TestIngressConflictResultStruct1873(t *testing.T) {
	r := IngressConflictResult{
		Summary: IngressConflictSummary{
			TotalIngresses: 10,
			WithTLS:        5,
			PathConflicts:  2,
			NoBackend:      1,
		},
		HealthScore: 75,
	}
	if r.Summary.PathConflicts != 2 {
		t.Errorf("expected 2, got %d", r.Summary.PathConflicts)
	}
}
