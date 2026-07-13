package dashboard

import (
	"testing"
)

func TestAssessHPAPerfRisk(t *testing.T) {
	// Healthy HPA
	entry := HPAPerfEntry{
		ScalingActive:  true,
		ScalingLimited: false,
		MinReplicas:    1,
		MaxReplicas:    10,
	}
	if got := assessHPAPerfRisk(entry); got != "healthy" {
		t.Errorf("healthy HPA risk = %s, want healthy", got)
	}

	// Critical: not active + scaling limited + no room
	entry = HPAPerfEntry{
		ScalingActive:  false,
		ScalingLimited: true,
		MinReplicas:    5,
		MaxReplicas:    5,
	}
	if got := assessHPAPerfRisk(entry); got != "critical" {
		t.Errorf("critical HPA risk = %s, want critical", got)
	}

	// Warning: not active
	entry = HPAPerfEntry{
		ScalingActive:  false,
		ScalingLimited: false,
		MinReplicas:    1,
		MaxReplicas:    10,
	}
	if got := assessHPAPerfRisk(entry); got != "warning" {
		t.Errorf("inactive HPA risk = %s, want warning", got)
	}
}

func TestComputeHPAPerfScore(t *testing.T) {
	// No HPAs → perfect
	score := computeHPAPerfScore(HPAPerfSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// No metrics is most critical
	score = computeHPAPerfScore(HPAPerfSummary{
		TotalHPAs:      10,
		NoMetrics:      3,
		ScalingLimited: 2,
		Overutilized:   1,
	}, 5)
	if score > 60 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-60", score)
	}

	// All healthy
	score = computeHPAPerfScore(HPAPerfSummary{
		TotalHPAs:     10,
		WithMetrics:   10,
		ScalingActive: 10,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}
}
