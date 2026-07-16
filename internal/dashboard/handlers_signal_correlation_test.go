package dashboard

import (
	"testing"
)

func TestCountHighRisk(t *testing.T) {
	corrs := []CorrelatedSignal{
		{RiskLevel: "critical"},
		{RiskLevel: "high"},
		{RiskLevel: "medium"},
		{RiskLevel: "low"},
	}
	if count := countHighRisk(corrs); count != 2 {
		t.Errorf("expected 2 high-risk, got %d", count)
	}
}

func TestComputeCorrelationScore(t *testing.T) {
	// Clean → perfect
	s0 := CorrelationSummary{}
	if score := computeCorrelationScore(s0, 100); score != 100 {
		t.Errorf("expected 100, got %d", score)
	}

	// With high-risk correlations
	s1 := CorrelationSummary{HighRiskCorrelations: 3, EmergingRisks: 2, HotspotsIdentified: 3}
	if score := computeCorrelationScore(s1, 100); score > 55 {
		t.Errorf("expected low score with issues, got %d", score)
	}
}

func TestBuildSignalMatrix(t *testing.T) {
	nsSignals := map[string]map[string]int{
		"default": {"crashloop": 2, "restart-spike": 1},
		"prod":    {"pending-pods": 3},
	}
	nodeSignals := map[string]map[string]int{
		"node-1": {"disk-pressure": 1},
	}
	matrix := buildSignalMatrix(nsSignals, nodeSignals)
	if len(matrix) < 5 {
		t.Errorf("expected at least 5 matrix entries, got %d", len(matrix))
	}

	// Check crashloop entry
	found := false
	for _, m := range matrix {
		if m.Source == "crashloop" && m.AnomalyCount == 2 {
			found = true
			break
		}
	}
	if !found {
		t.Error("crashloop signal with count 2 not found in matrix")
	}
}

func TestGenerateCorrelationRecs(t *testing.T) {
	// Empty correlations
	r0 := SignalCorrelationResult{
		Summary: CorrelationSummary{},
		HealthScore: 100,
	}
	recs := generateCorrelationRecs(r0)
	if len(recs) == 0 {
		t.Error("expected at least 1 rec for clean state")
	}

	// With critical correlation
	r1 := SignalCorrelationResult{
		Summary: CorrelationSummary{CorrelationsFound: 1, HighRiskCorrelations: 1},
		Correlations: []CorrelatedSignal{
			{RiskLevel: "critical", ScopeName: "default", Description: "test", ETA: "<1h", Confidence: 90},
		},
		HealthScore: 60,
	}
	recs = generateCorrelationRecs(r1)
	if len(recs) < 2 {
		t.Errorf("expected multiple recs, got %d", len(recs))
	}
}
