package dashboard

import (
	"testing"
)

func TestSLIsResourceShortage(t *testing.T) {
	shortages := []string{
		"Insufficient cpu (3)",
		"Insufficient memory (2)",
		"node(s) had untolerated taint",
		"exceeded quota",
		"node(s) resource",
	}
	for _, msg := range shortages {
		if !slIsResourceShortage(msg) {
			t.Errorf("Expected '%s' to be resource shortage", msg)
		}
	}

	nonShortages := []string{"node(s) didn't match node selector", "node affinity", ""}
	for _, msg := range nonShortages {
		if slIsResourceShortage(msg) {
			t.Errorf("Expected '%s' to NOT be resource shortage", msg)
		}
	}
}

func TestSLScore(t *testing.T) {
	// Empty
	if score := slScore(SLSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// Clean
	s := SLSummary{TotalPods: 50, RunningPods: 50}
	if score := slScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	// With issues
	s = SLSummary{
		TotalPods:       50,
		Unschedulable:   3, // -30
		NoNodeResources: 2, // -24
		VerySlowCount:   1, // -6
		SlowCount:       4, // (4-1)*3 = -9
		PendingPods:     5, // -20
	}
	// 100 - 30 - 24 - 6 - 9 - 20 = 11
	if score := slScore(s); score != 11 {
		t.Errorf("Expected 11, got %d", score)
	}
}

func TestSLGenRecs(t *testing.T) {
	s := SLSummary{
		TotalPods:       50,
		NoNodeResources: 3,
		Unschedulable:   5,
		VerySlowCount:   2,
		SlowCount:       5,
		PendingPods:     8,
		MaxScheduleSec:  400,
		EfficiencyScore: 30,
	}
	pending := []SLEntry{{Namespace: "app", PodName: "worker-1", PendingReason: "Insufficient cpu"}}

	recs := slGenRecs(s, pending, nil)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundResource := false
	foundUnschedulable := false
	foundSlow := false
	for _, r := range recs {
		if strContains(r, "insufficient CPU/memory") {
			foundResource = true
		}
		if strContains(r, "unschedulable") {
			foundUnschedulable = true
		}
		if strContains(r, "5min") {
			foundSlow = true
		}
	}
	if !foundResource {
		t.Error("Expected recommendation about resource shortage")
	}
	if !foundUnschedulable {
		t.Error("Expected recommendation about unschedulable pods")
	}
	if !foundSlow {
		t.Error("Expected recommendation about very slow scheduling")
	}
}

func TestSLGenRecsClean(t *testing.T) {
	s := SLSummary{TotalPods: 50, RunningPods: 50}
	recs := slGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestSLTruncate(t *testing.T) {
	short := "hello"
	if result := slTruncate(short, 60); result != short {
		t.Errorf("Expected unchanged, got %s", result)
	}

	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}
	result := slTruncate(long, 20)
	if len(result) > 20 {
		t.Errorf("Expected max 20 chars, got %d", len(result))
	}
	if result[len(result)-3:] != "..." {
		t.Error("Expected truncation suffix")
	}
}

func TestSLGetOrCreateNode(t *testing.T) {
	m := make(map[string]*SLNodeEntry)
	e1 := slGetOrCreateNode(m, "node-1")
	e1.PodCount = 5

	e2 := slGetOrCreateNode(m, "node-1")
	if e2.PodCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.PodCount)
	}

	e3 := slGetOrCreateNode(m, "node-2")
	if e3.NodeName != "node-2" {
		t.Errorf("Expected node-2, got %s", e3.NodeName)
	}
}

func TestSLIssueRank(t *testing.T) {
	if slIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if slIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if slIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
