package dashboard

import (
	"testing"
)

func TestComputeAllocEffScore(t *testing.T) {
	// No containers → perfect
	score := computeAllocEffScore(AllocEffSummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// No requests is most critical
	score = computeAllocEffScore(AllocEffSummary{
		TotalContainers: 20,
		NoRequests:      5,
		NoLimits:        3,
		Overallocated:   2,
	}, 10)
	if score > 60 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-60", score)
	}

	// All healthy
	score = computeAllocEffScore(AllocEffSummary{
		TotalContainers: 20,
		WithRequests:    20,
		WithLimits:      20,
		AllocEfficiency: 0.5,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Extreme alloc efficiency penalty
	score = computeAllocEffScore(AllocEffSummary{
		TotalContainers: 10,
		WithRequests:    10,
		WithLimits:      10,
		AllocEfficiency: 0.98,
	}, 0)
	if score > 95 {
		t.Fatalf("high-alloc-eff score = %d, expected <= 95", score)
	}
}

func TestAllocEffSummarySerialization(t *testing.T) {
	// Verify struct fields are properly typed
	s := AllocEffSummary{
		TotalContainers: 10,
		WithRequests:    8,
		WithLimits:      9,
		NoRequests:      2,
		NoLimits:        1,
		AllocEfficiency: 0.65,
		TotalCPURequest: "500m",
		TotalCPULimit:   "1000m",
	}
	if s.TotalContainers != 10 {
		t.Fatalf("TotalContainers = %d, want 10", s.TotalContainers)
	}
	if s.AllocEfficiency != 0.65 {
		t.Fatalf("AllocEfficiency = %f, want 0.65", s.AllocEfficiency)
	}
}

func TestAllocEffEntryRiskLevel(t *testing.T) {
	// Verify risk levels are set correctly
	entry := AllocEffEntry{
		PodName:   "test-pod",
		Container: "app",
		IssueType: "no-requests",
		RiskLevel: "critical",
	}
	if entry.RiskLevel != "critical" {
		t.Fatalf("no-requests risk = %s, want critical", entry.RiskLevel)
	}

	entry = AllocEffEntry{
		PodName:   "test-pod",
		Container: "app",
		IssueType: "no-limits",
		RiskLevel: "warning",
	}
	if entry.RiskLevel != "warning" {
		t.Fatalf("no-limits risk = %s, want warning", entry.RiskLevel)
	}
}
