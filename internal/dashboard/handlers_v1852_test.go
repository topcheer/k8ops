package dashboard

import (
	"testing"
)

func TestSLOBurnRecs(t *testing.T) {
	r := &SLOBurnRateResult{
		SLOTarget:   99.9,
		ErrorBudget: 0.1,
		Summary: SLOBurnSummary{
			TotalWorkloads:  20,
			FastBurnCount:   3,
			SlowBurnCount:   5,
			BudgetExhausted: 2,
			BudgetRemaining: 45.0,
			HealthySLO:      12,
		},
	}
	recs := buildSLOBurnRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestSurgeCapacityRecs(t *testing.T) {
	r := &SurgeCapacityResult{
		Summary: SurgeCapacitySummary{
			TotalWorkloads:  30,
			CanSurge:        25,
			InsufficientCPU: 3,
			InsufficientMem: 2,
			AvailableCPU:    4.0,
			AvailableMemGB:  16.0,
			NoSurge:         10,
		},
		SurgeScore: 83,
	}
	recs := buildSurgeCapacityRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(recs))
	}
}

func TestRunbookCoverageRecs(t *testing.T) {
	r := &RunbookCoverageResult{
		Summary: RunbookSummary{
			TotalWorkloads:  40,
			WithRunbook:     12,
			WithoutRunbook:  28,
			CriticalMissing: 5,
		},
		CoverageScore: 30,
		Undocumented: []RunbookEntry{
			{Workload: "critical-svc", Namespace: "prod", Priority: "critical"},
		},
	}
	recs := buildRunbookRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestSLOBurnTypes(t *testing.T) {
	entry := SLOBurnEntry{
		Workload:        "api-server",
		Namespace:       "prod",
		ErrorRate:       0.15,
		FastBurnRate:    21.6,
		SlowBurnRate:    1.5,
		BudgetRemaining: 30.0,
		Status:          "critical-fast",
		ETAHours:        2.5,
	}
	if entry.Status != "critical-fast" {
		t.Error("status should be critical-fast")
	}
}

func TestSurgeCapacityTypes(t *testing.T) {
	entry := SurgeEntry{
		Workload:    "web-app",
		Namespace:   "default",
		Kind:        "Deployment",
		Replicas:    3,
		MaxSurge:    1,
		PerPodCPU:   0.5,
		PerPodMemGB: 1.0,
		SurgeCPU:    0.5,
		SurgeMemGB:  1.0,
		CanDeploy:   true,
	}
	if entry.SurgeCPU != entry.PerPodCPU*float64(entry.MaxSurge) {
		t.Error("surgeCPU should equal perPodCPU * maxSurge")
	}
}

func TestRunbookTypes(t *testing.T) {
	entry := RunbookEntry{
		Workload:      "db-proxy",
		Namespace:     "prod",
		Kind:          "StatefulSet",
		HasRunbook:    true,
		RunbookURL:    "https://wiki.example.com/runbooks/db-proxy",
		RunbookSource: "runbook",
		Priority:      "critical",
	}
	if !entry.HasRunbook || entry.RunbookURL == "" {
		t.Error("runbook entry should be properly populated")
	}
}
