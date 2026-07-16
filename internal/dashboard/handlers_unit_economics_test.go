package dashboard

import (
	"testing"
)

func TestComputeUnitEconScore(t *testing.T) {
	// Good ratios → high score
	s0 := UnitEconSummary{TotalPods: 10, TotalCPURequests: 5}
	r0 := EfficiencyRatios{LimitToRequestCPU: 1.5, LimitToRequestMem: 1.5}
	if score := computeUnitEconScore(s0, r0); score < 90 {
		t.Errorf("expected score >= 90 for good ratios, got %d", score)
	}

	// High limit-to-request → lower score
	s1 := UnitEconSummary{TotalPods: 10, TotalCPURequests: 5}
	r1 := EfficiencyRatios{LimitToRequestCPU: 5, LimitToRequestMem: 5}
	if score := computeUnitEconScore(s1, r1); score > 80 {
		t.Errorf("expected score <= 80 for high ratios, got %d", score)
	}

	// No requests at all → penalized
	s2 := UnitEconSummary{TotalPods: 10, TotalCPURequests: 0}
	if score := computeUnitEconScore(s2, EfficiencyRatios{}); score > 80 {
		t.Errorf("expected score <= 80 for no requests, got %d", score)
	}
}

func TestGenerateUnitSavings(t *testing.T) {
	// High ratios → savings opportunities
	s := UnitEconSummary{MonthlySpend: 1000, TotalPods: 20}
	r := EfficiencyRatios{LimitToRequestCPU: 5, LimitToRequestMem: 4}
	savings := generateUnitSavings(s, r, nil)
	if len(savings) < 2 {
		t.Errorf("expected at least 2 savings opportunities, got %d", len(savings))
	}

	// Good ratios → no savings
	r2 := EfficiencyRatios{LimitToRequestCPU: 1.5, LimitToRequestMem: 1.5}
	savings2 := generateUnitSavings(s, r2, nil)
	if len(savings2) > 0 {
		t.Errorf("expected 0 savings for good ratios, got %d", len(savings2))
	}

	// Check savings are sorted by annual
	for i := 1; i < len(savings); i++ {
		if savings[i].AnnualSavings > savings[i-1].AnnualSavings {
			t.Error("savings not sorted by annual amount")
		}
	}
}
