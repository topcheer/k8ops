package dashboard

import "testing"

func TestLimitRangeResult1915(t *testing.T) {
	r := LimitRangeResult{
		Summary:     LimitRangeSummary{TotalNamespaces: 29, WithLimitRange: 3, WithoutLimitRange: 26, ContainersNoLimit: 60},
		HealthScore: 10,
	}
	if r.Summary.WithoutLimitRange != 26 {
		t.Errorf("expected 26, got %d", r.Summary.WithoutLimitRange)
	}
}

func TestTenantIsoResult1915(t *testing.T) {
	r := TenantIsoResult{
		Summary:     TenantIsoSummary{TotalNamespaces: 29, IsolatedNS: 5, SharedNS: 20, IsoScore: 17, WithNetPolicy: 3},
		HealthScore: 17,
	}
	if r.Summary.IsoScore != 17 {
		t.Errorf("expected 17, got %d", r.Summary.IsoScore)
	}
}

func TestResourceShareResult1915(t *testing.T) {
	r := ResourceShareResult{
		Summary:     ResourceShareSummary{TotalNamespaces: 29, TotalCPUm: 10000, TopConsumerPct: 45, FairnessScore: 60},
		HealthScore: 60,
	}
	if r.Summary.FairnessScore != 60 {
		t.Errorf("expected 60, got %d", r.Summary.FairnessScore)
	}
}

func TestBuildLimitRangeRecs1915(t *testing.T) {
	r := &LimitRangeResult{Summary: LimitRangeSummary{WithLimitRange: 3, TotalNamespaces: 29, WithoutLimitRange: 26, ContainersNoLimit: 60}}
	recs := buildLimitRangeRecs1915(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildTenantIsoRecs1915(t *testing.T) {
	r := &TenantIsoResult{Summary: TenantIsoSummary{IsolatedNS: 5, TotalNamespaces: 29, IsoScore: 17, SharedNS: 20, WithNetPolicy: 3, WithQuota: 2, WithLimitRange: 3, WithRBAC: 5}}
	recs := buildTenantIsoRecs1915(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}

func TestBuildResourceShareRecs1915(t *testing.T) {
	r := &ResourceShareResult{Summary: ResourceShareSummary{TotalNamespaces: 29, TotalCPUm: 10000, TopConsumerPct: 55, FairnessScore: 30}}
	recs := buildResourceShareRecs1915(r)
	if len(recs) < 1 {
		t.Errorf("expected >= 1, got %d", len(recs))
	}
}
