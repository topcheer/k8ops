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

func TestEnvConfigDrift_MissingConfigMap(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-deploy", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "c1", Image: "nginx",
							EnvFrom: []corev1.EnvFromSource{{
								ConfigMapRef: &corev1.ConfigMapEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: "missing-cm"},
								},
							}},
						}},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/env-config-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEnvConfigDrift(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result EnvConfigDriftResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.MissingRefs != 1 {
		t.Errorf("expected 1 missing ref, got %d", result.Summary.MissingRefs)
	}
}

func TestEnvConfigDrift_HardcodedSecret(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-deploy", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "c1", Image: "nginx",
							Env: []corev1.EnvVar{
								{Name: "DATABASE_PASSWORD", Value: "test-value-here"},
							},
						}},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/env-config-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEnvConfigDrift(rec, req)

	var result EnvConfigDriftResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HardcodedSecrets < 1 {
		t.Errorf("expected at least 1 hardcoded secret, got %d", result.Summary.HardcodedSecrets)
	}
}

func TestEnvConfigDrift_ValidRefs(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "default"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "app-secret", Namespace: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-deploy", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "c1", Image: "nginx",
							EnvFrom: []corev1.EnvFromSource{
								{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"}}},
								{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "app-secret"}}},
							},
						}},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/env-config-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEnvConfigDrift(rec, req)

	var result EnvConfigDriftResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.MissingRefs != 0 {
		t.Errorf("expected 0 missing refs, got %d", result.Summary.MissingRefs)
	}
	if result.HealthScore < 95 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestEnvConfigDrift_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/deployment/env-config-drift", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleEnvConfigDrift(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result EnvConfigDriftResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalDeployments != 0 {
		t.Errorf("expected 0 deployments, got %d", result.Summary.TotalDeployments)
	}
}
