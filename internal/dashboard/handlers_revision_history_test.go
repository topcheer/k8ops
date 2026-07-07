package dashboard

import (
	"testing"
)

func TestRevHAssessRisk(t *testing.T) {
	entry := RevHEntry{}
	if revHAssessRisk(entry, 0) != "critical" {
		t.Error("Expected critical for rhl=0")
	}
	if revHAssessRisk(entry, 3) != "high" {
		t.Error("Expected high for rhl<5")
	}
	entry.ReplicaSetCount = 20
	if revHAssessRisk(entry, 10) != "medium" {
		t.Error("Expected medium for high churn")
	}
	entry.ReplicaSetCount = 5
	if revHAssessRisk(entry, 10) != "low" {
		t.Error("Expected low")
	}
}

func TestRevHScore(t *testing.T) {
	if revHScore(RevHSummary{}) != 100 {
		t.Errorf("Expected 100, got %d", revHScore(RevHSummary{}))
	}

	s := RevHSummary{
		TotalDeployments: 10,
		NoHistoryLimit:   2, // -30
		LowHistoryLimit:  3, // -15
	}
	// 100 - 30 - 15 = 55
	if score := revHScore(s); score != 55 {
		t.Errorf("Expected 55, got %d", score)
	}

	s = RevHSummary{TotalDeployments: 5, NoHistoryLimit: 10}
	if score := revHScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestRevHGenRecs(t *testing.T) {
	s := RevHSummary{
		TotalDeployments:   10,
		NoHistoryLimit:     2,
		LowHistoryLimit:    3,
		HighChurnWorkloads: 4,
		StaleRevisions:     1,
		AvgRevisionHistory: 3.5,
		RollbackReadiness:  40,
	}
	noHistory := []RevHEntry{{Namespace: "app", Name: "api"}}

	recs := revHGenRecs(s, noHistory, nil)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundNoHistory := false
	foundChurn := false
	foundStale := false
	for _, r := range recs {
		if strContains(r, "revisionHistoryLimit=0") {
			foundNoHistory = true
		}
		if strContains(r, "churn") {
			foundChurn = true
		}
		if strContains(r, "30 days") {
			foundStale = true
		}
	}
	if !foundNoHistory {
		t.Error("Expected recommendation about no history limit")
	}
	if !foundChurn {
		t.Error("Expected recommendation about churn")
	}
	if !foundStale {
		t.Error("Expected recommendation about stale revisions")
	}
}

func TestRevHGenRecsClean(t *testing.T) {
	s := RevHSummary{TotalDeployments: 10, AvgRevisionHistory: 10}
	recs := revHGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestRevHRiskRank(t *testing.T) {
	if revHRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if revHRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
}

func TestRevHIssueRank(t *testing.T) {
	if revHIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if revHIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
