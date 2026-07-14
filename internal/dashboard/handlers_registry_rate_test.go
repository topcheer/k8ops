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

func TestRegistryRateLimit_DockerHubNoAuth(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-dockerhub", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25"}, // docker.io, no pull secrets
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/registry-rate-limit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRegistryRateLimit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RegistryRateLimitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.UsingDockerHub < 1 {
		t.Errorf("expected at least 1 Docker Hub image, got %d", result.Summary.UsingDockerHub)
	}
	if result.Summary.RateLimitRisk != 1 {
		t.Errorf("expected 1 rate limit risk, got %d", result.Summary.RateLimitRisk)
	}
}

func TestRegistryRateLimit_PrivateRegistryWithSecret(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-private", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}},
				Containers: []corev1.Container{
					{Name: "c1", Image: "registry.iot2.win/app:v1.0"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/registry-rate-limit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRegistryRateLimit(rec, req)

	var result RegistryRateLimitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.UsingPrivate < 1 {
		t.Errorf("expected at least 1 private registry image, got %d", result.Summary.UsingPrivate)
	}
	if result.Summary.RateLimitRisk != 0 {
		t.Errorf("expected 0 rate limit risk, got %d", result.Summary.RateLimitRisk)
	}
}

func TestRegistryRateLimit_PublicRegistry(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-quay", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "quay.io/prometheus/node-exporter:v1.6"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/registry-rate-limit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRegistryRateLimit(rec, req)

	var result RegistryRateLimitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.UsingPublic < 1 {
		t.Errorf("expected at least 1 public registry, got %d", result.Summary.UsingPublic)
	}
	if result.Summary.RateLimitRisk != 0 {
		t.Errorf("expected 0 rate limit risk for quay.io, got %d", result.Summary.RateLimitRisk)
	}
}

func TestRegistryRateLimit_DockerHubWithAuth(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-authed", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "dockerhub-secret"}},
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25"},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/registry-rate-limit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRegistryRateLimit(rec, req)

	var result RegistryRateLimitResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Docker Hub with auth should not be rate limit risk
	if result.Summary.RateLimitRisk != 0 {
		t.Errorf("expected 0 rate limit risk with auth, got %d", result.Summary.RateLimitRisk)
	}
}

func TestRegistryRateLimit_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/operations/registry-rate-limit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRegistryRateLimit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result RegistryRateLimitResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalImages != 0 {
		t.Errorf("expected 0 images, got %d", result.Summary.TotalImages)
	}
}
