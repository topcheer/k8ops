package dashboard

import "testing"

func TestCostAttributionResult1904(t *testing.T) {
	r := CostAttributionResult{
		Summary:     CostAttrSummary{Namespaces: 4, Teams: 2, EstMonthlyUSD: 500.0, WasteUSD: 50.0},
		HealthScore: 90,
	}
	if r.Summary.WasteUSD != 50.0 {
		t.Errorf("expected 50, got %f", r.Summary.WasteUSD)
	}
}

func TestQuotaForecastResult1904(t *testing.T) {
	r := QuotaForecastResult{
		Summary:     QuotaForecastSummary{TotalNamespaces: 10, WithQuota: 7, WithoutQuota: 3, CriticalUsage: 2},
		HealthScore: 55,
	}
	if r.Summary.WithoutQuota != 3 {
		t.Errorf("expected 3, got %d", r.Summary.WithoutQuota)
	}
}

func TestMeshReadinessDeepResult1904(t *testing.T) {
	r := MeshReadinessDeepResult{
		Summary:     MeshDeepSummary{TotalWorkloads: 60, ReadyForMesh: 30, MeshInjected: 10, Blockers: 40, WithoutProbes: 20},
		HealthScore: 50,
	}
	if r.Summary.WithoutProbes != 20 {
		t.Errorf("expected 20, got %d", r.Summary.WithoutProbes)
	}
}

func TestBuildCostAttrRecs1904(t *testing.T) {
	r := &CostAttributionResult{Summary: CostAttrSummary{EstMonthlyUSD: 500, WasteUSD: 100, Namespaces: 4, Teams: 2}}
	recs := buildCostAttrRecs1904(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildQuotaForecastRecs1904(t *testing.T) {
	r := &QuotaForecastResult{Summary: QuotaForecastSummary{WithQuota: 7, TotalNamespaces: 10, CriticalUsage: 2, HighUsage: 3, WithoutQuota: 3}}
	recs := buildQuotaForecastRecs1904(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}

func TestBuildMeshDeepRecs1904(t *testing.T) {
	r := &MeshReadinessDeepResult{Summary: MeshDeepSummary{ReadyForMesh: 30, TotalWorkloads: 60, MeshInjected: 10, Blockers: 40, WithoutProbes: 20, UnnamedPorts: 10}}
	recs := buildMeshDeepRecs1904(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2, got %d", len(recs))
	}
}
