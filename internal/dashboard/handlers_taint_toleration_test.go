package dashboard

import (
	"testing"
)

func TestTTAssessNodeRisk(t *testing.T) {
	entry := TTNodeEntry{HasNoExecute: true}
	if level := ttAssessNodeRisk(entry); level != "critical" {
		t.Errorf("Expected critical, got %s", level)
	}

	entry = TTNodeEntry{Unschedulable: true}
	if level := ttAssessNodeRisk(entry); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}

	entry = TTNodeEntry{HasNoSchedule: true}
	if level := ttAssessNodeRisk(entry); level != "high" {
		t.Errorf("Expected high, got %s", level)
	}

	entry = TTNodeEntry{TaintCount: 2}
	if level := ttAssessNodeRisk(entry); level != "medium" {
		t.Errorf("Expected medium, got %s", level)
	}

	entry = TTNodeEntry{}
	if level := ttAssessNodeRisk(entry); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestTTScore(t *testing.T) {
	if score := ttScore(TTSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := TTSummary{
		TotalNodes:       5,
		NoExecuteNodes:   1, // -15
		CordonedNodes:    1, // -8
		NoScheduleNodes:  2, // -10
		PodsWithBroadTol: 3, // -9
	}
	// 100 - 15 - 8 - 10 - 9 = 58
	if score := ttScore(s); score != 58 {
		t.Errorf("Expected 58, got %d", score)
	}
}

func TestTTGenRecs(t *testing.T) {
	s := TTSummary{
		TotalNodes:       5,
		NoExecuteNodes:   1,
		CordonedNodes:    1,
		NoScheduleNodes:  2,
		PodsWithBroadTol: 3,
		ImpactScore:      40,
	}
	cordoned := []TTNodeEntry{{NodeName: "node-3"}}

	recs := ttGenRecs(s, nil, cordoned, nil)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundNoExecute := false
	foundCordoned := false
	foundBroad := false
	for _, r := range recs {
		if strContains(r, "NoExecute") {
			foundNoExecute = true
		}
		if strContains(r, "cordoned") {
			foundCordoned = true
		}
		if strContains(r, "broad tolerations") {
			foundBroad = true
		}
	}
	if !foundNoExecute {
		t.Error("Expected recommendation about NoExecute")
	}
	if !foundCordoned {
		t.Error("Expected recommendation about cordoned nodes")
	}
	if !foundBroad {
		t.Error("Expected recommendation about broad tolerations")
	}
}

func TestTTGenRecsClean(t *testing.T) {
	s := TTSummary{TotalNodes: 5}
	recs := ttGenRecs(s, nil, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestTTEffectOrAny(t *testing.T) {
	if ttEffectOrAny("") != "any" {
		t.Error("Expected 'any' for empty effect")
	}
	if ttEffectOrAny("NoSchedule") != "NoSchedule" {
		t.Error("Expected 'NoSchedule'")
	}
}

func TestTTIssueRank(t *testing.T) {
	if ttIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if ttIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
