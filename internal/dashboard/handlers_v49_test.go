package dashboard

import (
	"testing"
)

func TestBuildRBACDriftRecs(t *testing.T) {
	r := &RBACDriftResult{
		Summary: RBACDriftSummary{ClusterAdmins: 2, WildcardPerms: 3, OverPermissive: 5},
	}
	recs := buildRBACDriftRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestBuildRBACDriftRecsClean(t *testing.T) {
	r := &RBACDriftResult{Summary: RBACDriftSummary{}}
	recs := buildRBACDriftRecs(r)
	if len(recs) != 1 {
		t.Errorf("expected 1 rec, got %d", len(recs))
	}
}

func TestCalcMonthsToExhaust(t *testing.T) {
	m := calcMonthsToExhaust(5, 20, 1)
	if m <= 0 || m >= 20 {
		t.Errorf("expected reasonable months, got %d", m)
	}
	m2 := calcMonthsToExhaust(20, 20, 1)
	if m2 != 0 {
		t.Errorf("expected 0 for at capacity, got %d", m2)
	}
}

func TestThresholdFromMonths(t *testing.T) {
	if thresholdFromMonths(0) != "critical" {
		t.Error("expected critical")
	}
	if thresholdFromMonths(2) != "warning" {
		t.Error("expected warning")
	}
	if thresholdFromMonths(5) != "notice" {
		t.Error("expected notice")
	}
	if thresholdFromMonths(12) != "safe" {
		t.Error("expected safe")
	}
}

func TestBuildWarmstartRecs(t *testing.T) {
	r := &ConfigWarmstartResult{
		Summary:       WarmstartSummary{SlowStarters: 3, WarmstartCandidates: 5},
		Optimizations: []WarmstartOpt{{Category: "test"}},
	}
	recs := buildWarmstartRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
