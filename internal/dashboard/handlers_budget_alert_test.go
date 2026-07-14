package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestBudgetAlert_NoBudget(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "c1", Image: "nginx",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			}}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/budget-alert", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleBudgetAlert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result BudgetAlertResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSWithoutBudget != 1 {
		t.Errorf("expected 1 namespace without budget, got %d", result.Summary.NSWithoutBudget)
	}
}

func TestBudgetAlert_OverBudget(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "app-prod",
				Annotations: map[string]string{"k8ops.io/monthly-budget": "10.0"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "c1", Image: "nginx",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4000m"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
			}}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/budget-alert", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleBudgetAlert(rec, req)

	var result BudgetAlertResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSOverBudget != 1 {
		t.Errorf("expected 1 over budget, got %d", result.Summary.NSOverBudget)
	}
}

func TestBudgetAlert_WithinBudget(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "app-prod",
				Annotations: map[string]string{"k8ops.io/monthly-budget": "1000.0"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "c1", Image: "nginx",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			}}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/budget-alert", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleBudgetAlert(rec, req)

	var result BudgetAlertResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.HealthScore < 90 {
		t.Errorf("expected high health score within budget, got %d", result.HealthScore)
	}
}

func TestBudgetAlert_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/scalability/budget-alert", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleBudgetAlert(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result BudgetAlertResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalNamespaces != 0 {
		t.Errorf("expected 0 namespaces, got %d", result.Summary.TotalNamespaces)
	}
}
