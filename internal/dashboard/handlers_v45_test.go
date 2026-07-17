package dashboard

import (
	"testing"
)

func TestBuildComplianceGapRecs(t *testing.T) {
	r := &ComplianceGapResult{
		Summary: ComplianceGapSummary{PassedControls: 10, TotalControls: 23, CriticalGaps: 1, HighGaps: 5},
		ByFramework: []ComplianceFramework{
			{Name: "CIS", CoveragePct: 30}, {Name: "NIST", CoveragePct: 50},
		},
	}
	recs := buildComplianceGapRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestBuildFairnessRecs(t *testing.T) {
	r := &SchedulerFairnessResult{
		Summary:    FairnessSummary{ImbalanceRatio: 0.45},
		Imbalances: []FairnessImbalance{{Type: "over-loaded", Severity: "high"}},
	}
	recs := buildFairnessRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestSqrtFair(t *testing.T) {
	v := sqrtFair(16)
	if v < 3.9 || v > 4.1 {
		t.Errorf("expected ~4.0, got %.2f", v)
	}
	v2 := sqrtFair(0)
	if v2 != 0 {
		t.Errorf("expected 0, got %.2f", v2)
	}
}

func TestClassifyProfile(t *testing.T) {
	if p := classifyProfile("redis-cache", 0.5, 1); p != "cache" {
		t.Errorf("expected cache, got %s", p)
	}
	if p := classifyProfile("api-gateway", 0.5, 0.5); p != "gateway" {
		t.Errorf("expected gateway, got %s", p)
	}
	if p := classifyProfile("web-frontend", 0.2, 0.2); p != "web" {
		t.Errorf("expected web, got %s", p)
	}
}

func TestBuildFingerprintRecs(t *testing.T) {
	r := &WorkloadFingerprintResult{
		Summary:   FingerprintSummary{Duplicates: 3, IdleWorkloads: 2},
		ByProfile: []ProfileStat{{Profile: "web", Count: 10}},
	}
	recs := buildFingerprintRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
