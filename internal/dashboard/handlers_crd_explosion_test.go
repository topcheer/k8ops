package dashboard

import (
	"testing"
)

func TestCRDAssessRisk(t *testing.T) {
	if crdAssessRisk(1500) != "critical" {
		t.Error("Expected critical for >1000")
	}
	if crdAssessRisk(700) != "high" {
		t.Error("Expected high for >500")
	}
	if crdAssessRisk(300) != "medium" {
		t.Error("Expected medium for >200")
	}
	if crdAssessRisk(50) != "low" {
		t.Error("Expected low for <=200")
	}
}

func TestCRDScore(t *testing.T) {
	if score := crdScore(CRDSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := CRDSummary{
		VeryHighCountCRDs: 2,    // -20
		HighCountCRDs:     4,    // (4-2)*5 = -10
		TotalConfigMaps:   1200, // -5
		TotalSecrets:      600,  // -5
		TotalCRDs:         60,   // -5
	}
	// 100 - 20 - 10 - 5 - 5 - 5 = 55
	if score := crdScore(s); score != 55 {
		t.Errorf("Expected 55, got %d", score)
	}

	// Floor at 0
	s = CRDSummary{VeryHighCountCRDs: 20}
	if score := crdScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestCRDGenRecs(t *testing.T) {
	s := CRDSummary{
		VeryHighCountCRDs:    2,
		HighCountCRDs:        3,
		TotalConfigMaps:      800,
		TotalSecrets:         300,
		TotalCRDs:            40,
		LargestNSObjectCount: 600,
		ScalabilityScore:     50,
	}

	recs := crdGenRecs(s, nil)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundVeryHigh := false
	foundSecrets := false
	foundCRDs := false
	for _, r := range recs {
		if strContains(r, ">1000 objects") {
			foundVeryHigh = true
		}
		if strContains(r, "external secret") {
			foundSecrets = true
		}
		if strContains(r, "CRDs installed") {
			foundCRDs = true
		}
	}
	if !foundVeryHigh {
		t.Error("Expected recommendation about very high object counts")
	}
	if !foundSecrets {
		t.Error("Expected recommendation about secret management")
	}
	if !foundCRDs {
		t.Error("Expected recommendation about CRD count")
	}
}

func TestCRDGenRecsClean(t *testing.T) {
	s := CRDSummary{TotalCRDs: 10}
	recs := crdGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestCRDGetOrCreateNS(t *testing.T) {
	m := make(map[string]*CRDNSEntry)
	e1 := crdGetOrCreateNS(m, "default")
	e1.Pods = 5

	e2 := crdGetOrCreateNS(m, "default")
	if e2.Pods != 5 {
		t.Errorf("Expected same entry, got %d", e2.Pods)
	}

	e3 := crdGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}

func TestCRDIssueRank(t *testing.T) {
	if crdIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if crdIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if crdIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
