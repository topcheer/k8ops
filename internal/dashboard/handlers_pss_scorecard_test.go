package dashboard

import (
	"testing"
)

func TestAssessPSSRisk(t *testing.T) {
	// Healthy entry with no violations
	entry := PSSContainerEntry{Violations: []string{}}
	if got := assessPSSRisk(entry); got != "healthy" {
		t.Errorf("no-violations risk = %s, want healthy", got)
	}

	// Privileged is always critical
	entry = PSSContainerEntry{
		IsPrivileged: true,
		Violations:   []string{"privileged"},
	}
	if got := assessPSSRisk(entry); got != "critical" {
		t.Errorf("privileged risk = %s, want critical", got)
	}

	// 1 violation = info
	entry = PSSContainerEntry{
		Violations: []string{"runAsNonRoot not set"},
	}
	if got := assessPSSRisk(entry); got != "info" {
		t.Errorf("1-violation risk = %s, want info", got)
	}

	// 2 violations = warning
	entry = PSSContainerEntry{
		Violations: []string{"a", "b"},
	}
	if got := assessPSSRisk(entry); got != "warning" {
		t.Errorf("2-violation risk = %s, want warning", got)
	}

	// 4+ violations = critical
	entry = PSSContainerEntry{
		Violations: []string{"a", "b", "c", "d"},
	}
	if got := assessPSSRisk(entry); got != "critical" {
		t.Errorf("4-violation risk = %s, want critical", got)
	}
}

func TestComputePSSScore(t *testing.T) {
	// No containers → perfect
	score := computePSSScore(PSSSummary{})
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// All compliant with high ratio
	score = computePSSScore(PSSSummary{
		TotalContainers:     20,
		RestrictedCompliant: 19,
	})
	if score < 100 {
		t.Fatalf("high-compliance score = %d, want 100 (bonus)", score)
	}

	// Privileged containers have highest impact
	score = computePSSScore(PSSSummary{
		TotalContainers: 20,
		Privileged:      3,
		HostNetwork:     1,
	})
	if score > 55 || score < 0 {
		t.Fatalf("privileged-heavy score = %d, expected 0-55", score)
	}

	// Missing security settings
	score = computePSSScore(PSSSummary{
		TotalContainers:  10,
		NoRunAsNonRoot:   5,
		NoSeccompProfile: 3,
		AllowPrivEscal:   2,
		NoCapDropAll:     2,
		NoReadOnlyRootFS: 2,
	})
	if score > 75 || score < 0 {
		t.Fatalf("missing-settings score = %d, expected 0-75", score)
	}
}
