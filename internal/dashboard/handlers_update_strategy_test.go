package dashboard

import (
	"testing"
)

func TestUSAssessRisk(t *testing.T) {
	// Recreate = critical
	entry := USEntry{Strategy: "Recreate"}
	if level := usAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for Recreate, got %s", level)
	}

	// maxUnavailable=100% = high
	entry = USEntry{Strategy: "RollingUpdate", Violations: []string{"maxUnavailable=100% — all pods can be down"}}
	if level := usAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for maxUnavailable=100%%, got %s", level)
	}

	// Multiple violations = medium
	entry = USEntry{Strategy: "RollingUpdate", Violations: []string{"violation1", "violation2"}}
	if level := usAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for 2 violations, got %s", level)
	}

	// Single violation = medium
	entry = USEntry{Strategy: "RollingUpdate", Violations: []string{"one violation"}}
	if level := usAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for 1 violation, got %s", level)
	}

	// Clean = low
	entry = USEntry{Strategy: "RollingUpdate"}
	if level := usAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestUSScore(t *testing.T) {
	// Empty
	if score := usScore(USSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := USSummary{TotalWorkloads: 10, RollingUpdate: 10}
	if score := usScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = USSummary{
		TotalWorkloads:     10,
		Recreate:           2, // -30
		MaxUnavailable100:  1, // -10
		MaxSurgeZero:       2, // -10
		RevHistoryLow:      3, // -12
		NoProgressDeadline: 2, // -6
	}
	// 100 - 30 - 10 - 10 - 12 - 6 = 32
	if score := usScore(s); score != 32 {
		t.Errorf("Expected 32, got %d", score)
	}
}

func TestUSGenRecs(t *testing.T) {
	s := USSummary{
		TotalWorkloads:     10,
		Recreate:           2,
		MaxUnavailable100:  1,
		MaxSurgeZero:       2,
		RevHistoryLow:      3,
		NoProgressDeadline: 2,
		RevHistoryHigh:     1,
		ReadinessScore:     30,
	}
	recreate := []USEntry{{Namespace: "default", Name: "api"}}

	recs := usGenRecs(s, recreate, nil)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundRecreate := false
	foundMaxUnavailable := false
	foundRevHistory := false
	foundProgressDeadline := false
	for _, r := range recs {
		if strContains(r, "Recreate strategy") {
			foundRecreate = true
		}
		if strContains(r, "maxUnavailable") {
			foundMaxUnavailable = true
		}
		if strContains(r, "revisionHistoryLimit") {
			foundRevHistory = true
		}
		if strContains(r, "progressDeadlineSeconds") {
			foundProgressDeadline = true
		}
	}
	if !foundRecreate {
		t.Error("Expected recommendation about Recreate strategy")
	}
	if !foundMaxUnavailable {
		t.Error("Expected recommendation about maxUnavailable")
	}
	if !foundRevHistory {
		t.Error("Expected recommendation about revisionHistoryLimit")
	}
	if !foundProgressDeadline {
		t.Error("Expected recommendation about progressDeadlineSeconds")
	}
}

func TestUSGenRecsClean(t *testing.T) {
	s := USSummary{TotalWorkloads: 10, RollingUpdate: 10}
	recs := usGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestUSRiskRank(t *testing.T) {
	if usRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if usRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if usRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if usRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestUSIssueRank(t *testing.T) {
	if usIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if usIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if usIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
