package dashboard

import (
	"testing"
)

func TestComputeGreenScore(t *testing.T) {
	// Clean → high score
	s0 := GreenSummary{}
	e0 := GreenEfficiency{}
	if score := computeGreenScore(s0, e0, 0); score < 80 {
		t.Errorf("expected score >= 80 for clean state, got %d", score)
	}

	// With waste
	s1 := GreenSummary{}
	e1 := GreenEfficiency{WastePercent: 30, ResourceUtilization: 20}
	if score := computeGreenScore(s1, e1, 2); score > 50 {
		t.Errorf("expected low score with waste, got %d", score)
	}
}

func TestGreenVerdict(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{85, "eco-friendly"},
		{65, "moderate"},
		{45, "wasteful"},
		{25, "critical"},
	}
	for _, tt := range tests {
		got := greenVerdict(tt.score)
		if got != tt.want {
			t.Errorf("greenVerdict(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestGenerateGreenRecs(t *testing.T) {
	// No waste → no recs
	s0 := GreenSummary{AnnualCostUSD: 100, AnnualCO2Kg: 500}
	e0 := GreenEfficiency{ResourceUtilization: 60}
	r := GreenComputingResult{Summary: s0, Efficiency: e0}
	recs := generateGreenRecs(r)
	if len(recs) > 0 {
		t.Errorf("expected 0 recs for efficient state, got %d", len(recs))
	}

	// With waste
	e1 := GreenEfficiency{WasteCPUCores: 2, ResourceUtilization: 20}
	r1 := GreenComputingResult{Summary: s0, Efficiency: e1}
	recs = generateGreenRecs(r1)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs for wasteful state, got %d", len(recs))
	}
}

func TestGenerateGreenTips(t *testing.T) {
	s := GreenSummary{EstimatedPowerKW: 1.5, PUEstimate: 1.55, AnnualEnergyKWh: 13140, AnnualCostUSD: 1576, AnnualCO2Kg: 6240}
	e := GreenEfficiency{WastePercent: 15, ResourceUtilization: 35}
	tips := generateGreenTips(s, e)
	if len(tips) < 3 {
		t.Errorf("expected at least 3 tips, got %d", len(tips))
	}
}
