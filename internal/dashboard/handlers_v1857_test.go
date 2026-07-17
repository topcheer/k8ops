package dashboard

import (
	"testing"
	"time"
)

func TestCertChainRecs(t *testing.T) {
	r := &CertChainValidatorResult{
		Summary: CertChainSummary{
			TotalSecrets:    20,
			TLSSecrets:      15,
			ValidChains:     10,
			ExpiringSoon:    3,
			Expired:         2,
			ChainIncomplete: 4,
			SelfSigned:      1,
		},
		ValidationScore: 55,
		CriticalCerts: []CertChainEntry{
			{SecretName: "prod-tls", Namespace: "default", Status: "expired", DaysRemaining: -5, CertCN: "*.prod.com"},
		},
	}
	recs := buildCertChainRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestFeatureFlagRecs(t *testing.T) {
	r := &FeatureFlagAuditResult{
		Summary: FeatureFlagSummary{
			TotalFlags:      25,
			ConfigMapFlags:  10,
			AnnotationFlags: 5,
			EnvVarFlags:     10,
			EnabledFlags:    15,
			DisabledFlags:   10,
			StaleFlags:      3,
			UnmanagedFlags:  12,
		},
		CoverageScore: 45,
		StaleFlags: []FeatureFlagEntry{
			{FlagName: "FEATURE_TEMP_DEBUG", Namespace: "default", Value: "true"},
		},
	}
	recs := buildFeatureFlagRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestAutoscalerGapRecs(t *testing.T) {
	r := &AutoscalerGapResult{
		Summary: AutoscalerGapSummary{
			TotalNodes:        3,
			WorkerNodes:       1,
			PendingPods:       5,
			UnschedulablePods: 3,
			HasAutoscaler:     false,
			HasKarpenter:      false,
		},
		GapScore: 30,
	}
	recs := buildAutoscalerGapRecs(r)
	if len(recs) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(recs))
	}
}

func TestCertChainTypes(t *testing.T) {
	entry := CertChainEntry{
		SecretName:    "tls-prod",
		Namespace:     "prod",
		CertCN:        "*.example.com",
		Issuer:        "Let's Encrypt",
		NotBefore:     time.Now().AddDate(0, -2, 0),
		NotAfter:      time.Now().AddDate(0, 1, 0),
		DaysRemaining: 30,
		IsExpired:     false,
		ChainComplete: true,
		KeySize:       2048,
		Severity:      "warning",
		Status:        "expiring-warning",
	}
	if entry.DaysRemaining != 30 || entry.IsExpired {
		t.Error("should be 30 days remaining and not expired")
	}
}

func TestFeatureFlagTypes(t *testing.T) {
	entry := FeatureFlagEntry{
		FlagName:   "FEATURE_NEW_UI",
		Namespace:  "default",
		Source:     "ConfigMap",
		SourceName: "feature-config",
		Value:      "true",
		IsEnabled:  true,
		IsManaged:  true,
		RiskLevel:  "low",
	}
	if !entry.IsEnabled || entry.RiskLevel != "low" {
		t.Error("managed enabled flag should be low risk")
	}

	unmanaged := FeatureFlagEntry{
		FlagName:  "FEATURE_DEBUG_MODE",
		Source:    "EnvVar",
		Value:     "true",
		IsEnabled: true,
		IsStale:   true,
		RiskLevel: "high",
	}
	if unmanaged.RiskLevel != "high" {
		t.Error("stale env var flag should be high risk")
	}
}

func TestAutoscalerGapTypes(t *testing.T) {
	entry := PendingPodEntry{
		PodName:      "worker-5",
		Namespace:    "prod",
		Workload:     "worker",
		PendingTime:  "15m",
		Reason:       "Insufficient cpu",
		CPURequest:   2.0,
		MemRequestGB: 4.0,
	}
	if entry.CPURequest < 1.0 {
		t.Error("should have significant CPU request")
	}

	status := AutoscalerStatus{
		Detected:         "cluster-autoscaler",
		MinNodes:         3,
		MaxNodes:         10,
		ScaleDownEnabled: true,
		ExpanderType:     "least-waste",
	}
	if status.Detected != "cluster-autoscaler" || !status.ScaleDownEnabled {
		t.Error("CA should be detected with scale-down enabled")
	}
}

func TestIsFeatureFlagKey(t *testing.T) {
	if !isFeatureFlagKey("feature.new-ui") {
		t.Error("feature.new-ui should be detected as flag")
	}
	if !isFeatureFlagKey("FEATURE_DARK_MODE") {
		t.Error("FEATURE_DARK_MODE should be detected as flag")
	}
	if !isFeatureFlagKey("enable.beta-api") {
		t.Error("enable.beta-api should be detected as flag")
	}
	if isFeatureFlagKey("DATABASE_HOST") {
		t.Error("DATABASE_HOST should NOT be detected as flag")
	}
}

func TestIsFlagEnabled(t *testing.T) {
	if !isFlagEnabled("true") {
		t.Error("true should be enabled")
	}
	if !isFlagEnabled("1") {
		t.Error("1 should be enabled")
	}
	if !isFlagEnabled("on") {
		t.Error("on should be enabled")
	}
	if isFlagEnabled("false") {
		t.Error("false should not be enabled")
	}
	if isFlagEnabled("0") {
		t.Error("0 should not be enabled")
	}
}
