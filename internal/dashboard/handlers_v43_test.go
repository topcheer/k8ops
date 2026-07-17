package dashboard

import (
	"testing"
)

func TestBuildContainerHardeningRecs(t *testing.T) {
	r := &ContainerHardeningResult{
		Summary:    HardeningSummary{MissingAll: 5, NonRoot: 10, ReadOnlyRootFS: 5, DropAllCaps: 3, TotalContainers: 20},
		BatchPatch: []HardeningPatch{{Workload: "app", Namespace: "ns"}},
	}
	recs := buildContainerHardeningRecs(r)
	if len(recs) < 4 {
		t.Errorf("expected >= 4 recs, got %d", len(recs))
	}
}

func TestGenerateHardeningPatch(t *testing.T) {
	issues := []string{"missing runAsNonRoot=true", "missing readOnlyRootFilesystem=true"}
	p := generateHardeningPatch("app", issues)
	if p == "" {
		t.Error("expected non-empty patch")
	}
}

func TestBuildASReadinessRecs(t *testing.T) {
	r := &AutoscaleReadinessResult{
		Summary: ASReadinessSummary{GoodCandidates: 5, WithoutHPA: 10, MultiReplica: 8, WithRequests: 6},
	}
	recs := buildASReadinessRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected >= 2 recs, got %d", len(recs))
	}
}

func TestGenerateHPAYAML(t *testing.T) {
	yaml := generateHPAYAML("web", "prod", 3)
	if yaml == "" {
		t.Error("expected non-empty YAML")
	}
}

func TestBuildEfficiencyRecs(t *testing.T) {
	r := &WorkloadEfficiencyResult{
		Summary:       EfficiencySummary{OverProvisioned: 3, IdleReplicas: 2, UnderProvisioned: 5},
		WasteEstimate: EfficiencyWaste{TotalWasteUSD: 50.5, WastePct: 25},
	}
	recs := buildEfficiencyRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}
