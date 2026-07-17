package dashboard

import (
	"testing"
)

func TestMTLSTrustRecs(t *testing.T) {
	r := &MTLSTrustDomainResult{
		Summary:    MTLSTrustSummary{TotalNamespaces: 10, MeshInjected: 5, StrictMtls: 2, PermissiveMtls: 3, MtlsDisabled: 5},
		MeshStatus: MeshStatus{Detected: "istio"},
		TrustScore: 32,
	}
	recs := buildMTLSTrustRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >=2 recs, got %d", len(recs))
	}
}

func TestLatencyBudgetRecs(t *testing.T) {
	r := &LatencyBudgetResult{
		TargetP99:   500,
		Summary:     LatencyBudgetSummary{TotalServices: 20, OverBudgetCount: 5, WithinBudget: 12, AvgEstimated: 350},
		BudgetScore: 60,
		OverBudget:  []LatencyBudgetEntry{{ServiceName: "slow-api", Namespace: "prod", EstimatedMs: 800, AllocatedMs: 500, BudgetUtilPct: 160}},
	}
	recs := buildLatencyBudgetRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >=2 recs, got %d", len(recs))
	}
}

func TestDisruptionToleranceRecs(t *testing.T) {
	r := &PodDisruptionToleranceResult{
		Summary:        DisruptionToleranceSummary{TotalWorkloads: 30, SingleReplica: 10, WithPDB: 5, DataLossRiskWL: 3},
		ToleranceScore: 40,
		VoluntaryRisk:  []ToleranceEntry{{Workload: "db", ToleranceLevel: "none"}},
	}
	recs := buildDisruptionToleranceRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >=2 recs, got %d", len(recs))
	}
}

func TestMTLSTypes(t *testing.T) {
	e := MTLSNsEntry{Namespace: "prod", MeshInjected: true, MtlsMode: "strict", RiskLevel: "none"}
	if !e.MeshInjected || e.MtlsMode != "strict" {
		t.Error("should be strict mesh")
	}
}

func TestLatencyBudgetTypes(t *testing.T) {
	e := LatencyBudgetEntry{ServiceName: "api", EstimatedMs: 300, AllocatedMs: 500, BudgetUtilPct: 60, Status: "within-budget"}
	if e.BudgetUtilPct > 100 {
		t.Error("should be within budget")
	}
}

func TestToleranceEntryTypes(t *testing.T) {
	e := ToleranceEntry{Workload: "web", Replicas: 3, NodeSpread: 2, VoluntaryScore: 70, ToleranceLevel: "high"}
	if e.ToleranceLevel != "high" {
		t.Error("should be high tolerance")
	}
}
