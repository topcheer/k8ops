package dashboard

import (
	"testing"
)

func TestRIAssessRisk(t *testing.T) {
	// Missing + not optional = critical
	entry := RIEntry{Exists: false, Optional: false}
	if level := riAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	// Missing + optional = low
	entry = RIEntry{Exists: false, Optional: true}
	if level := riAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}

	// Exists = low
	entry = RIEntry{Exists: true}
	if level := riAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestRIScore(t *testing.T) {
	if score := riScore(RISummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := RISummary{TotalRefs: 20, BrokenRefs: 3}
	// 100 - 45 = 55
	if score := riScore(s); score != 55 {
		t.Errorf("Expected 55, got %d", score)
	}

	// Floor at 0
	s = RISummary{TotalRefs: 10, BrokenRefs: 10}
	if score := riScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestRIGenRecs(t *testing.T) {
	s := RISummary{
		TotalRefs:          20,
		BrokenRefs:         3,
		OptionalRefs:       2,
		WorkloadsWithIssue: 2,
	}
	broken := []RIEntry{{Namespace: "app", Workload: "api", RefType: "Secret", RefName: "db-cred"}}

	recs := riGenRecs(s, broken)

	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}

	foundBroken := false
	foundOptional := false
	for _, r := range recs {
		if strContains(r, "broken reference") {
			foundBroken = true
		}
		if strContains(r, "optional") {
			foundOptional = true
		}
	}
	if !foundBroken {
		t.Error("Expected recommendation about broken refs")
	}
	if !foundOptional {
		t.Error("Expected recommendation about optional refs")
	}
}

func TestRIGenRecsClean(t *testing.T) {
	s := RISummary{TotalRefs: 10}
	recs := riGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestRIIssueRank(t *testing.T) {
	if riIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if riIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if riIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
