package dashboard

import (
	"testing"
)

func TestAssessPDRisk(t *testing.T) {
	// Healthy deployment
	entry := PDDeploymentEntry{
		Strategy:            "Rolling",
		HasProgressDeadline: true,
		Stalled:             false,
	}
	if got := assessPDRisk(entry); got != "healthy" {
		t.Errorf("healthy entry risk = %s, want healthy", got)
	}

	// Critical: stalled + recreate with replicas + no progress deadline
	entry = PDDeploymentEntry{
		Strategy:            "Recreate",
		Replicas:            3,
		HasProgressDeadline: false,
		Stalled:             true,
	}
	if got := assessPDRisk(entry); got != "critical" {
		t.Errorf("critical entry risk = %s, want critical", got)
	}

	// Warning: recreate with replicas
	entry = PDDeploymentEntry{
		Strategy:            "Recreate",
		Replicas:            3,
		Stalled:             false,
		HasProgressDeadline: true,
	}
	if got := assessPDRisk(entry); got != "warning" {
		t.Errorf("warning entry risk = %s, want warning", got)
	}
}

func TestComputePDHealthScore(t *testing.T) {
	// No deployments → perfect
	score := computePDHealthScore(PDSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// Stalled rollouts + recreate
	score = computePDHealthScore(PDSummary{
		TotalDeployments:   20,
		StalledRollouts:    3,
		RecreateStrategy:   2,
		NoProgressDeadline: 5,
	}, 8)
	if score > 55 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-55", score)
	}

	// All healthy
	score = computePDHealthScore(PDSummary{
		TotalDeployments: 10,
		RollingStrategy:  10,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}
}
