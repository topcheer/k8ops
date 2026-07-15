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

func TestImageProvenance_AllTrusted(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName:   "node-1",
				Containers: []corev1.Container{{Name: "app", Image: "registry.iot2.win/myapp@sha256:abc123"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/image-provenance", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImageProvenance(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ImageProvenanceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalImages != 1 {
		t.Errorf("expected 1 image, got %d", result.Summary.TotalImages)
	}
	if result.Summary.WithDigest != 1 {
		t.Errorf("expected 1 with digest, got %d", result.Summary.WithDigest)
	}
	if result.Summary.TrustedRegistries != 1 {
		t.Errorf("expected 1 trusted, got %d", result.Summary.TrustedRegistries)
	}
}

func TestImageProvenance_LatestTag(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName:   "node-1",
				Containers: []corev1.Container{{Name: "app", Image: "nginx:latest"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/image-provenance", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImageProvenance(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ImageProvenanceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.LatestTag != 1 {
		t.Errorf("expected 1 latest tag, got %d", result.Summary.LatestTag)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestImageProvenance_MixedImages(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName:   "node-1",
				Containers: []corev1.Container{{Name: "app", Image: "gcr.io/myproject/app:v1.0"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-2", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName:   "node-1",
				Containers: []corev1.Container{{Name: "app", Image: "docker.io/library/redis:7.0"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/image-provenance", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImageProvenance(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ImageProvenanceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalImages != 2 {
		t.Errorf("expected 2 images, got %d", result.Summary.TotalImages)
	}
	if result.Summary.TrustedRegistries != 1 {
		t.Errorf("expected 1 trusted, got %d", result.Summary.TrustedRegistries)
	}
	if result.Summary.UntrustedRegistries != 1 {
		t.Errorf("expected 1 untrusted, got %d", result.Summary.UntrustedRegistries)
	}
}

func TestImageProvenance_NoPods(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/security/image-provenance", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleImageProvenance(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ImageProvenanceResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalImages != 0 {
		t.Errorf("expected 0 images, got %d", result.Summary.TotalImages)
	}
	if result.HealthScore < 80 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}
