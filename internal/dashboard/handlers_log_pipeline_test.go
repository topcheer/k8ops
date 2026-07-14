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

func TestLogPipeline_NoCollectors(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/log-pipeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleLogPipeline(rec, req)

	var result LogPipelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalCollectors != 0 {
		t.Errorf("expected 0 collectors, got %d", result.Summary.TotalCollectors)
	}
	if result.HealthScore > 30 {
		t.Errorf("expected low health score for no collectors, got %d", result.HealthScore)
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected recommendations for missing collectors")
	}
}

func TestLogPipeline_WithFluentBit(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "logging"}},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "fluent-bit", Namespace: "logging"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "fluent-bit", Image: "fluent/fluent-bit:3.0"},
						},
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 5,
				NumberReady:            5,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "loki", Namespace: "logging"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "loki", Image: "grafana/loki:2.8"},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 3, AvailableReplicas: 3},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "fluent-bit-config", Namespace: "logging"},
			Data: map[string]string{
				"fluent-bit.conf": "[OUTPUT]\n    Name loki\n    Host loki\n",
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/log-pipeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleLogPipeline(rec, req)

	var result LogPipelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.HasFluentBit {
		t.Error("expected fluentbit collector to be detected")
	}
	if result.Summary.TotalCollectors != 1 {
		t.Errorf("expected 1 collector, got %d", result.Summary.TotalCollectors)
	}
	if result.Summary.ReadyCollectors != 1 {
		t.Errorf("expected 1 ready collector, got %d", result.Summary.ReadyCollectors)
	}
	if len(result.StorageBackends) == 0 {
		t.Error("expected at least one storage backend (loki)")
	}
	if len(result.Forwarders) == 0 {
		t.Error("expected at least one forwarder config")
	}
	if result.HealthScore < 70 {
		t.Errorf("expected high health score with fluent-bit + loki, got %d", result.HealthScore)
	}
}

func TestLogPipeline_UnhealthyCollector(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "fluent-bit", Namespace: "logging"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "fluent-bit", Image: "fluent/fluent-bit:3.0"},
						},
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 5,
				NumberReady:            2,
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/log-pipeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleLogPipeline(rec, req)

	var result LogPipelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.UnhealthyCollectors != 1 {
		t.Errorf("expected 1 unhealthy collector, got %d", result.Summary.UnhealthyCollectors)
	}
	// Check collector status
	if len(result.Collectors) > 0 && result.Collectors[0].Status != "degraded" {
		t.Errorf("expected degraded status, got %s", result.Collectors[0].Status)
	}
}

func TestLogPipeline_VectorCollector(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "vector", Namespace: "observability"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "vector", Image: "timberio/vector:0.35"},
						},
					},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/log-pipeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleLogPipeline(rec, req)

	var result LogPipelineResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.HasVector {
		t.Error("expected vector collector to be detected")
	}
}

func TestLogPipeline_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/operations/log-pipeline", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleLogPipeline(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result LogPipelineResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.HealthScore > 30 {
		t.Errorf("expected low health score for empty cluster, got %d", result.HealthScore)
	}
}
