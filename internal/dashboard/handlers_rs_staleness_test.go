package dashboard

import (
	"testing"
)

func TestAssessRSRisk(t *testing.T) {
	// Healthy deployment
	entry := RSDeploymentEntry{
		RevisionHistoryLimit: 10,
		StaleRSCount:         3,
		ReplicaSetCount:      5,
	}
	if got := assessRSRisk(entry); got != "healthy" {
		t.Errorf("healthy risk = %s, want healthy", got)
	}

	// Critical: low revision + many stale + excess
	entry = RSDeploymentEntry{
		RevisionHistoryLimit: 1,
		StaleRSCount:         15,
		ReplicaSetCount:      20,
	}
	if got := assessRSRisk(entry); got != "critical" {
		t.Errorf("critical risk = %s, want critical", got)
	}

	// Warning: low revision
	entry = RSDeploymentEntry{
		RevisionHistoryLimit: 1,
		StaleRSCount:         2,
		ReplicaSetCount:      3,
	}
	if got := assessRSRisk(entry); got != "warning" {
		t.Errorf("low-revision risk = %s, want warning", got)
	}
}

func TestComputeRSStalenessScore(t *testing.T) {
	// No deployments → perfect
	score := computeRSStalenessScore(RSSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// Problem-heavy
	score = computeRSStalenessScore(RSSummary{
		TotalDeployments:    20,
		LowRevisionHistory:  3,
		HighRevisionHistory: 2,
		NoRevisionHistory:   5,
		StaleReplicaSets:    60,
	}, 8)
	if score > 60 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-60", score)
	}

	// All healthy
	score = computeRSStalenessScore(RSSummary{
		TotalDeployments:  10,
		ActiveReplicaSets: 10,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}
}
