package dashboard

import (
	"testing"
)

func TestDICalcEvictable(t *testing.T) {
	// maxUnavailable takes precedence
	if e := diCalcEvictable(5, "3", "1"); e != 1 {
		t.Errorf("Expected 1, got %d", e)
	}
	// Only minAvailable
	if e := diCalcEvictable(5, "3", ""); e != 2 {
		t.Errorf("Expected 2, got %d", e)
	}
	// All evictable (no PDB constraints)
	if e := diCalcEvictable(3, "", ""); e != 3 {
		t.Errorf("Expected 3, got %d", e)
	}
	// minAvailable = replicas → 0 evictable
	if e := diCalcEvictable(3, "3", ""); e != 0 {
		t.Errorf("Expected 0, got %d", e)
	}
}

func TestDIScore(t *testing.T) {
	if score := diScore(DISummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := DISummary{
		TotalWorkloads: 10,
		BlockDrain:     2, // -30
		RiskyPDB:       2, // -10
		NoPDB:          3, // -9
	}
	// 100 - 30 - 10 - 9 = 51
	if score := diScore(s); score != 51 {
		t.Errorf("Expected 51, got %d", score)
	}
}

func TestDIGenRecs(t *testing.T) {
	s := DISummary{
		TotalWorkloads:   10,
		BlockDrain:       2,
		RiskyPDB:         2,
		NoPDB:            3,
		MaintenanceScore: 40,
	}
	blocking := []DIEntry{{Namespace: "app", Name: "api"}}
	noPDB := []DIEntry{{Namespace: "app", Name: "web"}}

	recs := diGenRecs(s, blocking, noPDB)

	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundBlock := false
	foundNoPDB := false
	for _, r := range recs {
		if strContains(r, "block node drains") {
			foundBlock = true
		}
		if strContains(r, "no PDB") {
			foundNoPDB = true
		}
	}
	if !foundBlock {
		t.Error("Expected recommendation about blocking drains")
	}
	if !foundNoPDB {
		t.Error("Expected recommendation about missing PDBs")
	}
}

func TestDIGenRecsClean(t *testing.T) {
	s := DISummary{TotalWorkloads: 10, BlockDrain: 0, NoPDB: 0}
	recs := diGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestDIRiskRank(t *testing.T) {
	if diRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if diRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
}

func TestDIIssueRank(t *testing.T) {
	if diIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if diIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
