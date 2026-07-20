package dashboard

import (
	"testing"
)

func TestDeployReproducibilityResultStruct1894(t *testing.T) {
	r := DeployReproducibilityResult{
		Summary: ReproducibilitySummary{
			TotalWorkloads:    60,
			FullyReproducible: 20,
			NonReproducible:   10,
			WithLatestTag:     30,
		},
		HealthScore: 33,
	}
	if r.Summary.WithLatestTag != 30 {
		t.Errorf("expected 30 latest tags, got %d", r.Summary.WithLatestTag)
	}
}

func TestUpdateComplianceResultStruct1894(t *testing.T) {
	r := UpdateComplianceResult{
		Summary: UpdateComplianceSummary{
			TotalWorkloads:   60,
			Compliant:        40,
			NonCompliant:     20,
			RecreateStrategy: 6,
		},
		HealthScore: 66,
	}
	if r.Summary.RecreateStrategy != 6 {
		t.Errorf("expected 6 Recreate, got %d", r.Summary.RecreateStrategy)
	}
}

func TestRestartPolicyResultStruct1894(t *testing.T) {
	r := RestartPolicyDeepResult{
		Summary: RestartPolicyDeepSummary{
			TotalWorkloads:  60,
			AlwaysPolicy:    58,
			OnFailurePolicy: 1,
			NeverPolicy:     1,
			Misconfigured:   30,
			HighRestartRate: 3,
			TotalRestarts:   150,
		},
		HealthScore: 50,
	}
	if r.Summary.TotalRestarts != 150 {
		t.Errorf("expected 150 restarts, got %d", r.Summary.TotalRestarts)
	}
}

func TestIsSensitiveEnvKey1894(t *testing.T) {
	sensitive := []string{"DB_PASSWORD", "API_KEY", "secret_token", "authCredential"}
	for _, s := range sensitive {
		if !isSensitiveEnvKey1894(s) {
			t.Errorf("expected %q to be sensitive", s)
		}
	}
	nonSensitive := []string{"LOG_LEVEL", "APP_NAME", "PORT", "DEBUG"}
	for _, s := range nonSensitive {
		if isSensitiveEnvKey1894(s) {
			t.Errorf("expected %q to NOT be sensitive", s)
		}
	}
}

func TestBuildReproducibilityRecs1894(t *testing.T) {
	result := &DeployReproducibilityResult{
		Summary: ReproducibilitySummary{
			TotalWorkloads:    50,
			FullyReproducible: 20,
			NonReproducible:   10,
			WithLatestTag:     25,
			WithNoTag:         5,
		},
	}
	recs := buildReproducibilityRecs1894(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}

func TestBuildUpdateComplianceRecs1894(t *testing.T) {
	result := &UpdateComplianceResult{
		Summary: UpdateComplianceSummary{
			TotalWorkloads:          50,
			Compliant:               35,
			NonCompliant:            15,
			RecreateStrategy:        5,
			WithoutProgressDeadline: 10,
		},
	}
	recs := buildUpdateComplianceRecs1894(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}

func TestBuildRestartPolicyRecs1894(t *testing.T) {
	result := &RestartPolicyDeepResult{
		Summary: RestartPolicyDeepSummary{
			TotalWorkloads:  50,
			Misconfigured:   20,
			HighRestartRate: 3,
			TotalRestarts:   100,
			WithoutLiveness: 30,
		},
	}
	recs := buildRestartPolicyRecs1894(result)
	if len(recs) < 2 {
		t.Errorf("expected at least 2 recs, got %d", len(recs))
	}
}

func TestHasPreStopHook1894(t *testing.T) {
	// No lifecycle
	if hasPreStopHook1894(nil) {
		t.Error("expected false for nil containers")
	}
}
