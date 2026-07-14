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

func TestObservabilityStack_NoBackends(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/observability-stack", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleObservabilityStack(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ObservabilityStackResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.MissingPillars != 3 {
		t.Errorf("expected 3 missing pillars, got %d", result.Summary.MissingPillars)
	}
	if result.HealthScore > 50 {
		t.Errorf("expected low health score with no backends, got %d", result.HealthScore)
	}
}

func TestObservabilityStack_WithPrometheus(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "monitoring"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-main-0", Namespace: "monitoring"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "prometheus", Image: "prom/prometheus:v2.45"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "node-exporter-abc", Namespace: "monitoring"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/observability-stack", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleObservabilityStack(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ObservabilityStackResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Should detect metrics pillar
	metricsHealthy := false
	for _, p := range result.Pillars {
		if p.Name == "metrics" && p.Status == "healthy" {
			metricsHealthy = true
		}
	}
	if !metricsHealthy {
		t.Errorf("expected metrics pillar to be healthy")
	}
	if result.Summary.MissingPillars != 2 {
		t.Errorf("expected 2 missing pillars (logging+tracing), got %d", result.Summary.MissingPillars)
	}
}

func TestObservabilityStack_DegradedBackend(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "monitoring"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-0", Namespace: "monitoring"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "prometheus", Image: "prom/prometheus:v2.45"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: false}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-1", Namespace: "monitoring"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "prometheus", Image: "prom/prometheus:v2.45"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/observability-stack", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleObservabilityStack(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ObservabilityStackResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.DegradedBackends == 0 {
		t.Errorf("expected at least 1 degraded backend")
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score with degraded backend, got %d", result.HealthScore)
	}
}

func TestObservabilityStack_FullStack(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "monitoring"}},
		// Metrics: Prometheus
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "prometheus-main-0", Namespace: "monitoring"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "prometheus", Image: "prom/prometheus:v2.45"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
		// Metrics agent: node-exporter
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "node-exporter-node1", Namespace: "monitoring"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Logging: Loki
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "loki-0", Namespace: "monitoring"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "loki", Image: "grafana/loki:2.9"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
		// Logging agent: fluent-bit
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "fluent-bit-node1", Namespace: "monitoring"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		// Tracing: Jaeger
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "jaeger-collector-0", Namespace: "monitoring"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "jaeger", Image: "jaegertracing/all-in-one:1.47"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/observability-stack", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleObservabilityStack(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ObservabilityStackResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HealthyPillars != 3 {
		t.Errorf("expected 3 healthy pillars, got %d", result.Summary.HealthyPillars)
	}
	if result.Summary.MissingPillars != 0 {
		t.Errorf("expected 0 missing pillars, got %d", result.Summary.MissingPillars)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score with full stack, got %d", result.HealthScore)
	}
}
