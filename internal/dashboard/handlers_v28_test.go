package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPctInt(t *testing.T) {
	if v := pctInt(50, 100); v != 50 {
		t.Errorf("expected 50, got %f", v)
	}
	if v := pctInt(0, 0); v != 0 {
		t.Errorf("expected 0 for 0/0, got %f", v)
	}
	if v := pctInt(3, 4); v < 74 || v > 76 {
		t.Errorf("expected ~75, got %f", v)
	}
}

func TestBuildChaosScenarios(t *testing.T) {
	wls := []ChaosWorkload{
		{Name: "a", SurvivePodKill: true, SurviveNodeDrain: true, ResourceLimitsOK: true},
		{Name: "b", SurvivePodKill: false, SurviveNodeDrain: false, ResourceLimitsOK: false},
	}
	scs := buildChaosScenarios(wls)
	if len(scs) != 4 {
		t.Fatalf("expected 4 scenarios, got %d", len(scs))
	}
	// random-pod-kill: 1 safe, 1 impact
	for _, s := range scs {
		if s.Name == "random-pod-kill" {
			if s.SafeCount != 1 || s.ImpactCount != 1 {
				t.Errorf("pod-kill: expected 1 safe 1 impact, got %d safe %d impact", s.SafeCount, s.ImpactCount)
			}
		}
	}
}

func TestBuildChaosRecommendations(t *testing.T) {
	// Empty
	r := &ChaosReadinessResult{}
	recs := buildChaosRecommendations(r)
	if len(recs) != 0 {
		t.Errorf("expected 0 recs for empty, got %d", len(recs))
	}

	// Low PDB coverage
	r2 := &ChaosReadinessResult{
		Summary: ChaosSummary{TotalWorkloads: 10, WithPDB: 1, WithHealthProbe: 5, WithResourceLimits: 5, WithAntiAffinity: 2, MultiReplica: 8, AtRiskWorkloads: 5},
	}
	recs2 := buildChaosRecommendations(r2)
	if len(recs2) < 4 {
		t.Errorf("expected >= 4 recs, got %d", len(recs2))
	}
}

func TestAssessChaosReadinessFull(t *testing.T) {
	tgp := int64(30)
	cw := assessChaosReadiness("test", "default", "Deployment", 3,
		corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				TerminationGracePeriodSeconds: &tgp,
				Containers: []corev1.Container{
					{
						LivenessProbe: &corev1.Probe{},
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		},
		nil, []string{"app=test"}, 3,
	)
	if cw.GracefulShutdown != true {
		t.Error("expected graceful shutdown true")
	}
	if cw.HealthProbeOK != true {
		t.Error("expected health probe OK")
	}
	if cw.ResourceLimitsOK != true {
		t.Error("expected resource limits OK")
	}
	if cw.ReadinessScore < 60 {
		t.Errorf("expected score >= 60 for well-configured workload, got %d", cw.ReadinessScore)
	}
}

func TestBuildSupplyChainRecs(t *testing.T) {
	r := &SupplyChainResult{
		Summary: SupplyChainSummary{
			TotalImages:    10,
			ByDigest:       2,
			ByLatest:       3,
			NonRoot:        5,
			Privileged:     1,
			ReadOnlyRootFS: 3,
		},
	}
	recs := buildSupplyChainRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

func TestCalculateCapacityScore(t *testing.T) {
	// Low utilization, good headroom
	r := &CapacityForecastResult{
		Current: CapacityCurrent{
			CPUUtilization: 0.3,
			MemUtilization: 0.3,
			PodUtilization: 0.3,
		},
		Forecast: CapForecastData{
			Projection90d: CapacityProjection{Status: "ok"},
		},
	}
	score := calculateCapacityScore(r)
	if score < 90 {
		t.Errorf("expected >= 90 for low utilization, got %d", score)
	}

	// High utilization
	r2 := &CapacityForecastResult{
		Current: CapacityCurrent{
			CPUUtilization: 0.85,
			MemUtilization: 0.85,
			PodUtilization: 0.85,
		},
		Forecast: CapForecastData{
			Projection90d: CapacityProjection{Status: "critical"},
		},
	}
	score2 := calculateCapacityScore(r2)
	if score2 > 40 {
		t.Errorf("expected <= 40 for high utilization, got %d", score2)
	}
}

func TestBuildCapacityRecs(t *testing.T) {
	r := &CapacityForecastResult{
		Current: CapacityCurrent{
			CPUUtilization: 0.75,
			MemUtilization: 0.6,
		},
		Forecast: CapForecastData{
			GrowthRate30d:    CapacityGrowth{NewPodsPerWeek: 10},
			TimeToExhaustion: CapacityTTE{FirstBottleneck: "cpu", CPUExhaustionDays: 60},
		},
	}
	recs := buildCapacityRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}

	// Healthy cluster
	r2 := &CapacityForecastResult{
		Current: CapacityCurrent{
			CPUUtilization: 0.3,
			MemUtilization: 0.3,
			PodUtilization: 0.3,
		},
		Forecast: CapForecastData{
			GrowthRate30d:    CapacityGrowth{NewPodsPerWeek: 1},
			TimeToExhaustion: CapacityTTE{FirstBottleneck: "none"},
		},
	}
	recs2 := buildCapacityRecs(r2)
	if len(recs2) == 0 {
		t.Error("expected at least 1 rec for healthy cluster")
	}
}
