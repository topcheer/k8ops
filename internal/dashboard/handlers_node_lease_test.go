package dashboard

import (
	"testing"
	"time"
)

func TestNLScore(t *testing.T) {
	if score := nlScore(NLSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := NLSummary{TotalNodes: 5, ReadyNodes: 5}
	if score := nlScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s = NLSummary{
		TotalNodes:         5,
		NoLease:            1, // -15
		VeryStaleHeartbeat: 1, // -12
		StaleHeartbeat:     3, // (3-1)*6 = -12
		NotReadyNodes:      2, // -16
	}
	// 100 - 15 - 12 - 12 - 16 = 45
	if score := nlScore(s); score != 45 {
		t.Errorf("Expected 45, got %d", score)
	}

	// Floor at 0
	s = NLSummary{TotalNodes: 3, NoLease: 5, VeryStaleHeartbeat: 5, StaleHeartbeat: 5}
	if score := nlScore(s); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestNLGenRecs(t *testing.T) {
	s := NLSummary{
		TotalNodes:         5,
		NoLease:            1,
		VeryStaleHeartbeat: 1,
		StaleHeartbeat:     2,
		NotReadyNodes:      1,
		AvgHeartbeatAgeSec: 25,
		HealthScore:        40,
	}
	noLease := []NLEntry{{NodeName: "worker-3"}}
	stale := []NLEntry{{NodeName: "worker-2", HeartbeatAgeSec: 65}}

	recs := nlGenRecs(s, stale, noLease)

	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}

	foundNoLease := false
	foundVeryStale := false
	foundStale := false
	for _, r := range recs {
		if strContains(r, "no Lease") {
			foundNoLease = true
		}
		if strContains(r, ">2min") {
			foundVeryStale = true
		}
		if strContains(r, "stale heartbeat") {
			foundStale = true
		}
	}
	if !foundNoLease {
		t.Error("Expected recommendation about no lease")
	}
	if !foundVeryStale {
		t.Error("Expected recommendation about very stale heartbeat")
	}
	if !foundStale {
		t.Error("Expected recommendation about stale heartbeat")
	}
}

func TestNLGenRecsClean(t *testing.T) {
	s := NLSummary{TotalNodes: 5, ReadyNodes: 5}
	recs := nlGenRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestNLIssueRank(t *testing.T) {
	if nlIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if nlIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
	if nlIssueRank("info") != 2 {
		t.Error("Expected 2")
	}
}

func TestNLEntryHeartbeatAge(t *testing.T) {
	// Verify time math: a lease renewed 30s ago should have age ~30s
	now := time.Now()
	oldHeartbeat := now.Add(-30 * time.Second)
	age := now.Sub(oldHeartbeat).Seconds()
	if age < 29 || age > 31 {
		t.Errorf("Expected ~30s, got %.1f", age)
	}

	// 3 minutes ago
	oldHeartbeat = now.Add(-3 * time.Minute)
	age = now.Sub(oldHeartbeat).Seconds()
	if age < 179 || age > 181 {
		t.Errorf("Expected ~180s, got %.1f", age)
	}
}
