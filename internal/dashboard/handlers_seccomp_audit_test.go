package dashboard

import (
	"testing"
)

func TestSPAssessPSS(t *testing.T) {
	// Privileged = privileged level, critical risk
	entry := SPAEntry{IsPrivileged: true, Violations: []string{"Privileged"}}
	level, risk := spAssessPSS(entry)
	if level != "privileged" || risk != "critical" {
		t.Errorf("Expected privileged/critical, got %s/%s", level, risk)
	}

	// Clean = restricted, low
	entry = SPAEntry{Violations: nil}
	level, risk = spAssessPSS(entry)
	if level != "restricted" || risk != "low" {
		t.Errorf("Expected restricted/low, got %s/%s", level, risk)
	}

	// 1 violation = baseline, low
	entry = SPAEntry{Violations: []string{"no seccomp"}}
	level, risk = spAssessPSS(entry)
	if level != "baseline" || risk != "low" {
		t.Errorf("Expected baseline/low, got %s/%s", level, risk)
	}

	// 2 violations = baseline, medium
	entry = SPAEntry{Violations: []string{"a", "b"}}
	level, risk = spAssessPSS(entry)
	if level != "baseline" || risk != "medium" {
		t.Errorf("Expected baseline/medium, got %s/%s", level, risk)
	}

	// 4+ violations = baseline, high
	entry = SPAEntry{Violations: []string{"a", "b", "c", "d"}}
	level, risk = spAssessPSS(entry)
	if level != "baseline" || risk != "high" {
		t.Errorf("Expected baseline/high, got %s/%s", level, risk)
	}
}

func TestSPScore(t *testing.T) {
	// Empty
	if score := spScore(SPASummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := SPASummary{TotalContainers: 10, HasSeccomp: 10, DroppedAllCaps: 10}
	if score := spScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = SPASummary{
		TotalContainers: 10,
		NoSeccomp:       3, // -24
		PSSBaselineFail: 1, // -20
		NotDroppedAll:   4, // -16
		CanEscalate:     2, // -12
		RunsAsRoot:      3, // -9
	}
	// 100 - 24 - 20 - 16 - 12 - 9 = 19
	if score := spScore(s); score != 19 {
		t.Errorf("Expected 19, got %d", score)
	}
}

func TestSPGenRecs(t *testing.T) {
	s := SPASummary{
		TotalContainers: 10,
		PSSBaselineFail: 1,
		NoSeccomp:       4,
		CanEscalate:     3,
		NotDroppedAll:   5,
		RunsAsRoot:      2,
		ReadOnlyRootfs:  3,
		PSSRestrictedOK: 4,
		HardeningScore:  35,
	}

	recs := spGenRecs(s, nil, nil)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundPrivileged := false
	foundSeccomp := false
	foundEscalate := false
	foundDropCaps := false
	for _, r := range recs {
		if strContains(r, "privileged") {
			foundPrivileged = true
		}
		if strContains(r, "seccompProfile") {
			foundSeccomp = true
		}
		if strContains(r, "allowPrivilegeEscalation") {
			foundEscalate = true
		}
		if strContains(r, "capabilities.drop") {
			foundDropCaps = true
		}
	}
	if !foundPrivileged {
		t.Error("Expected recommendation about privileged")
	}
	if !foundSeccomp {
		t.Error("Expected recommendation about seccomp")
	}
	if !foundEscalate {
		t.Error("Expected recommendation about privilege escalation")
	}
	if !foundDropCaps {
		t.Error("Expected recommendation about capabilities drop")
	}
}

func TestSPGenRecsClean(t *testing.T) {
	s := SPASummary{TotalContainers: 10, PSSRestrictedOK: 10}
	recs := spGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestSPRiskRank(t *testing.T) {
	if spRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if spRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if spRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if spRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestSPIssueRank(t *testing.T) {
	if spIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if spIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if spIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
