package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestDeployTraceability_NoMetadata(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:latest"}},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/traceability", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployTraceability(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result DeployTraceabilityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalWorkloads != 1 {
		t.Errorf("expected 1 workload, got %d", result.Summary.TotalWorkloads)
	}
	if result.Summary.WithNoTrace != 1 {
		t.Errorf("expected 1 with no trace, got %d", result.Summary.WithNoTrace)
	}
	if result.HealthScore >= 50 {
		t.Errorf("expected low health score, got %d", result.HealthScore)
	}
}

func TestDeployTraceability_FullTraceability(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/version":    "v1.2.3",
							"app.kubernetes.io/managed-by": "helm",
							"app.kubernetes.io/part-of":    "myapp",
						},
						Annotations: map[string]string{
							"app.kubernetes.io/git-commit": "abc1234",
							"build-timestamp":              "2026-07-14T12:00:00Z",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "registry.io/app@sha256:abc123def456"},
						},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/traceability", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployTraceability(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result DeployTraceabilityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithFullTrace != 1 {
		t.Errorf("expected 1 with full trace, got %d", result.Summary.WithFullTrace)
	}
	if result.Summary.WithNoTrace != 0 {
		t.Errorf("expected 0 with no trace, got %d", result.Summary.WithNoTrace)
	}
	if result.Summary.HasImageDigest != 1 {
		t.Errorf("expected 1 with image digest, got %d", result.Summary.HasImageDigest)
	}
	if result.HealthScore < 80 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestDeployTraceability_PartialTraceability(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app1", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/version": "v1.0",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
					},
				},
			},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
			Spec: appsv1.StatefulSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"git-commit": "def5678",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "db", Image: "mysql:8.0"}},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/traceability", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployTraceability(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result DeployTraceabilityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalWorkloads != 2 {
		t.Errorf("expected 2 workloads, got %d", result.Summary.TotalWorkloads)
	}
	if result.Summary.HasVersionLabel != 1 {
		t.Errorf("expected 1 with version label, got %d", result.Summary.HasVersionLabel)
	}
	if result.Summary.HasGitCommit != 1 {
		t.Errorf("expected 1 with git commit, got %d", result.Summary.HasGitCommit)
	}
	if result.Summary.WithFullTrace != 0 {
		t.Errorf("expected 0 with full trace, got %d", result.Summary.WithFullTrace)
	}
	if len(result.LowTraceability) < 1 {
		t.Errorf("expected at least 1 low traceability workload, got %d", len(result.LowTraceability))
	}
}

func TestDeployTraceability_StatefulSetWithDigest(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "cache"},
			Spec: appsv1.StatefulSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app.kubernetes.io/version":    "v7.0",
							"app.kubernetes.io/managed-by": "kustomize",
							"app.kubernetes.io/part-of":    "cache-stack",
						},
						Annotations: map[string]string{
							"app.kubernetes.io/git-commit": "xyz999",
							"build-timestamp":              "2026-07-14T10:00:00Z",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "redis", Image: "redis@sha256:def456"},
						},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/traceability", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployTraceability(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result DeployTraceabilityResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.StatefulSets != 1 {
		t.Errorf("expected 1 statefulset, got %d", result.Summary.StatefulSets)
	}
	if result.Summary.WithFullTrace != 1 {
		t.Errorf("expected 1 with full trace, got %d", result.Summary.WithFullTrace)
	}
}
