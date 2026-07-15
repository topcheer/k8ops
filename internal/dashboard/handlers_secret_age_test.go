package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeSecretAge_Basic(t *testing.T) {
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "fresh", Namespace: "app", CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * 24 * time.Hour))},
			Type:       corev1.SecretTypeOpaque,
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "old", Namespace: "app", CreationTimestamp: metav1.NewTime(time.Now().Add(-200 * 24 * time.Hour))},
			Type:       corev1.SecretTypeOpaque,
		},
	}

	result := analyzeSecretAge(secrets, nil)

	if result.Summary.TotalSecrets != 2 {
		t.Errorf("expected 2 secrets, got %d", result.Summary.TotalSecrets)
	}
	if result.Summary.OlderThan180d != 1 {
		t.Errorf("expected 1 secret older than 180d, got %d", result.Summary.OlderThan180d)
	}
	if result.Summary.StaleCount != 1 {
		t.Errorf("expected 1 stale secret, got %d", result.Summary.StaleCount)
	}
}

func TestAnalyzeSecretAge_VeryOld(t *testing.T) {
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ancient", Namespace: "prod", CreationTimestamp: metav1.NewTime(time.Now().Add(-400 * 24 * time.Hour))},
			Type:       corev1.SecretTypeTLS,
		},
	}

	result := analyzeSecretAge(secrets, nil)

	if result.Summary.OlderThan365d != 1 {
		t.Errorf("expected 1 secret older than 365d, got %d", result.Summary.OlderThan365d)
	}
	if result.Summary.TLSSecrets != 1 {
		t.Errorf("expected 1 TLS secret, got %d", result.Summary.TLSSecrets)
	}
	if len(result.StaleSecrets) != 1 {
		t.Fatalf("expected 1 stale secret, got %d", len(result.StaleSecrets))
	}
	if result.StaleSecrets[0].Severity != "critical" {
		t.Errorf("expected severity critical, got %s", result.StaleSecrets[0].Severity)
	}
}

func TestAnalyzeSecretAge_OrphanedDetection(t *testing.T) {
	secrets := []corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "used", Namespace: "app", CreationTimestamp: metav1.Now()},
			Type:       corev1.SecretTypeOpaque,
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "unused", Namespace: "app", CreationTimestamp: metav1.Now()},
			Type:       corev1.SecretTypeOpaque,
		},
	}

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "app"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Env: []corev1.EnvVar{
							{Name: "SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "used"}}}},
						},
					},
				},
			},
		},
	}

	result := analyzeSecretAge(secrets, pods)

	if result.Summary.OrphanedCount != 1 {
		t.Errorf("expected 1 orphaned secret, got %d", result.Summary.OrphanedCount)
	}
	if len(result.OrphanedSecrets) != 1 {
		t.Fatalf("expected 1 orphaned secret entry, got %d", len(result.OrphanedSecrets))
	}
	if result.OrphanedSecrets[0].Name != "unused" {
		t.Errorf("expected orphaned secret to be 'unused', got %s", result.OrphanedSecrets[0].Name)
	}
}

func TestAnalyzeSecretAge_AgeBuckets(t *testing.T) {
	secrets := []corev1.Secret{
		{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "app", CreationTimestamp: metav1.NewTime(time.Now().Add(-3 * 24 * time.Hour))}, Type: corev1.SecretTypeOpaque},
		{ObjectMeta: metav1.ObjectMeta{Name: "s2", Namespace: "app", CreationTimestamp: metav1.NewTime(time.Now().Add(-20 * 24 * time.Hour))}, Type: corev1.SecretTypeOpaque},
		{ObjectMeta: metav1.ObjectMeta{Name: "s3", Namespace: "app", CreationTimestamp: metav1.NewTime(time.Now().Add(-60 * 24 * time.Hour))}, Type: corev1.SecretTypeOpaque},
		{ObjectMeta: metav1.ObjectMeta{Name: "s4", Namespace: "app", CreationTimestamp: metav1.NewTime(time.Now().Add(-120 * 24 * time.Hour))}, Type: corev1.SecretTypeOpaque},
	}

	result := analyzeSecretAge(secrets, nil)

	if len(result.ByAge) != 6 {
		t.Fatalf("expected 6 age buckets, got %d", len(result.ByAge))
	}
	bucketMap := make(map[string]int)
	for _, b := range result.ByAge {
		bucketMap[b.Range] = b.Count
	}
	if bucketMap["<7d"] != 1 {
		t.Errorf("expected 1 secret <7d, got %d", bucketMap["<7d"])
	}
	if bucketMap["7-30d"] != 1 {
		t.Errorf("expected 1 secret 7-30d, got %d", bucketMap["7-30d"])
	}
	if bucketMap["30-90d"] != 1 {
		t.Errorf("expected 1 secret 30-90d, got %d", bucketMap["30-90d"])
	}
	if bucketMap["90-180d"] != 1 {
		t.Errorf("expected 1 secret 90-180d, got %d", bucketMap["90-180d"])
	}
}
