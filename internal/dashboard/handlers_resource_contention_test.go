package dashboard

import (
	"testing"
)

func TestRCAssessRisk(t *testing.T) {
	// Node pressure = critical
	entry := RCEntry{NodeName: "node-1", RestartCount: 0, Violations: nil}
	pressure := map[string]bool{"node-1": true}
	if level := rcAssessRisk(entry, pressure); level != "critical" {
		t.Errorf("Expected critical for node pressure, got %s", level)
	}

	// High restarts = high
	entry = RCEntry{NodeName: "node-1", RestartCount: 5}
	if level := rcAssessRisk(entry, pressure); level != "critical" {
		t.Errorf("Expected critical for pressure+restarts, got %s", level)
	}
	entry = RCEntry{NodeName: "ok", RestartCount: 5}
	if level := rcAssessRisk(entry, map[string]bool{}); level != "high" {
		t.Errorf("Expected high for 5 restarts, got %s", level)
	}

	// Medium restarts
	entry = RCEntry{NodeName: "ok", RestartCount: 3}
	if level := rcAssessRisk(entry, map[string]bool{}); level != "medium" {
		t.Errorf("Expected medium for 3 restarts, got %s", level)
	}

	// Violations = medium
	entry = RCEntry{NodeName: "ok", RestartCount: 0, Violations: []string{"no CPU limit"}}
	if level := rcAssessRisk(entry, map[string]bool{}); level != "medium" {
		t.Errorf("Expected medium for violations, got %s", level)
	}

	// Clean = low
	entry = RCEntry{NodeName: "ok", RestartCount: 0}
	if level := rcAssessRisk(entry, map[string]bool{}); level != "low" {
		t.Errorf("Expected low, got %s", level)
	}
}

func TestRCScore(t *testing.T) {
	if score := rcScore(RCSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := RCSummary{TotalPods: 20, RestartedPods: 5}
	if score := rcScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s = RCSummary{
		TotalPods:      20,
		MemoryPressure: 3, // -36
		ThrottledPods:  4, // -24
		CPULimitTooLow: 3, // -12
		NoCPULimits:    5, // -10
		NoMemoryLimits: 4, // -12
	}
	// 100 - 36 - 24 - 12 - 10 - 12 = 6
	if score := rcScore(s); score != 6 {
		t.Errorf("Expected 6, got %d", score)
	}
}

func TestRCGenRecs(t *testing.T) {
	s := RCSummary{
		TotalPods:         20,
		MemoryPressure:    2,
		ThrottledPods:     3,
		CPULimitTooLow:    4,
		NoCPULimits:       5,
		NoMemoryLimits:    3,
		MemoryLimitTooLow: 2,
		ContentionScore:   30,
	}
	throttled := []RCEntry{{Namespace: "app", PodName: "worker-1", RestartCount: 5}}

	recs := rcGenRecs(s, throttled, nil)

	if len(recs) < 7 {
		t.Errorf("Expected at least 7 recommendations, got %d", len(recs))
	}

	foundPressure := false
	foundThrottle := false
	foundNoCPU := false
	for _, r := range recs {
		if strContains(r, "MemoryPressure") {
			foundPressure = true
		}
		if strContains(r, "throttled") {
			foundThrottle = true
		}
		if strContains(r, "no CPU limit") {
			foundNoCPU = true
		}
	}
	if !foundPressure {
		t.Error("Expected recommendation about node pressure")
	}
	if !foundThrottle {
		t.Error("Expected recommendation about throttling")
	}
	if !foundNoCPU {
		t.Error("Expected recommendation about no CPU limits")
	}
}

func TestRCGenRecsClean(t *testing.T) {
	s := RCSummary{TotalPods: 20}
	recs := rcGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestRCGetOrCreateNS(t *testing.T) {
	m := make(map[string]*RCNSEntry)
	e1 := rcGetOrCreateNS(m, "default")
	e1.PodCount = 5

	e2 := rcGetOrCreateNS(m, "default")
	if e2.PodCount != 5 {
		t.Errorf("Expected same entry, got %d", e2.PodCount)
	}

	e3 := rcGetOrCreateNS(m, "app1")
	if e3.Namespace != "app1" {
		t.Errorf("Expected app1, got %s", e3.Namespace)
	}
}

func TestRCIssueRank(t *testing.T) {
	if rcIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if rcIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if rcIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}
