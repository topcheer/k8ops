package dashboard

import (
	"testing"
)

func TestComputeStability(t *testing.T) {
	// DaemonSet in prod → very stable
	score := computeStability("app", "production", "DaemonSet", 5)
	if score < 80 {
		t.Errorf("expected score >= 80 for prod DaemonSet, got %d", score)
	}

	// Deployment in dev → less stable
	score = computeStability("app", "dev", "Deployment", 1)
	if score > 70 {
		t.Errorf("expected score <= 70 for dev Deployment, got %d", score)
	}

	// StatefulSet → high stability
	score = computeStability("db", "prod", "StatefulSet", 3)
	if score < 85 {
		t.Errorf("expected score >= 85 for prod StatefulSet, got %d", score)
	}
}

func TestRecommendCommitment(t *testing.T) {
	if c := recommendCommitment(85, "Deployment"); c != "reserved-instance" {
		t.Errorf("expected 'reserved-instance' for stability 85, got %q", c)
	}
	if c := recommendCommitment(65, "Deployment"); c != "sustained-use" {
		t.Errorf("expected 'sustained-use' for stability 65, got %q", c)
	}
	if c := recommendCommitment(40, "Deployment"); c != "on-demand" {
		t.Errorf("expected 'on-demand' for stability 40, got %q", c)
	}
}

func TestComputeCommitScore(t *testing.T) {
	// Good: high stable ratio, low savings
	s := CommitSummary{StableCPUPercent: 80, SavingsPct: 10}
	if score := computeCommitScore(s); score < 85 {
		t.Errorf("expected score >= 85, got %d", score)
	}

	// Bad: low stable ratio, high savings
	s2 := CommitSummary{StableCPUPercent: 30, SavingsPct: 60}
	if score := computeCommitScore(s2); score > 60 {
		t.Errorf("expected score <= 60, got %d", score)
	}
}

func TestGenerateCommitPlan(t *testing.T) {
	stable := []StableResource{
		{Name: "app", Namespace: "prod", CPUCores: 2, MemGB: 4, StabilityScore: 85, MonthlyCost: 70, CommitmentType: "reserved-instance"},
		{Name: "api", Namespace: "prod", CPUCores: 1, MemGB: 2, StabilityScore: 65, MonthlyCost: 35, CommitmentType: "sustained-use"},
	}
	plan := generateCommitmentPlan(stable, 3, 6)
	if len(plan) < 2 {
		t.Errorf("expected at least 2 plan items, got %d", len(plan))
	}

	// Check first item is aggregate reserved
	if plan[0].Type != "reserved-instance" {
		t.Errorf("expected first item type 'reserved-instance', got %q", plan[0].Type)
	}
}

func TestGenerateCommitRecs(t *testing.T) {
	r := CommitOptimizerResult{
		Summary: CommitSummary{
			CurrentMonthlyCost: 500,
			OptimizedMonthlyCost: 300,
			SavingsPct: 40,
			StableCPUPercent: 70,
			StableMemPercent: 65,
			TotalCPURequested: 10,
			TotalMemRequested: 20,
		},
		SavingsEstimate: SavingsBreakdown{
			RightSizeSavings: 75,
			TotalAnnualSavings: 2400,
		},
		VolatileUsage: []VolatileResource{
			{Name: "batch", Namespace: "prod"},
		},
	}
	recs := generateCommitRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recs, got %d", len(recs))
	}
}
