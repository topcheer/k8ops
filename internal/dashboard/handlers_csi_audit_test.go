package dashboard

import "testing"

func TestCSIAssessRisk(t *testing.T) {
	if csiAssessRisk(CSIEntry{Provisioner: ""}) != "critical" {
		t.Error("Expected critical for empty provisioner")
	}
	if csiAssessRisk(CSIEntry{Provisioner: "p", IsDefault: true, AllowExpansion: false}) != "medium" {
		t.Error("Expected medium for non-expandable default")
	}
	if csiAssessRisk(CSIEntry{Provisioner: "p", ReclaimPolicy: "Delete"}) != "medium" {
		t.Error("Expected medium for Delete policy")
	}
	if csiAssessRisk(CSIEntry{Provisioner: "p", AllowExpansion: true, ReclaimPolicy: "Retain"}) != "low" {
		t.Error("Expected low")
	}
}

func TestCSIScore(t *testing.T) {
	if score := csiScore(CSISummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := CSISummary{
		TotalStorageClasses: 5,
		ExpandableSCs:       2,    // 3 non-expandable * 2 = -6
		NoDefaultSC:         true, // -15
		TotalCSIDrivers:     2,    // avoid -20 penalty
	}
	// 100 - 6 - 15 = 79
	if score := csiScore(s); score != 79 {
		t.Errorf("Expected 79, got %d", score)
	}

	s = CSISummary{TotalStorageClasses: 3, TotalCSIDrivers: 0}
	// 100 - 20 = 80, nonExpandable = 3*2 = -6
	// 100 - 6 - 20 = 74
	if score := csiScore(s); score != 74 {
		t.Errorf("Expected 74, got %d", score)
	}
}

func TestCSIGenRecs(t *testing.T) {
	s := CSISummary{
		TotalStorageClasses: 5,
		ExpandableSCs:       2,
		NoDefaultSC:         true,
		TotalCSIDrivers:     1,
		HealthScore:         60,
	}

	recs := csiGenRecs(s, nil)
	if len(recs) < 3 {
		t.Errorf("Expected at least 3 recommendations, got %d", len(recs))
	}

	foundDefault := false
	foundExpansion := false
	for _, r := range recs {
		if strContains(r, "default StorageClass") {
			foundDefault = true
		}
		if strContains(r, "expansion") {
			foundExpansion = true
		}
	}
	if !foundDefault {
		t.Error("Expected recommendation about default StorageClass")
	}
	if !foundExpansion {
		t.Error("Expected recommendation about expansion")
	}
}

func TestCSIGenRecsClean(t *testing.T) {
	s := CSISummary{
		TotalStorageClasses: 3,
		ExpandableSCs:       3,
		DefaultSCCount:      1,
		NoDefaultSC:         false,
		TotalCSIDrivers:     2,
	}
	recs := csiGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestCSIRiskRank(t *testing.T) {
	if csiRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if csiRiskRank("medium") != 1 {
		t.Error("Expected 1")
	}
}

func TestCSIIssueRank(t *testing.T) {
	if csiIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if csiIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
