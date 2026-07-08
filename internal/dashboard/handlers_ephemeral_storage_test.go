package dashboard

import "testing"

func TestESScore(t *testing.T) {
	// Clean cluster
	if score := esScore(ESSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Some issues
	s := ESSummary{
		TotalPods:        50,
		NoEphemeralLimit: 10, // -30
		UnboundedTmpfs:   5,  // -25
	}
	// 100 - 30 - 25 = 45
	if score := esScore(s); score != 45 {
		t.Errorf("Expected 45, got %d", score)
	}
}

func TestESGenRecs(t *testing.T) {
	s := ESSummary{
		TotalPods:        50,
		NoEphemeralLimit: 10,
		UnboundedTmpfs:   5,
		ComplianceScore:  30,
	}
	recs := esGenRecs(s)
	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}

	foundNoLimit := false
	foundUnbounded := false
	for _, r := range recs {
		if strContains(r, "ephemeral storage limit") {
			foundNoLimit = true
		}
		if strContains(r, "unbounded emptyDir") {
			foundUnbounded = true
		}
	}
	if !foundNoLimit {
		t.Error("Expected recommendation about missing ephemeral limits")
	}
	if !foundUnbounded {
		t.Error("Expected recommendation about unbounded emptyDir")
	}
}

func TestESGenRecsClean(t *testing.T) {
	s := ESSummary{TotalPods: 30, NoEphemeralLimit: 0, UnboundedTmpfs: 0}
	recs := esGenRecs(s)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestESRiskRank(t *testing.T) {
	if esRiskRank("high") != 0 {
		t.Error("Expected 0")
	}
	if esRiskRank("medium") != 1 {
		t.Error("Expected 1")
	}
	if esRiskRank("low") != 2 {
		t.Error("Expected 2")
	}
}

func TestESIssueRank(t *testing.T) {
	if esIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if esIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
