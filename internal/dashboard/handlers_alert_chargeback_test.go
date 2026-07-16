package dashboard

import (
	"testing"
)

func TestAlertRuleQualityTypes(t *testing.T) {
	r := AlertRuleResult{QualityScore: 45, Grade: "D"}
	if r.QualityScore != 45 || r.Grade != "D" {
		t.Error("struct field error")
	}
	s := AlertRuleSummary{TotalWorkloads: 55, HasPrometheus: false, TotalRules: 0, WorkloadsWithAlerts: 0}
	if s.TotalWorkloads != 55 {
		t.Error("summary error")
	}
	ag := AlertGap{Workload: "api", Severity: "high"}
	if ag.Severity != "high" {
		t.Error("alertGap error")
	}
	nr := NoiseRisk{Source: "rules", RiskType: "explosion"}
	if nr.RiskType != "explosion" {
		t.Error("noiseRisk error")
	}
}

func TestAlertRuleScoring(t *testing.T) {
	tests := []struct {
		hasProm  bool
		hasAM    bool
		hasGraf  bool
		withAlerts int
		total     int
		expectedMin int
		expectedMax int
	}{
		{true, true, true, 50, 55, 90, 100},
		{false, false, false, 0, 55, 0, 5},
		{true, false, true, 20, 55, 40, 60},
	}
	for _, tc := range tests {
		score := 0
		if tc.hasProm {
			score += 30
		}
		if tc.hasAM {
			score += 25
		}
		if tc.hasGraf {
			score += 15
		}
		if tc.total > 0 {
			score += tc.withAlerts * 30 / tc.total
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("prom=%v am=%v graf=%v alerts=%d/%d: expected %d-%d, got %d",
				tc.hasProm, tc.hasAM, tc.hasGraf, tc.withAlerts, tc.total, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestChargebackTypes(t *testing.T) {
	r := ChargebackResult{QualityScore: 65, Grade: "D"}
	if r.QualityScore != 65 {
		t.Error("struct error")
	}
	s := ChargebackSummary{TotalMonthlyCost: 150.5, NamespaceCount: 28}
	if s.TotalMonthlyCost != 150.5 || s.NamespaceCount != 28 {
		t.Error("summary error")
	}
	nc := NamespaceCost{Namespace: "app", PodCount: 5, MonthlyCost: 45.2}
	if nc.MonthlyCost != 45.2 {
		t.Error("nsCost error")
	}
	sc := SharedCost{Category: "node-base", MonthlyCost: 50.0}
	if sc.MonthlyCost != 50 {
		t.Error("sharedCost error")
	}
}

func TestChargebackScoring(t *testing.T) {
	tests := []struct {
		waste     float64
		totalCost float64
		nsCount   int
		expectedMin int
		expectedMax int
	}{
		{0, 100, 15, 75, 100},
		{100, 600, 5, 40, 60},
		{10, 200, 3, 65, 80},
	}
	for _, tc := range tests {
		score := 70
		if tc.waste > 50 {
			score -= 20
		}
		if tc.totalCost > 500 {
			score -= 10
		}
		if tc.nsCount > 10 {
			score += 10
		}
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("waste=%.0f cost=%.0f ns=%d: expected %d-%d, got %d",
				tc.waste, tc.totalCost, tc.nsCount, tc.expectedMin, tc.expectedMax, score)
		}
	}
}
