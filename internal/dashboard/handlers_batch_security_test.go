package dashboard

import (
	"testing"
)

func TestBSIsSuspiciousSchedule(t *testing.T) {
	suspicious := []string{"* * * * *", "*/1 * * * *"}
	for _, s := range suspicious {
		if !bsIsSuspiciousSchedule(s) {
			t.Errorf("Expected '%s' to be suspicious", s)
		}
	}

	normal := []string{"0 2 * * *", "*/5 * * * *", "0 0 * * 0", "*/30 * * * *"}
	for _, s := range normal {
		if bsIsSuspiciousSchedule(s) {
			t.Errorf("Expected '%s' to NOT be suspicious", s)
		}
	}
}

func TestBSAssessRisk(t *testing.T) {
	// Privileged = critical
	entry := BSEntry{IsPrivileged: true}
	if level := bsAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	// HostPath = critical
	entry = BSEntry{HostPath: []string{"/etc"}}
	if level := bsAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for hostPath, got %s", level)
	}

	// HostNetwork = high
	entry = BSEntry{HasHostNetwork: true}
	if level := bsAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for hostNetwork, got %s", level)
	}

	// Default SA = medium
	entry = BSEntry{IsDefaultSA: true, HasResourceLimits: true}
	if level := bsAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for default SA, got %s", level)
	}

	// No limits = medium
	entry = BSEntry{HasResourceLimits: false}
	if level := bsAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for no limits, got %s", level)
	}

	// Clean = low
	entry = BSEntry{HasResourceLimits: true, IsDefaultSA: false}
	if level := bsAssessRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestBSScore(t *testing.T) {
	// Empty
	if score := bsScore(BSSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := BSSummary{TotalCronJobs: 5, TotalJobs: 5}
	if score := bsScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = BSSummary{
		TotalCronJobs:    5,
		Privileged:       1, // -20
		HostPath:         2, // -30
		HostNetwork:      1, // -8
		DefaultSA:        3, // -12
		NoResourceLimits: 2, // -6
		SuspiciousSched:  1, // -6
	}
	// 100 - 20 - 30 - 8 - 12 - 6 - 6 = 18
	if score := bsScore(s); score != 18 {
		t.Errorf("Expected 18, got %d", score)
	}
}

func TestBSGenRecs(t *testing.T) {
	s := BSSummary{
		TotalCronJobs:      5,
		TotalJobs:          3,
		Privileged:         1,
		HostPath:           2,
		HostNetwork:        1,
		DefaultSA:          3,
		NoResourceLimits:   2,
		SuspiciousSched:    1,
		NoConcurrencyLimit: 2,
		MountsSecrets:      4,
		SecurityScore:      25,
	}

	recs := bsGenRecs(s, nil, nil)

	if len(recs) < 8 {
		t.Errorf("Expected at least 8 recommendations, got %d", len(recs))
	}

	foundPriv := false
	foundHostPath := false
	foundSuspicious := false
	foundConcurrency := false
	for _, r := range recs {
		if strContains(r, "privileged") {
			foundPriv = true
		}
		if strContains(r, "hostPath") {
			foundHostPath = true
		}
		if strContains(r, "suspicious") {
			foundSuspicious = true
		}
		if strContains(r, "concurrency") {
			foundConcurrency = true
		}
	}
	if !foundPriv {
		t.Error("Expected recommendation about privileged")
	}
	if !foundHostPath {
		t.Error("Expected recommendation about hostPath")
	}
	if !foundSuspicious {
		t.Error("Expected recommendation about suspicious schedule")
	}
	if !foundConcurrency {
		t.Error("Expected recommendation about concurrency limit")
	}
}

func TestBSGenRecsClean(t *testing.T) {
	s := BSSummary{TotalCronJobs: 5, TotalJobs: 5}
	recs := bsGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestBSRiskRank(t *testing.T) {
	if bsRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if bsRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if bsRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if bsRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestBSIssueRank(t *testing.T) {
	if bsIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if bsIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if bsIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
