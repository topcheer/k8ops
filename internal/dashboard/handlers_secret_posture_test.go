package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestSecretPosture_NoExternalTools(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "db-password", Namespace: "default"},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"password": []byte("super-secret")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "tls-cert", Namespace: "default"},
			Type:       corev1.SecretTypeTLS,
			Data:       map[string][]byte{"tls.crt": []byte("cert"), "tls.key": []byte("key")},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/secret-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSecretPosture(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SecretPostureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Integration.Status != "missing" {
		t.Errorf("expected missing integration, got %s", result.Integration.Status)
	}
	if result.Summary.TotalSecrets != 2 {
		t.Errorf("expected 2 secrets, got %d", result.Summary.TotalSecrets)
	}
	if result.Summary.UnmanagedSecrets < 1 {
		t.Errorf("expected at least 1 unmanaged secret, got %d", result.Summary.UnmanagedSecrets)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score without external tools, got %d", result.HealthScore)
	}
}

func TestSecretPosture_WithExternalSecrets(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "external-secrets"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "external-secrets-controller-0", Namespace: "external-secrets"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "controller", Image: "ghcr.io/external-secrets/external-secrets:v0.9.4"},
			}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "managed-secret", Namespace: "default",
				Annotations: map[string]string{"external-secrets.io/managed": "true"},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{"key": []byte("value")},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/secret-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSecretPosture(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SecretPostureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Integration.ExternalSecretsOperator {
		t.Error("expected External Secrets Operator to be detected")
	}
	if result.Integration.Status == "missing" {
		t.Error("expected non-missing integration status")
	}
}

func TestSecretPosture_WithSealedSecrets(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "sealed-secrets-controller-abc", Namespace: "kube-system"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "controller", Image: "bitnami/sealed-secrets:v0.27.0"},
			}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sealed-secret", Namespace: "kube-system",
				Labels: map[string]string{"sealedsecrets.bitnami.com/sealed": "true"},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{"key": []byte("encrypted-value")},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/secret-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSecretPosture(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SecretPostureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Integration.SealedSecretsController {
		t.Error("expected Sealed Secrets controller to be detected")
	}
	if result.Summary.SealedSecrets < 1 {
		t.Errorf("expected at least 1 sealed secret, got %d", result.Summary.SealedSecrets)
	}
}

func TestSecretPosture_EmptySecrets(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-secret", Namespace: "default"},
			Type:       corev1.SecretTypeOpaque,
			Data:       nil,
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "big-secret", Namespace: "default"},
			Type:       corev1.SecretTypeOpaque,
			Data:       map[string][]byte{"data": make([]byte, 2*1024*1024)}, // 2MB
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/secret-posture", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSecretPosture(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SecretPostureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.EmptySecrets != 1 {
		t.Errorf("expected 1 empty secret, got %d", result.Summary.EmptySecrets)
	}
	if result.Summary.LargeSecrets != 1 {
		t.Errorf("expected 1 large secret, got %d", result.Summary.LargeSecrets)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}
