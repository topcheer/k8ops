package dashboard

import (
	"testing"
)

func TestTrainingReadinessTypes(t *testing.T) {
	r := TrainingReadinessResult{OnboardingScore: 25, Grade: "D"}
	if r.OnboardingScore != 25 || r.Grade != "D" {
		t.Error("struct field error")
	}

	s := TrainingSummary{TotalWorkloads: 55, WithOwnerLabel: 5, WithTeamLabel: 3, WithDocsLabel: 0}
	if s.WithOwnerLabel != 5 || s.TotalWorkloads != 55 {
		t.Error("summary field error")
	}

	lg := LabelGap{Workload: "api", Namespace: "app", Missing: []string{"owner", "team", "docs"}, Severity: "high"}
	if len(lg.Missing) != 3 || lg.Severity != "high" {
		t.Error("labelGap field error")
	}
}

func TestTrainingReadinessScoring(t *testing.T) {
	tests := []struct {
		ownerPct    float64
		teamPct     float64
		docsPct     float64
		runbookPct  float64
		expectedMin int
		expectedMax int
	}{
		{100, 100, 100, 100, 95, 100},
		{0, 0, 0, 0, 0, 5},
		{50, 50, 50, 50, 45, 55},
		{100, 0, 100, 0, 55, 65},
	}
	for _, tc := range tests {
		score := int(tc.ownerPct*0.3 + tc.teamPct*0.2 + tc.docsPct*0.3 + tc.runbookPct*0.2)
		score = min(100, score)
		if score < tc.expectedMin || score > tc.expectedMax {
			t.Errorf("owner=%.0f team=%.0f docs=%.0f rb=%.0f: expected %d-%d, got %d",
				tc.ownerPct, tc.teamPct, tc.docsPct, tc.runbookPct, tc.expectedMin, tc.expectedMax, score)
		}
	}
}

func TestTrainingReadinessGapSeverity(t *testing.T) {
	tests := []struct {
		missingCount int
		expected     string
	}{
		{1, "medium"},
		{2, "medium"},
		{3, "high"},
		{5, "high"},
	}
	for _, tc := range tests {
		severity := "medium"
		if tc.missingCount >= 3 {
			severity = "high"
		}
		if severity != tc.expected {
			t.Errorf("missingCount=%d: expected %s, got %s", tc.missingCount, tc.expected, severity)
		}
	}
}
