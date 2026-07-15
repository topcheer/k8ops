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

func TestOperatorHealth_NoOperators(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-app-abc", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: "nginx"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/operator-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleOperatorHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result OperatorHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalOperators != 0 {
		t.Errorf("expected 0 operators, got %d", result.Summary.TotalOperators)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score with no operators, got %d", result.HealthScore)
	}
}

func TestOperatorHealth_HealthyOperator(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-operator-system"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-operator-controller-manager-abc123", Namespace: "my-operator-system",
				Labels: map[string]string{"control-plane": "controller-manager"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "manager", Image: "myoperator/manager:v1.0"},
			}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true, RestartCount: 0}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/operator-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleOperatorHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result OperatorHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalOperators < 1 {
		t.Errorf("expected at least 1 operator, got %d", result.Summary.TotalOperators)
	}
	if result.Summary.HealthyOperators < 1 {
		t.Errorf("expected at least 1 healthy operator, got %d", result.Summary.HealthyOperators)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestOperatorHealth_FailingOperator(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "my-operator-system"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "my-operator-controller-xyz789", Namespace: "my-operator-system"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "manager", Image: "myoperator/manager:v1.0"},
			}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Ready: false, RestartCount: 8, State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					}},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/operator-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleOperatorHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result OperatorHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.FailedOperators != 1 {
		t.Errorf("expected 1 failed operator, got %d", result.Summary.FailedOperators)
	}
	if result.HealthScore >= 80 {
		t.Errorf("expected reduced health score with failed operator, got %d", result.HealthScore)
	}
}

func TestOperatorHealth_OLMDetected(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "olm-system"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "olm-operator-abc", Namespace: "olm-system"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "olm", Image: "quay.io/operator-framework/olm:v0.28"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true, RestartCount: 0}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/operator-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleOperatorHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result OperatorHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.OLMDetected {
		t.Error("expected OLM to be detected")
	}
	if len(result.Summary.OLMNamespaces) == 0 {
		t.Error("expected at least 1 OLM namespace")
	}
}
