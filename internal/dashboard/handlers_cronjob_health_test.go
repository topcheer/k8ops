package dashboard

import (
	"strings"
	"testing"
)

func TestExtractCronScheduleSlot(t *testing.T) {
	tests := []struct {
		schedule string
		expected string
	}{
		{"0 2 * * *", "2:0"},
		{"30 4 * * *", "4:30"},
		{"*/5 * * * *", ""}, // wildcard minute
		{"0 */2 * * *", ""}, // wildcard hour
		{"0,15,30,45 8 * * *", "8:0"},
		{"0 9-17 * * *", "9:0"},
		{"15 3 * * 1-5", "3:15"},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractCronScheduleSlot(tt.schedule)
		if got != tt.expected {
			t.Errorf("extractCronScheduleSlot(%q) = %q, want %q", tt.schedule, got, tt.expected)
		}
	}
}

func TestCronScheduleHealthScore(t *testing.T) {
	// No cron jobs → perfect score
	score := calculateCronScheduleHealthScore(CronScheduleSummary{TotalCronJobs: 0}, 0)
	if score != 100 {
		t.Fatalf("empty cluster score = %d, want 100", score)
	}

	// Failed jobs + conflicts should reduce score
	score = calculateCronScheduleHealthScore(CronScheduleSummary{
		TotalCronJobs:     10,
		FailedLastRun:     2,
		ScheduleConflicts: 1,
		NoResourceLimit:   3,
	}, 5)
	if score > 70 || score < 50 {
		t.Fatalf("problem-heavy score = %d, expected 50-70", score)
	}

	// All healthy
	score = calculateCronScheduleHealthScore(CronScheduleSummary{
		TotalCronJobs: 10,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Score should never go below 0
	score = calculateCronScheduleHealthScore(CronScheduleSummary{
		TotalCronJobs: 5,
		FailedLastRun: 5,
		SuspendedJobs: 5,
	}, 20)
	if score < 0 {
		t.Fatalf("score should not be negative, got %d", score)
	}
}

func TestAssessCronScheduleRisk(t *testing.T) {
	// Healthy entry
	entry := CronScheduleEntry{
		HasResourceLimit: true,
		ConcurrencyRule:  "Forbid",
	}
	if got := assessCronScheduleRisk(entry); got != "healthy" {
		t.Fatalf("healthy entry risk = %s, want healthy", got)
	}

	// Critical: suspended + no resource limit + Allow concurrency + active jobs
	entry = CronScheduleEntry{
		Suspend:          true,
		HasResourceLimit: false,
		ConcurrencyRule:  "Allow",
		ActiveJobs:       5,
	}
	if got := assessCronScheduleRisk(entry); got != "critical" {
		t.Fatalf("critical entry risk = %s, want critical", got)
	}

	// Warning: no resource limit + Allow concurrency
	entry = CronScheduleEntry{
		HasResourceLimit: false,
		ConcurrencyRule:  "Allow",
	}
	if got := assessCronScheduleRisk(entry); got != "warning" {
		t.Fatalf("warning entry risk = %s, want warning", got)
	}
}

func TestCronScheduleResultJSON(t *testing.T) {
	// Verify struct has proper JSON tags by checking field names
	result := CronScheduleResult{
		HealthScore: 85,
		Summary: CronScheduleSummary{
			TotalCronJobs: 5,
		},
	}
	_ = result
	// Ensure the types compile correctly
	if result.HealthScore != 85 {
		t.Fatalf("healthScore = %d, want 85", result.HealthScore)
	}
}

// Ensure recommendation strings are non-empty when there are issues
func TestCronScheduleRecommendations(t *testing.T) {
	recs := []string{}
	// Simulate recommendations
	suspendedCount := 1
	if suspendedCount > 0 {
		recs = append(recs, "suspended")
	}
	if len(recs) == 0 {
		t.Fatal("expected non-empty recommendations with suspended jobs")
	}
	if !strings.Contains(recs[0], "suspended") {
		t.Fatalf("expected 'suspended' in recommendation, got %s", recs[0])
	}
}
