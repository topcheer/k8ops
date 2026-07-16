package dashboard

import "testing"

func TestComputeSecretCompScore(t *testing.T) {
	if s := computeSecretCompScore(SecretCompSummary{}); s != 100 {
		t.Errorf("expected 100, got %d", s)
	}
	s2 := SecretCompSummary{TotalSecrets: 10, StaleCount: 5}
	if s := computeSecretCompScore(s2); s > 75 {
		t.Errorf("expected <= 75, got %d", s)
	}
}

func TestComputeHPABehaviorScore(t *testing.T) {
	e := HPABehaviorEntry{HasBehavior: true, FlapRisk: "low", MinReplicas: 1, MaxReplicas: 10}
	if s := computeHPAScore(e); s < 80 {
		t.Errorf("expected >= 80, got %d", s)
	}
	e2 := HPABehaviorEntry{HasBehavior: false, FlapRisk: "high", MinReplicas: 3, MaxReplicas: 3}
	if s := computeHPAScore(e2); s > 40 {
		t.Errorf("expected <= 40, got %d", s)
	}
}

func TestComputeAccessScore(t *testing.T) {
	if s := computeAccessScore(AccessSummary{}); s != 100 {
		t.Errorf("expected 100, got %d", s)
	}
	s2 := AccessSummary{TotalEvents: 100, FailedCount: 60}
	if s := computeAccessScore(s2); s > 75 {
		t.Errorf("expected <= 75, got %d", s)
	}
}

func TestClassifyScalePolicy(t *testing.T) {
	if p := classifyScalePolicy(nil); p != "default" {
		t.Errorf("expected 'default', got %q", p)
	}
}

func TestGenerateSecretCompRecs(t *testing.T) {
	r := SecretComplianceResult{
		Summary: SecretCompSummary{TotalSecrets: 20, CompliantCount: 15, StaleCount: 5, CompliancePct: 75, OldestAgeDays: 400},
		HealthScore: 70,
		StaleSecrets: []SecretCompStale{{Severity: "critical", AgeDays: 400}},
	}
	recs := generateSecretCompRecs(r)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}
