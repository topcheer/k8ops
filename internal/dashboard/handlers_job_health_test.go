package dashboard

import "testing"

func TestJBAssessRisk(t *testing.T) {
	if jbAssessRisk(JBEntry{Status: "Failed"}) != "high" {
		t.Error("Expected high for failed")
	}
	if jbAssessRisk(JBEntry{Status: "Running", DurationHours: 30}) != "high" {
		t.Error("Expected high for long running")
	}
	if jbAssessRisk(JBEntry{Status: "Suspended"}) != "medium" {
		t.Error("Expected medium for suspended")
	}
	if jbAssessRisk(JBEntry{Status: "Complete"}) != "low" {
		t.Error("Expected low for complete")
	}
}

func TestJBScore(t *testing.T) {
	if jbScore(JBSummary{}) != 100 {
		t.Errorf("Expected 100, got %d", jbScore(JBSummary{}))
	}
	s := JBSummary{
		TotalJobs:       10,
		FailedJobs:      3, // -30
		LongRunningJobs: 2, // -16
		NoBackoffLimit:  4, // -8
	}
	// 100 - 30 - 16 - 8 = 46
	if score := jbScore(s); score != 46 {
		t.Errorf("Expected 46, got %d", score)
	}
}

func TestJBGenRecs(t *testing.T) {
	s := JBSummary{
		TotalJobs:       10,
		FailedJobs:      3,
		LongRunningJobs: 2,
		SuspendedJobs:   1,
		NoBackoffLimit:  4,
		HealthScore:     40,
	}
	recs := jbGenRecs(s, nil, nil)
	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}
	foundFailed := false
	foundLong := false
	for _, r := range recs {
		if strContains(r, "failed") {
			foundFailed = true
		}
		if strContains(r, "24h") {
			foundLong = true
		}
	}
	if !foundFailed {
		t.Error("Expected recommendation about failed jobs")
	}
	if !foundLong {
		t.Error("Expected recommendation about long-running jobs")
	}
}

func TestJBGenRecsClean(t *testing.T) {
	s := JBSummary{TotalJobs: 5, FailedJobs: 0, LongRunningJobs: 0}
	recs := jbGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestJBRiskRank(t *testing.T) {
	if jbRiskRank("high") != 0 {
		t.Error("Expected 0")
	}
}

func TestJBIssueRank(t *testing.T) {
	if jbIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
