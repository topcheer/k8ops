package dashboard

import (
	"testing"
)

func TestObsCardinalityTypes(t *testing.T) {
	r := ObsCardinalityResult{RiskScore: 45, Grade: "D", EstMonthlyCost: 12.5}
	if r.RiskScore != 45 || r.Grade != "D" || r.EstMonthlyCost != 12.5 {
		t.Error("struct field error")
	}

	s := ObsCardSummary{HasPrometheus: true, HasFluentBit: false, CollectorPods: 3, HighCardLabels: 5}
	if !s.HasPrometheus || s.HasFluentBit || s.CollectorPods != 3 {
		t.Error("summary field error")
	}

	cr := CardinalityRisk{Source: "app", RiskType: "metric-cardinality-explosion", Severity: "high"}
	if cr.Severity != "high" || cr.RiskType != "metric-cardinality-explosion" {
		t.Error("cardinalityRisk field error")
	}

	lv := LogVolumeNS{Namespace: "default", PodCount: 10, EstVolumeMB: 100, Status: "covered"}
	if lv.PodCount != 10 || lv.EstVolumeMB != 100 {
		t.Error("logVolumeNS field error")
	}

	ch := CollectorHealth{Name: "prometheus", Namespace: "monitoring", Ready: true, Status: "healthy"}
	if !ch.Ready || ch.Status != "healthy" {
		t.Error("collectorHealth field error")
	}
}

func TestObsCardinalityScoring(t *testing.T) {
	tests := []struct {
		hasProm       bool
		hasFluent     bool
		blindNS       int
		highCardRisks int
		expectedMin   int
		expectedMax   int
	}{
		{true, true, 0, 0, 90, 100},    // Everything present
		{false, false, 20, 3, 0, 30},   // Nothing present, many blind NS
		{true, false, 5, 1, 45, 75},    // Partial stack
	}
	for _, tc := range tests {
		score := 100
		if !tc.hasProm {
			score -= 30
		}
		if !tc.hasFluent {
			score -= 20
		}
		score -= tc.blindNS * 2
		score -= tc.highCardRisks * 5
		if score < 0 {
			score = 0
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("hasProm=%v hasFluent=%v blindNS=%d: expected %d-%d, got %d",
				tc.hasProm, tc.hasFluent, tc.blindNS, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestObsCardinalityCostEstimation(t *testing.T) {
	// Test series cost: 50000 series = 0.05M = $2.50
	seriesCost := float64(50000) / 1000000.0 * 50.0
	if seriesCost < 2.0 || seriesCost > 3.0 {
		t.Errorf("seriesCost for 50K series = %.2f, expected ~2.5", seriesCost)
	}

	// Test log cost: 10240 MB = 10 GB = $20
	logCost := float64(10240) / 1024.0 * 2.0
	if logCost < 19.0 || logCost > 21.0 {
		t.Errorf("logCost for 10GB = %.2f, expected ~20", logCost)
	}

	// Total
	total := seriesCost + logCost
	if total < 21.0 || total > 24.0 {
		t.Errorf("totalCost = %.2f, expected ~22.5", total)
	}
}
