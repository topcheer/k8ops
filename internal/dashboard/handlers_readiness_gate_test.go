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

func TestReadinessGate_NoGates(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Ready: true}}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/readiness-gate", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleReadinessGate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ReadinessGateResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithoutGates != 1 {
		t.Errorf("expected 1 without gates, got %d", result.Summary.WithoutGates)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestReadinessGate_WithGates(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
				ReadinessGates: []corev1.PodReadinessGate{
					{ConditionType: "myapp.example.com/ready"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: "myapp.example.com/ready", Status: corev1.ConditionTrue},
				},
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/readiness-gate", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleReadinessGate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ReadinessGateResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithReadinessGates != 1 {
		t.Errorf("expected 1 with gates, got %d", result.Summary.WithReadinessGates)
	}
	if len(result.WithGates) != 1 {
		t.Errorf("expected 1 gate entry, got %d", len(result.WithGates))
	}
}

func TestReadinessGate_BlockedByGate(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
				ReadinessGates: []corev1.PodReadinessGate{
					{ConditionType: "myapp.example.com/ready"},
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: "myapp.example.com/ready", Status: corev1.ConditionFalse, Reason: "NotReady"},
				},
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/readiness-gate", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleReadinessGate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ReadinessGateResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.GateBlockedPods != 1 {
		t.Errorf("expected 1 gate blocked pod, got %d", result.Summary.GateBlockedPods)
	}
	if len(result.BlockedPods) != 1 {
		t.Errorf("expected 1 blocked pod entry, got %d", len(result.BlockedPods))
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestReadinessGate_UnknownGate(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
				ReadinessGates: []corev1.PodReadinessGate{
					{ConditionType: "missing.example.com/ready"},
				},
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				Conditions:        []corev1.PodCondition{},
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/readiness-gate", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleReadinessGate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result ReadinessGateResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if len(result.BlockedPods) != 1 {
		t.Errorf("expected 1 blocked pod, got %d", len(result.BlockedPods))
	}
	if result.BlockedPods[0].Status != "Unknown" {
		t.Errorf("expected Unknown status, got %s", result.BlockedPods[0].Status)
	}
}
