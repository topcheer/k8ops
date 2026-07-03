package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeSecretExposure_EmptyCluster(t *testing.T) {
	report := analyzeSecretExposure([]corev1.Secret{}, []corev1.Pod{})

	if report.Summary.TotalSecrets != 0 {
		t.Errorf("expected 0 secrets, got %d", report.Summary.TotalSecrets)
	}
	if len(report.Exposed) != 0 {
		t.Errorf("expected 0 exposed vars, got %d", len(report.Exposed))
	}
}

func TestAnalyzeSecretExposure_HardcodedPassword(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "nginx",
						Env: []corev1.EnvVar{
							{Name: "DATABASE_PASSWORD", Value: "super_secret_123"},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	report := analyzeSecretExposure([]corev1.Secret{}, pods)

	if report.Summary.ExposedEnvVars != 1 {
		t.Errorf("expected 1 exposed var, got %d", report.Summary.ExposedEnvVars)
	}
	if len(report.Exposed) != 1 {
		t.Fatalf("expected 1 exposure, got %d", len(report.Exposed))
	}
	if report.Exposed[0].Severity != "high" {
		t.Errorf("expected high severity for real password, got %s", report.Exposed[0].Severity)
	}
}

func TestAnalyzeSecretExposure_PlaceholderPassword(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "nginx",
						Env: []corev1.EnvVar{
							{Name: "API_KEY", Value: "your-api-key-here"},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	report := analyzeSecretExposure([]corev1.Secret{}, pods)

	if len(report.Exposed) != 1 {
		t.Fatalf("expected 1 exposure, got %d", len(report.Exposed))
	}
	if report.Exposed[0].Severity != "low" {
		t.Errorf("expected low severity for placeholder, got %s", report.Exposed[0].Severity)
	}
}

func TestAnalyzeSecretExposure_SecretKeyRefNotFlagged(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "nginx",
						Env: []corev1.EnvVar{
							{
								Name: "DB_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "db-secret"},
										Key:                  "password",
									},
								},
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	report := analyzeSecretExposure([]corev1.Secret{}, pods)

	if report.Summary.ExposedEnvVars != 0 {
		t.Errorf("secretKeyRef should not be flagged, got %d exposed", report.Summary.ExposedEnvVars)
	}
}

func TestAnalyzeSecretExposure_UnusedSecret(t *testing.T) {
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "unused-secret", Namespace: "default"},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"key": []byte("val")},
		},
	}

	report := analyzeSecretExposure(secrets, []corev1.Pod{})

	if report.Summary.UnusedSecrets != 1 {
		t.Errorf("expected 1 unused secret, got %d", report.Summary.UnusedSecrets)
	}
}

func TestAnalyzeSecretExposure_SystemNamespaceSkipped(t *testing.T) {
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "sys-secret", Namespace: "kube-system"},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"key": []byte("val")},
		},
	}

	report := analyzeSecretExposure(secrets, []corev1.Pod{})

	if report.Summary.TotalSecrets != 0 {
		t.Errorf("kube-system secrets should be skipped, got %d", report.Summary.TotalSecrets)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		dur      time.Duration
		expected string
	}{
		{30 * time.Minute, "30m"},
		{5 * time.Hour, "5h"},
		{100 * 24 * time.Hour, "100d"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.dur)
		if got != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.dur, got, tt.expected)
		}
	}
}

func TestSensitiveKeyPatterns(t *testing.T) {
	if len(sensitiveKeyPatterns) == 0 {
		t.Error("sensitiveKeyPatterns should not be empty")
	}

	// Verify common patterns exist
	found := false
	for _, p := range sensitiveKeyPatterns {
		if p == "password" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'password' in sensitiveKeyPatterns")
	}
}
