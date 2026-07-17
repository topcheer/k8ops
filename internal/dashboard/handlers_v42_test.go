package dashboard

import (
	"testing"
)

func TestBuildFatigueRecs(t *testing.T) {
	r := &AlertFatigueResult{
		Summary:      FatigueSummary{WarningEvents: 50, TotalEvents: 100, NoisyNS: 3},
		ByReason:     []FatigueReasonStat{{Reason: "FailedScheduling", Count: 15, Action: "Add resources"}},
		TopOffenders: []FatigueOffender{{Count: 10}, {Count: 8}, {Count: 6}, {Count: 5}, {Count: 4}, {Count: 3}},
	}
	recs := buildFatigueRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestFatigueAction(t *testing.T) {
	if a := fatigueAction("FailedScheduling"); a == "FailedScheduling" {
		t.Error("expected action text")
	}
	if a := fatigueAction("Unknown"); a != "Investigate event details" {
		t.Errorf("expected fallback, got %s", a)
	}
}

func TestFatigueSeverity(t *testing.T) {
	if s := fatigueSeverity("FailedScheduling", ""); s != "critical" {
		t.Errorf("expected critical, got %s", s)
	}
	if s := fatigueSeverity("BackOff", "container failed"); s != "high" {
		t.Errorf("expected high, got %s", s)
	}
	if s := fatigueSeverity("SomeWarning", "rate limited"); s != "medium" {
		t.Errorf("expected medium, got %s", s)
	}
}

func TestBuildDeployFreqRecs(t *testing.T) {
	r := &DeployFrequencyResult{
		Summary:     DeployFreqSummary{RolloutRate: 1.5, RecentlyUpdated7d: 10, OldReplicaSets: 25},
		ByNamespace: []DeployFreqNS{{Namespace: "prod", Count: 5, Updated24h: 3}},
	}
	recs := buildDeployFreqRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestPlatformScoreStatus(t *testing.T) {
	if platformScoreStatus(90) != "healthy" {
		t.Error("expected healthy")
	}
	if platformScoreStatus(65) != "warning" {
		t.Error("expected warning")
	}
	if platformScoreStatus(45) != "at-risk" {
		t.Error("expected at-risk")
	}
	if platformScoreStatus(20) != "critical" {
		t.Error("expected critical")
	}
}

func TestPlatformGradeFromScore(t *testing.T) {
	if platformGradeFromScore(85) != "A" {
		t.Error("expected A")
	}
	if platformGradeFromScore(65) != "B" {
		t.Error("expected B")
	}
	if platformGradeFromScore(45) != "C" {
		t.Error("expected C")
	}
	if platformGradeFromScore(25) != "D" {
		t.Error("expected D")
	}
}
