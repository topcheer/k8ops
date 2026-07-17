package dashboard

import (
	"testing"
	"time"
)

func TestComputeFreezeScore(t *testing.T) {
	// Clean state
	s0 := FreezeSummary{}
	if score := computeFreezeScore(s0); score < 85 {
		t.Errorf("expected high score for clean state, got %d", score)
	}

	// With crash loops
	s1 := FreezeSummary{CrashLoopPods: 3}
	if score := computeFreezeScore(s1); score > 60 {
		t.Errorf("expected lower score with crash loops, got %d", score)
	}

	// With warning events
	s2 := FreezeSummary{WarningEvents1h: 15}
	if score := computeFreezeScore(s2); score > 75 {
		t.Errorf("expected lower score with warnings, got %d", score)
	}
}

func TestGenerateFreezeRecs(t *testing.T) {
	r := ChangeFreezeResult{
		FreezeStatus: "none",
		Verdict:      "proceed",
		CurrentRisk:  "low",
		Summary:      FreezeSummary{StabilityScore: 90},
	}
	recs := generateFreezeRecs(r)
	if len(recs) == 0 {
		t.Error("expected recs for clean state")
	}

	// With crash loops → freeze
	r2 := ChangeFreezeResult{
		FreezeStatus: "active",
		Verdict:      "freeze",
		Summary:      FreezeSummary{CrashLoopPods: 2, StabilityScore: 55},
	}
	recs2 := generateFreezeRecs(r2)
	found := false
	for _, r := range recs2 {
		if len(r) > 6 && r[:6] == "FREEZE" {
			found = true
		}
	}
	if !found {
		t.Error("expected FREEZE recommendation")
	}
}

func TestGenerateFreezeWindows(t *testing.T) {
	// Test during Christmas period
	christmas := parseDate("2025-12-25")
	windows := generateFreezeWindows(christmas)
	if len(windows) == 0 {
		t.Error("expected freeze windows during Christmas")
	}

	// Test during non-holiday period
	summer := parseDate("2025-07-15")
	windows2 := generateFreezeWindows(summer)
	// May or may not have windows, but should not panic
	_ = windows2
}

func parseDate(s string) (t time.Time) {
	t, _ = time.Parse("2006-01-02", s)
	return
}
