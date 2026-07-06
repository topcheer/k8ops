package dashboard

import (
	"testing"
)

func TestLHIsValidLabelKey(t *testing.T) {
	// Valid keys
	valid := []string{
		"app",
		"app.kubernetes.io/name",
		"team",
		"my-label",
		"version",
		"foo.bar/baz",
	}
	for _, k := range valid {
		if !isValidLabelKey(k) {
			t.Errorf("Expected '%s' to be valid", k)
		}
	}

	// Invalid keys
	invalid := []string{
		"",
		"UPPERCASE",
		"app with space",
		"-leading-dash",
		"trailing-dash-",
		"has/slash/extra",
	}
	for _, k := range invalid {
		if isValidLabelKey(k) {
			t.Errorf("Expected '%s' to be invalid", k)
		}
	}
}

func TestLHAssessRisk(t *testing.T) {
	// No labels
	entry := LHEntry{LabelCount: 0}
	if level := lhAssessRisk(entry); level != "critical" {
		t.Errorf("Expected critical for no labels, got %s", level)
	}

	// Malformed
	entry = LHEntry{LabelCount: 3, HasMalformed: true}
	if level := lhAssessRisk(entry); level != "high" {
		t.Errorf("Expected high for malformed, got %s", level)
	}

	// Missing both standard and team
	entry = LHEntry{LabelCount: 2, MissingStandard: true, MissingTeam: true}
	if level := lhAssessRisk(entry); level != "medium" {
		t.Errorf("Expected medium for missing both, got %s", level)
	}

	// Missing just standard
	entry = LHEntry{LabelCount: 3, MissingStandard: true, MissingTeam: false}
	if level := lhAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for missing standard only, got %s", level)
	}

	// Clean
	entry = LHEntry{LabelCount: 5, MissingStandard: false, MissingTeam: false}
	if level := lhAssessRisk(entry); level != "low" {
		t.Errorf("Expected low for clean, got %s", level)
	}
}

func TestLHScore(t *testing.T) {
	// Empty
	if score := lhScore(LHSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := LHSummary{TotalWorkloads: 10, HasStandardLabel: 10, HasTeamLabel: 10}
	if score := lhScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = LHSummary{
		TotalWorkloads:   10,
		NoLabels:         2, // -40
		MalformedKeys:    1, // -10
		HasStandardLabel: 5, // 5 missing = -15
	}
	// 100 - 40 - 10 - 15 = 35
	if score := lhScore(s); score != 35 {
		t.Errorf("Expected 35, got %d", score)
	}
}

func TestLHNSScore(t *testing.T) {
	// Empty
	if score := lhNSScore(LHNSEntry{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	ns := LHNSEntry{WorkloadCount: 10}
	if score := lhNSScore(ns); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	ns = LHNSEntry{WorkloadCount: 10, MissingStandard: 5, MissingTeam: 3}
	// issues = 5 + 3 = 8
	// score = 100 - (8*100)/(10*4) = 100 - 20 = 80
	if score := lhNSScore(ns); score != 80 {
		t.Errorf("Expected 80, got %d", score)
	}
}

func TestLHGenRecs(t *testing.T) {
	s := LHSummary{
		TotalWorkloads:   10,
		NoLabels:         2,
		HasStandardLabel: 5,
		HasTeamLabel:     3,
		MalformedKeys:    1,
		ExcessiveLabels:  2,
		HealthScore:      40,
	}
	noLabels := []LHEntry{
		{Namespace: "default", Name: "app1"},
	}

	recs := lhGenRecs(s, noLabels, nil, nil)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundNoLabels := false
	foundStandard := false
	foundTeam := false
	foundMalformed := false
	for _, r := range recs {
		if strContains(r, "ZERO labels") {
			foundNoLabels = true
		}
		if strContains(r, "app.kubernetes.io/name") {
			foundStandard = true
		}
		if strContains(r, "team/owner") {
			foundTeam = true
		}
		if strContains(r, "malformed") {
			foundMalformed = true
		}
	}
	if !foundNoLabels {
		t.Error("Expected recommendation about no labels")
	}
	if !foundStandard {
		t.Error("Expected recommendation about standard labels")
	}
	if !foundTeam {
		t.Error("Expected recommendation about team labels")
	}
	if !foundMalformed {
		t.Error("Expected recommendation about malformed keys")
	}
}

func TestLHGenRecsClean(t *testing.T) {
	s := LHSummary{TotalWorkloads: 10, HasStandardLabel: 10, NoLabels: 0, MalformedKeys: 0}
	recs := lhGenRecs(s, nil, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestLHRiskRank(t *testing.T) {
	if lhRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if lhRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
	if lhRiskRank("medium") != 2 {
		t.Error("Expected 2")
	}
	if lhRiskRank("low") != 3 {
		t.Error("Expected 3")
	}
}

func TestLHIssueRank(t *testing.T) {
	if lhIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if lhIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if lhIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}

func TestLHGetOrCreateNS(t *testing.T) {
	m := make(map[string]*LHNSEntry)
	e1 := lhGetOrCreateNS(m, "default")
	e1.WorkloadCount = 5

	e2 := lhGetOrCreateNS(m, "default")
	if e2.WorkloadCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.WorkloadCount)
	}

	e3 := lhGetOrCreateNS(m, "monitoring")
	if e3.Namespace != "monitoring" {
		t.Errorf("Expected monitoring, got %s", e3.Namespace)
	}
}

func TestLHIsValidLabelSegment(t *testing.T) {
	if !isValidLabelSegment("app") {
		t.Error("Expected 'app' to be valid")
	}
	if !isValidLabelSegment("my-app") {
		t.Error("Expected 'my-app' to be valid")
	}
	if isValidLabelSegment("") {
		t.Error("Expected '' to be invalid")
	}
	if isValidLabelSegment("-leading") {
		t.Error("Expected '-leading' to be invalid")
	}
	if isValidLabelSegment("UPPER") {
		t.Error("Expected 'UPPER' to be invalid")
	}
}
