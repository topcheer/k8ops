package dashboard

import "testing"

func TestDRScore(t *testing.T) {
	// Nothing configured
	if score := drScore(DRSummary{}); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}

	// Fully configured
	s := DRSummary{
		HasVelero:       true, // +35
		HasSnapshotCtrl: true, // +15
		MultiAZ:         true, // +15
		TotalNamespaces: 5,
		ProtectedNS:     5, // +35
	}
	if score := drScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Partial
	s = DRSummary{
		HasVelero:       true, // +35
		HasSnapshotCtrl: false,
		MultiAZ:         true, // +15
		TotalNamespaces: 10,
		ProtectedNS:     5, // +17
	}
	// 35 + 15 + 17 = 67
	if score := drScore(s); score != 67 {
		t.Errorf("Expected 67, got %d", score)
	}
}

func TestDRGenRecs(t *testing.T) {
	s := DRSummary{
		HasVelero:       false,
		TotalNamespaces: 5,
		ProtectedNS:     2,
		HasPVCs:         10,
		MultiAZ:         false,
		ReadinessScore:  20,
	}
	recs := drGenRecs(s)
	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundVelero := false
	foundUnprotected := false
	for _, r := range recs {
		if strContains(r, "Velero") {
			foundVelero = true
		}
		if strContains(r, "backup labels") {
			foundUnprotected = true
		}
	}
	if !foundVelero {
		t.Error("Expected recommendation about Velero")
	}
	if !foundUnprotected {
		t.Error("Expected recommendation about unprotected namespaces")
	}
}

func TestDRGenRecsGood(t *testing.T) {
	s := DRSummary{
		HasVelero:       true,
		HasSnapshotCtrl: true,
		MultiAZ:         true,
		TotalNamespaces: 5,
		ProtectedNS:     5,
		ReadinessScore:  100,
	}
	recs := drGenRecs(s)
	foundPositive := false
	for _, r := range recs {
		if strContains(r, "Good DR posture") {
			foundPositive = true
		}
	}
	if !foundPositive {
		t.Error("Expected positive recommendation for good DR posture")
	}
}

func TestDRStatusRank(t *testing.T) {
	if drStatusRank("fail") != 0 {
		t.Error("Expected 0")
	}
	if drStatusRank("pass") != 3 {
		t.Error("Expected 3")
	}
}

func TestDRIssueRank(t *testing.T) {
	if drIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if drIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
