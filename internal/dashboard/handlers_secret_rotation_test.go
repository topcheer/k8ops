package dashboard

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildReferencedSecretSet(t *testing.T) {
	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "secret-vol",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: "my-secret"},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "c1",
							Env: []corev1.EnvVar{
								{
									Name: "PASS",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{Name: "db-pass"},
											Key:                  "password",
										},
									},
								},
							},
						},
					},
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: "registry-cred"},
					},
				},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
		},
	}

	refSet := buildReferencedSecretSet(pods)

	if refSet["default/my-secret"] != 1 {
		t.Errorf("Expected my-secret referenced once, got %d", refSet["default/my-secret"])
	}
	if refSet["default/db-pass"] != 1 {
		t.Errorf("Expected db-pass referenced once, got %d", refSet["default/db-pass"])
	}
	if refSet["default/registry-cred"] != 1 {
		t.Errorf("Expected registry-cred referenced once, got %d", refSet["default/registry-cred"])
	}
	if refSet["default/unreferenced"] != 0 {
		t.Error("Unreferenced secret should not be in set")
	}
}

func TestBuildReferencedSecretSetNilPods(t *testing.T) {
	refSet := buildReferencedSecretSet(nil)
	if len(refSet) != 0 {
		t.Errorf("Expected empty refSet for nil pods, got %d", len(refSet))
	}
}

func TestCheckTLSExpiry(t *testing.T) {
	// Generate a self-signed cert expiring in 10 days
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().Add(10 * 24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	secret := &corev1.Secret{
		Data: map[string][]byte{
			corev1.TLSCertKey: certPEM,
		},
	}

	days, expired, expiryStr := checkTLSExpiry(secret, time.Now())

	if expired {
		t.Error("Cert should not be expired")
	}
	if days < 9 || days > 10 {
		t.Errorf("Expected ~10 days to expiry, got %d", days)
	}
	if expiryStr == "" {
		t.Error("Expected non-empty expiry string")
	}
}

func TestCheckTLSExpiryExpired(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "expired"},
		NotBefore:    time.Now().Add(-30 * 24 * time.Hour),
		NotAfter:     time.Now().Add(-10 * 24 * time.Hour),
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	secret := &corev1.Secret{
		Data: map[string][]byte{
			corev1.TLSCertKey: certPEM,
		},
	}

	days, expired, _ := checkTLSExpiry(secret, time.Now())

	if !expired {
		t.Error("Cert should be expired")
	}
	if days >= 0 {
		t.Errorf("Expected negative days, got %d", days)
	}
}

func TestCheckTLSExpiryInvalidData(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{
			corev1.TLSCertKey: []byte("not-a-cert"),
		},
	}

	days, expired, expiryStr := checkTLSExpiry(secret, time.Now())

	if expired {
		t.Error("Should not be expired for invalid data")
	}
	if days != 0 {
		t.Errorf("Expected 0 days for invalid data, got %d", days)
	}
	if expiryStr != "" {
		t.Error("Expected empty expiry string for invalid data")
	}
}

func TestCheckTLSExpiryNoCertKey(t *testing.T) {
	secret := &corev1.Secret{
		Data: map[string][]byte{},
	}

	days, expired, expiryStr := checkTLSExpiry(secret, time.Now())

	if expired {
		t.Error("Should not be expired when no cert key")
	}
	if days != 0 {
		t.Errorf("Expected 0 days, got %d", days)
	}
	if expiryStr != "" {
		t.Error("Expected empty expiry string")
	}
}

func TestAssessSecretRisk(t *testing.T) {
	tests := []struct {
		name   string
		entry  SecretAuditEntry
		expect string
	}{
		{
			"expired-tls",
			SecretAuditEntry{TLSExpired: true},
			"critical",
		},
		{
			"very-stale-docker",
			SecretAuditEntry{IsVeryStale: true, IsDockerSecret: true},
			"critical",
		},
		{
			"expiring-tls",
			SecretAuditEntry{HasTLSExpiry: true, TLSDaysToExp: 15, TLSExpired: false},
			"high",
		},
		{
			"stale-unused-sensitive",
			SecretAuditEntry{IsStale: true, IsUnused: true, SensitiveName: true},
			"high",
		},
		{
			"stale-docker",
			SecretAuditEntry{IsStale: true, IsDockerSecret: true},
			"medium",
		},
		{
			"unused-sensitive",
			SecretAuditEntry{IsUnused: true, SensitiveName: true},
			"medium",
		},
		{
			"stale-in-use",
			SecretAuditEntry{IsStale: true, ReferencedBy: 3},
			"low",
		},
		{
			"healthy",
			SecretAuditEntry{AgeDays: 5, ReferencedBy: 2},
			"low",
		},
	}

	for _, tt := range tests {
		got := assessSecretRisk(tt.entry)
		if got != tt.expect {
			t.Errorf("assessSecretRisk(%s) = %q, want %q", tt.name, got, tt.expect)
		}
	}
}

func TestCalculateRotationScore(t *testing.T) {
	// Perfect score
	perfect := SecretRotationSummary{TotalSecrets: 10}
	if score := calculateRotationScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// With issues
	withIssues := SecretRotationSummary{
		TotalSecrets:     10,
		ExpiredTLS:       1, // -15
		ExpiringTLS:      2, // -10
		VeryStaleSecrets: 2, // -6
		StaleSecrets:     5, // includes 2 very stale, so 3 more at -1 = -3
		UnusedSecrets:    2, // -4
	}
	// 100 - 15 - 10 - 6 - 3 - 4 = 62
	score := calculateRotationScore(withIssues)
	if score != 62 {
		t.Errorf("Expected 62, got %d", score)
	}

	// Floor at 0
	terrible := SecretRotationSummary{
		TotalSecrets: 10,
		ExpiredTLS:   10, // -150
	}
	if score := calculateRotationScore(terrible); score != 0 {
		t.Errorf("Expected 0 for terrible, got %d", score)
	}

	// No secrets = 100
	empty := SecretRotationSummary{}
	if score := calculateRotationScore(empty); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}
}

func TestGenerateSecretRecommendations(t *testing.T) {
	result := SecretRotationResult{
		Summary: SecretRotationSummary{
			ExpiredTLS:       1,
			ExpiringTLS:      2,
			VeryStaleSecrets: 3,
			UnusedSecrets:    4,
			SATokens:         1,
			RotationScore:    40,
		},
		ByNamespace: []SecretNsStat{
			{Namespace: "kube-system", Stale: 5, Total: 10},
		},
	}

	recs := generateSecretRecommendations(result)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundExpired := false
	foundScore := false
	for _, r := range recs {
		if containsSubstr(r, "ALREADY EXPIRED") {
			foundExpired = true
		}
		if containsSubstr(r, "rotation score") {
			foundScore = true
		}
	}
	if !foundExpired {
		t.Error("Expected recommendation about expired TLS")
	}
	if !foundScore {
		t.Error("Expected recommendation about rotation score")
	}
}

func TestGenerateSecretRecommendationsClean(t *testing.T) {
	result := SecretRotationResult{
		Summary: SecretRotationSummary{
			TotalSecrets:  10,
			RotationScore: 100,
		},
	}

	recs := generateSecretRecommendations(result)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean cluster, got %d", len(recs))
	}
}

func TestSecretRiskRank(t *testing.T) {
	if secretRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if secretRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if secretRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if secretRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}

func TestGetOrCreateSecretNs(t *testing.T) {
	m := make(map[string]*SecretNsStat)

	e1 := getOrCreateSecretNs(m, "default")
	e1.Total = 5

	e2 := getOrCreateSecretNs(m, "default")
	if e2.Total != 5 {
		t.Errorf("Expected same entry with Total=5, got %d", e2.Total)
	}

	e3 := getOrCreateSecretNs(m, "kube-system")
	if e3.Namespace != "kube-system" {
		t.Errorf("Expected kube-system, got %s", e3.Namespace)
	}
}
