package dashboard

import (
	"testing"
)

func TestPEAssessNodeRisk(t *testing.T) {
	if peAssessNodeRisk(&PENodeEntry{EvictionCount: 15}) != "critical" {
		t.Error("Expected critical for >=10")
	}
	if peAssessNodeRisk(&PENodeEntry{EvictionCount: 7}) != "high" {
		t.Error("Expected high for >=5")
	}
	if peAssessNodeRisk(&PENodeEntry{EvictionCount: 3}) != "medium" {
		t.Error("Expected medium for >=2")
	}
	if peAssessNodeRisk(&PENodeEntry{EvictionCount: 1}) != "low" {
		t.Error("Expected low")
	}
}

func TestPEScore(t *testing.T) {
	if score := peScore(PESummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := PESummary{
		RecentEvictions: 3, // -24
		MemoryEvictions: 5, // -15
		DiskEvictions:   4, // -12
		PIDEvictions:    2, // -4
	}
	// 100 - 24 - 15 - 12 - 4 = 45
	if score := peScore(s); score != 45 {
		t.Errorf("Expected 45, got %d", score)
	}
}

func TestPEGenRecs(t *testing.T) {
	s := PESummary{
		EvictedPods:     10,
		RecentEvictions: 3,
		MemoryEvictions: 5,
		DiskEvictions:   4,
		PIDEvictions:    1,
		HealthScore:     40,
	}
	byNode := []PENodeEntry{{NodeName: "node-1", EvictionCount: 8}}

	recs := peGenRecs(s, byNode)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundMemory := false
	foundDisk := false
	foundRecent := false
	for _, r := range recs {
		if strContains(r, "memory") {
			foundMemory = true
		}
		if strContains(r, "disk") {
			foundDisk = true
		}
		if strContains(r, "24h") {
			foundRecent = true
		}
	}
	if !foundMemory {
		t.Error("Expected recommendation about memory evictions")
	}
	if !foundDisk {
		t.Error("Expected recommendation about disk evictions")
	}
	if !foundRecent {
		t.Error("Expected recommendation about recent evictions")
	}
}

func TestPEGenRecsClean(t *testing.T) {
	s := PESummary{}
	recs := peGenRecs(s, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestPETruncate(t *testing.T) {
	if peTruncate("short", 60) != "short" {
		t.Error("Expected unchanged short string")
	}
	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}
	result := peTruncate(long, 20)
	if len(result) > 20 {
		t.Errorf("Expected max 20 chars, got %d", len(result))
	}
}

func TestPEIssueRank(t *testing.T) {
	if peIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if peIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
