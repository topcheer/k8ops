package dashboard

import (
	"testing"
)

func TestBuildDrainRecs(t *testing.T) {
	// Single node - critical
	r := &DrainImpactResult{
		RescheduleFeas: DrainReschedule{RemainingNodes: 0},
	}
	recs := buildDrainRecs(r)
	if len(recs) == 0 {
		t.Error("expected recommendations for single node")
	}
	found := false
	for _, rec := range recs {
		if containsStr(rec, "CRITICAL") {
			found = true
		}
	}
	if !found {
		t.Error("expected CRITICAL warning for single node")
	}

	// CPU insufficient
	r2 := &DrainImpactResult{
		RescheduleFeas: DrainReschedule{
			RemainingNodes: 2,
			FitsCPU:        false,
			FitsMem:        true,
			FitsPods:       true,
			AvailableCPU:   2.0,
			NeededCPU:      4.0,
		},
		ImpactSummary: DrainImpactSummary{},
	}
	recs2 := buildDrainRecs(r2)
	if len(recs2) == 0 {
		t.Error("expected recommendations for CPU insufficient")
	}

	// Safe to drain
	r3 := &DrainImpactResult{
		RescheduleFeas: DrainReschedule{
			RemainingNodes: 3,
			FitsCPU:        true,
			FitsMem:        true,
			FitsPods:       true,
		},
		SafeToDrain: true,
	}
	recs3 := buildDrainRecs(r3)
	if len(recs3) == 0 {
		t.Error("expected at least one recommendation for safe drain")
	}
}

func TestImpactTypeForPod(t *testing.T) {
	dpi := DrainPodImpact{CanReschedule: false}
	if v := impactTypeForPod(dpi); v != "unavailable" {
		t.Errorf("expected unavailable, got %s", v)
	}
	dpi2 := DrainPodImpact{CanReschedule: true, HasPDB: true}
	if v := impactTypeForPod(dpi2); v != "degraded" {
		t.Errorf("expected degraded, got %s", v)
	}
	dpi3 := DrainPodImpact{CanReschedule: true, HasPDB: false}
	if v := impactTypeForPod(dpi3); v != "transient" {
		t.Errorf("expected transient, got %s", v)
	}
}

func TestComputeReqAccScore(t *testing.T) {
	// Well-configured
	r := &RequestAccuracyResult{
		Summary: ReqAccSummary{
			TotalContainers:    10,
			WithRequests:       10,
			WithLimits:         10,
			Balanced:           9,
			OvercommitRatio:    1.5,
			MemOvercommitRatio: 1.5,
		},
	}
	score := computeReqAccScore(r)
	if score < 90 {
		t.Errorf("expected >= 90 for well-configured, got %d", score)
	}

	// Poor configuration
	r2 := &RequestAccuracyResult{
		Summary: ReqAccSummary{
			TotalContainers:    10,
			NoRequests:         5,
			NoLimits:           5,
			Balanced:           2,
			OvercommitRatio:    4.0,
			MemOvercommitRatio: 3.0,
			UnderProvisioned:   2,
		},
	}
	score2 := computeReqAccScore(r2)
	if score2 > 40 {
		t.Errorf("expected <= 40 for poor config, got %d", score2)
	}
}

func TestBuildReqAccRecs(t *testing.T) {
	r := &RequestAccuracyResult{
		Summary: ReqAccSummary{
			NoRequests:       3,
			NoLimits:         5,
			OverProvisioned:  2,
			OvercommitRatio:  2.5,
			UnderProvisioned: 1,
		},
		RightsizingSavings: ReqAccSavings{
			WastedCPU:            2.5,
			WastedMem:            5.0,
			EstimatedMonthlyCost: 70.0,
		},
	}
	recs := buildReqAccRecs(r)
	if len(recs) < 5 {
		t.Errorf("expected >= 5 recs, got %d", len(recs))
	}
}

func TestIsSensitiveEnvVar(t *testing.T) {
	if !isSensitiveEnvVar("DB_PASSWORD") {
		t.Error("expected DB_PASSWORD to be sensitive")
	}
	if !isSensitiveEnvVar("API_KEY") {
		t.Error("expected API_KEY to be sensitive")
	}
	if !isSensitiveEnvVar("secret_token") {
		t.Error("expected secret_token to be sensitive")
	}
	if isSensitiveEnvVar("APP_NAME") {
		t.Error("expected APP_NAME to NOT be sensitive")
	}
}

func TestHardeningDimStatus(t *testing.T) {
	if v := hardeningDimStatus(90); v != "healthy" {
		t.Errorf("expected healthy, got %s", v)
	}
	if v := hardeningDimStatus(65); v != "warning" {
		t.Errorf("expected warning, got %s", v)
	}
	if v := hardeningDimStatus(45); v != "at-risk" {
		t.Errorf("expected at-risk, got %s", v)
	}
	if v := hardeningDimStatus(20); v != "critical" {
		t.Errorf("expected critical, got %s", v)
	}
}

func TestMinIntVal(t *testing.T) {
	if v := minIntVal(3, 7); v != 3 {
		t.Errorf("expected 3, got %d", v)
	}
	if v := minIntVal(10, 5); v != 5 {
		t.Errorf("expected 5, got %d", v)
	}
}

func TestBuildHardeningRecs(t *testing.T) {
	r := &HardeningScoreResult{
		OverallScore: 35,
		Dimensions: []HardeningDim{
			{Name: "PSS", Score: 25, Description: "安全上下文"},
			{Name: "Network", Score: 45, Description: "网络策略"},
			{Name: "RBAC", Score: 85, Description: "权限管理"},
		},
		TopRisks: []HardeningRisk{
			{Severity: "critical", Finding: "特权容器"},
			{Severity: "high", Finding: "无网络策略"},
		},
	}
	recs := buildHardeningRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected >= 3 recs, got %d", len(recs))
	}
}

// containsStr and contains are defined in handlers_bottleneck_test.go
