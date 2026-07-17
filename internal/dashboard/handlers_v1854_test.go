package dashboard

import (
	"testing"
)

func TestGoldenSignalRecs(t *testing.T) {
	r := &GoldenSignalBudgetResult{
		Summary: GoldenSignalSummary{
			TotalWorkloads:    30,
			LatencyHealthy:    20,
			TrafficHealthy:    25,
			ErrorsHealthy:     18,
			SaturationHealthy: 22,
			CriticalWorkloads: 5,
			AvgBudget:         55.5,
		},
		CompositeScore: 55,
		CriticalSignals: []GoldenSignalEntry{
			{Workload: "api-server", Namespace: "prod", CompositeScore: 30, TopSignal: "errors", IssueDetail: "errors signal weakest (score=20)"},
		},
	}
	recs := buildGoldenSignalRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestPreflightRecs(t *testing.T) {
	r := &PreflightCheckResult{
		Summary: PreflightSummary{
			TotalChecks:   8,
			Passed:        6,
			Failed:        2,
			Warnings:      1,
			BlockingCount: 1,
		},
		PassRate: 75,
		BlockingChecks: []PreflightCheck{
			{Name: "node-health", Message: "1 node not ready"},
		},
	}
	recs := buildPreflightRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestCapacityRunbookRecs(t *testing.T) {
	r := &CapacityRunbookResult{
		CapacityScore: 35,
		Grade:         "C",
		Summary: CapacityRunbookSummary{
			HeadroomCPU: 60.0,
			HeadroomMem: 45.0,
		},
		HeadroomAnalysis: CapacityRunbookHeadroom{
			Bottleneck:      "Memory",
			NewPodsFittable: 12,
		},
		GrowthProjection: CapacityRunbookProjection{
			ScaleUrgency:      "urgent",
			CPUExhaustionDays: 45,
			MemExhaustionDays: 20,
		},
		EmergencyRunbook: []string{"step1", "step2", "step3", "step4", "step5"},
	}
	recs := buildCapacityRunbookRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestGoldenSignalTypes(t *testing.T) {
	entry := GoldenSignalEntry{
		Workload:        "web-api",
		Namespace:       "prod",
		LatencyScore:    85,
		TrafficScore:    90,
		ErrorScore:      70,
		SaturationScore: 60,
	}
	entry.CompositeScore = (entry.LatencyScore*30 + entry.TrafficScore*20 + entry.ErrorScore*30 + entry.SaturationScore*20) / 100
	if entry.CompositeScore != 76 {
		t.Errorf("composite score should be 76, got %d", entry.CompositeScore)
	}
}

func TestPreflightTypes(t *testing.T) {
	check := PreflightCheck{
		Name:     "resource-requests",
		Category: "resources",
		Status:   "pass",
		Severity: "none",
		Message:  "All deployments have resource requests",
	}
	if check.Status != "pass" {
		t.Error("status should be pass")
	}
}

func TestCapacityRunbookTypes(t *testing.T) {
	section := CapacityRunbookSection{
		Title:    "Cluster Overview",
		Content:  "3 worker nodes, 16 CPU, 64GB RAM",
		Priority: "info",
	}
	if section.Title == "" {
		t.Error("title should not be empty")
	}

	proj := CapacityRunbookProjection{
		CurrentPods:       50,
		ProjectedPods30d:  52,
		CPUExhaustionDays: 60,
		MemExhaustionDays: 45,
		ScaleUrgency:      "normal",
	}
	if proj.ProjectedPods30d <= proj.CurrentPods {
		t.Error("projected should be larger")
	}
}

func TestGoldenClampScore(t *testing.T) {
	if goldenClampScore(-5) != 0 {
		t.Error("negative should clamp to 0")
	}
	if goldenClampScore(150) != 100 {
		t.Error("over 100 should clamp to 100")
	}
	if goldenClampScore(55) != 55 {
		t.Error("in-range should stay")
	}
}
