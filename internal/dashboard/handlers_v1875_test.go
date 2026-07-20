package dashboard

import (
	"testing"
)

func TestPodAffinitySpreadResultStruct1875(t *testing.T) {
	r := PodAffinitySpreadResult{
		Summary: AffinitySpreadSummary{
			TotalWorkloads: 49,
			MultiReplica:   2,
			SinglePoint:    47,
		},
		HealthScore: 4,
	}
	if r.Summary.SinglePoint != 47 {
		t.Errorf("expected 47, got %d", r.Summary.SinglePoint)
	}
}

func TestNamespaceBudgetEnforceResultStruct1875(t *testing.T) {
	r := NamespaceBudgetEnforceResult{
		Summary: BudgetEnforceSummary{
			TotalNamespaces: 29,
			WithQuota:       0,
			WithoutQuota:    29,
		},
		HealthScore: 0,
	}
	if r.Summary.WithoutQuota != 29 {
		t.Errorf("expected 29, got %d", r.Summary.WithoutQuota)
	}
}

func TestResourceWasteDeepResultStruct1875(t *testing.T) {
	r := ResourceWasteDeepResult{
		Summary: ResourceWasteSummary{
			TotalWorkloads: 49,
			OverProvCount:  10,
			ZombieCount:    5,
		},
		PotentialSavings: 250.50,
	}
	if r.PotentialSavings != 250.50 {
		t.Errorf("expected 250.50, got %.2f", r.PotentialSavings)
	}
}
