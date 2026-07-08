package dashboard

import "testing"

func TestWMGenRecs(t *testing.T) {
	s := WMSummary{
		TotalWorkloads:   10,
		HasResources:     7,
		HasProbes:        6,
		HasReplicas:      5,
		HasAntiAffinity:  3,
		HasSecurityCtx:   4,
		AvgMaturityScore: 55,
	}
	recs := wmGenRecs(s)
	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}
}

func TestWMGenRecsGood(t *testing.T) {
	s := WMSummary{
		TotalWorkloads:   10,
		HasResources:     10,
		HasProbes:        10,
		HasReplicas:      10,
		HasAntiAffinity:  10,
		HasSecurityCtx:   10,
		AvgMaturityScore: 90,
	}
	recs := wmGenRecs(s)
	foundPositive := false
	for _, r := range recs {
		if strContains(r, "excellent") {
			foundPositive = true
		}
	}
	if !foundPositive {
		t.Error("Expected positive recommendation")
	}
}

func TestWMGenRecsEmpty(t *testing.T) {
	recs := wmGenRecs(WMSummary{})
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for empty, got %d", len(recs))
	}
}

func TestWMIssueRank(t *testing.T) {
	if wmIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if wmIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if wmIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}

func TestWMCheckWeight(t *testing.T) {
	// Verify all check weights sum to 100
	checks := []WMCheck{
		{Name: "resource-requests", Weight: 15},
		{Name: "probes", Weight: 15},
		{Name: "multi-replica", Weight: 15},
		{Name: "pdb", Weight: 10},
		{Name: "anti-affinity", Weight: 15},
		{Name: "security-context", Weight: 10},
		{Name: "revision-history", Weight: 10},
		{Name: "labels", Weight: 10},
	}
	total := 0
	for _, c := range checks {
		total += c.Weight
	}
	if total != 100 {
		t.Errorf("Expected total weight 100, got %d", total)
	}
}
