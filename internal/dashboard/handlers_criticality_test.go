package dashboard

import (
	"testing"
)

func TestComputeCritScore(t *testing.T) {
	// High criticality: 5 replicas, PDB, HPA, ingress, high CPU, prod ns, old
	e := CritEntry{
		Replicas: 5, HasPDB: true, HasHPA: true, HasIngress: true,
		CPURequest: 3, AgeDays: 200, Namespace: "production",
	}
	score, signals := computeCritScore(e)
	if score < 70 {
		t.Errorf("expected score >= 70 for critical workload, got %d", score)
	}
	if len(signals) < 3 {
		t.Errorf("expected at least 3 signals, got %d", len(signals))
	}

	// Low criticality: 1 replica, no PDB/HPA/ingress, low CPU, dev ns, new
	e2 := CritEntry{
		Replicas: 1, Namespace: "dev-test", AgeDays: 3,
	}
	score2, _ := computeCritScore(e2)
	if score2 > 30 {
		t.Errorf("expected score <= 30 for dev workload, got %d", score2)
	}
}

func TestComputeCritHealthScore(t *testing.T) {
	// Good: Tier-0 with PDB
	s := CritSummary{TotalWorkloads: 10, Tier0Critical: 3, WithPDB: 3, WithHPA: 3, HAWorkloads: 3}
	if score := computeCritHealthScore(s); score < 80 {
		t.Errorf("expected score >= 80, got %d", score)
	}

	// Bad: Tier-0 without PDB
	s2 := CritSummary{TotalWorkloads: 10, Tier0Critical: 5, WithPDB: 0}
	if score := computeCritHealthScore(s2); score > 60 {
		t.Errorf("expected score <= 60 for no PDB, got %d", score)
	}
}

func TestTierName(t *testing.T) {
	if name := tierName("Tier-0"); name != "Critical" {
		t.Errorf("expected 'Critical', got %q", name)
	}
	if name := tierName("Tier-3"); name != "Best-effort" {
		t.Errorf("expected 'Best-effort', got %q", name)
	}
}

func TestGenerateCritRecs(t *testing.T) {
	r := CriticalityResult{
		Summary: CritSummary{
			TotalWorkloads: 10, Tier0Critical: 3, Tier1Important: 2,
			Tier2Standard: 3, Tier3BestEffort: 2, WithPDB: 2,
		},
		HealthScore: 70,
		ByTier: []TierStat{
			{Tier: "Tier-0", Count: 3, WithPDB: 2, WithHPA: 1},
		},
	}
	recs := generateCritRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}
