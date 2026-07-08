package dashboard

import "testing"

func TestRLAssessRisk(t *testing.T) {
	if rlAssessRisk(15) != "high" {
		t.Error("Expected high for >10m")
	}
	if rlAssessRisk(7) != "medium" {
		t.Error("Expected medium for >5m")
	}
	if rlAssessRisk(3) != "low" {
		t.Error("Expected low")
	}
}

func TestRLScore(t *testing.T) {
	if score := rlScore(RLSummary{APIResponsive: false}); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
	if score := rlScore(RLSummary{APIResponsive: true}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}
	s := RLSummary{
		APIResponsive:    true,
		TotalPods:        50,
		LongStartingPods: 3, // -24
		NotReadyPods:     5, // -25
		ContainerWait:    4, // -12
	}
	// 100 - 24 - 25 - 12 = 39
	if score := rlScore(s); score != 39 {
		t.Errorf("Expected 39, got %d", score)
	}
}

func TestRLGenRecs(t *testing.T) {
	s := RLSummary{
		APIResponsive:       true,
		TotalPods:           50,
		LongStartingPods:    3,
		NotReadyPods:        5,
		ContainerWait:       4,
		ResponsivenessScore: 30,
	}
	slowPods := []RLEntry{{Namespace: "app", PodName: "api", PendingMin: 8.5}}

	recs := rlGenRecs(s, slowPods)
	if len(recs) < 4 {
		t.Errorf("Expected at least 4 recommendations, got %d", len(recs))
	}

	foundSlow := false
	foundNotReady := false
	for _, r := range recs {
		if strContains(r, "pending") {
			foundSlow = true
		}
		if strContains(r, "not ready") {
			foundNotReady = true
		}
	}
	if !foundSlow {
		t.Error("Expected recommendation about slow starting pods")
	}
	if !foundNotReady {
		t.Error("Expected recommendation about not-ready pods")
	}
}

func TestRLGenRecsClean(t *testing.T) {
	s := RLSummary{APIResponsive: true, TotalPods: 30}
	recs := rlGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestRLGenRecsUnresponsive(t *testing.T) {
	s := RLSummary{APIResponsive: false}
	recs := rlGenRecs(s, nil)
	found := false
	for _, r := range recs {
		if strContains(r, "not responding") {
			found = true
		}
	}
	if !found {
		t.Error("Expected recommendation about unresponsive API")
	}
}

func TestRLIssueRank(t *testing.T) {
	if rlIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if rlIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
