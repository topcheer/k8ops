package dashboard

import "testing"

func TestWLDepsTypes(t *testing.T) {
	r := WLDepsResult{HealthScore: 85, Grade: "B"}
	if r.HealthScore != 85 { t.Error("struct error") }
	s := WLDepsSummary{TotalWorkloads: 55, WithInitContainers: 10, StartupOrderRisks: 5}
	if s.WithInitContainers != 10 { t.Error("summary error") }
	ri := WLDepRisk{Workload: "api", RiskType: "startup-order", Severity: "medium"}
	if ri.RiskType != "startup-order" { t.Error("risk error") }
}

func TestMetricsPipeTypes(t *testing.T) {
	r := MetricsPipeResult{HealthScore: 0, Grade: "F"}
	if r.HealthScore != 0 { t.Error("struct error") }
	s := MetricsPipeSummary{HasPrometheus: false, ExportersFound: 0, BlindWorkloads: 55}
	if s.BlindWorkloads != 55 { t.Error("summary error") }
	g := MetricsGap{Category: "backend", Severity: "critical"}
	if g.Severity != "critical" { t.Error("gap error") }
}

func TestMetricsPipeScoring(t *testing.T) {
	tests := []struct{ prom bool; exporters, targets, blind int; expMin, expMax int }{
		{true, 2, 10, 0, 90, 100},
		{false, 0, 0, 55, 0, 10},
	}
	for _, tc := range tests {
		score := 0
		if tc.prom { score += 35 }
		if tc.exporters >= 2 { score += 25 }
		if tc.exporters >= 1 { score += 10 }
		if tc.targets > 0 { score += 15 }
		if tc.blind < 30 { score += 15 }
		score = min(100, score)
		if score < tc.expMin || score > tc.expMax {
			t.Errorf("prom=%v exp=%d tgt=%d blind=%d: expected %d-%d, got %d", tc.prom, tc.exporters, tc.targets, tc.blind, tc.expMin, tc.expMax, score)
		}
	}
}

func TestChangeLogTypes(t *testing.T) {
	r := ChangeLogResult{HealthScore: 65, Grade: "D"}
	if r.HealthScore != 65 { t.Error("struct error") }
	s := ChangeLogSummary{TotalChanges24h: 10, NewWorkloads: 3, UpdatedWorkloads: 7}
	if s.NewWorkloads != 3 { t.Error("summary error") }
	ce := ChangeEntry{Kind: "Deployment", Name: "api", Action: "created"}
	if ce.Action != "created" { t.Error("entry error") }
}
