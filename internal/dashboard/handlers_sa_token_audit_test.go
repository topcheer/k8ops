package dashboard

import (
	"testing"
)

func TestAssessSATokenRisk(t *testing.T) {
	// Healthy SA: dedicated, automount off or used properly
	entry := SATokenEntry{
		IsDefault:      false,
		AutoMountToken: false,
		PodUsageCount:  1,
	}
	if got := assessSATokenRisk(entry); got != "healthy" {
		t.Errorf("healthy SA risk = %s, want healthy", got)
	}

	// Critical: default SA used by pods with automount
	entry = SATokenEntry{
		IsDefault:      true,
		AutoMountToken: true,
		PodUsageCount:  3,
		SecretAge:      "120d",
	}
	if got := assessSATokenRisk(entry); got != "critical" {
		t.Errorf("default-sa risk = %s, want critical", got)
	}

	// Warning: default SA used by pods
	entry = SATokenEntry{
		IsDefault:      true,
		AutoMountToken: true,
		PodUsageCount:  2,
	}
	if got := assessSATokenRisk(entry); got != "warning" {
		t.Errorf("default-sa-no-age risk = %s, want warning", got)
	}

	// Info: unused SA with automount
	entry = SATokenEntry{
		IsDefault:      false,
		AutoMountToken: true,
		PodUsageCount:  0,
	}
	if got := assessSATokenRisk(entry); got != "info" {
		t.Errorf("unused-sa risk = %s, want info", got)
	}
}

func TestComputeSATokenScore(t *testing.T) {
	// No SAs → perfect
	score := computeSATokenScore(SATokenSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// Default SA usage is most impactful
	score = computeSATokenScore(SATokenSummary{
		TotalSAs:        20,
		DefaultSAUsed:   3,
		LongLivedTokens: 2,
	}, 5)
	if score > 75 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-75", score)
	}

	// All healthy
	score = computeSATokenScore(SATokenSummary{
		TotalSAs:          20,
		AutoMountDisabled: 15,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}
}
