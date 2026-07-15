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

func TestRestartStorm_NoRestarts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Ready: true, RestartCount: 0}},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/restart-storm", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRestartStorm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RestartStormResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.PodsWithRestarts != 0 {
		t.Errorf("expected 0 pods with restarts, got %d", result.Summary.PodsWithRestarts)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestRestartStorm_HighRestarts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "myapp:v1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Ready: false, RestartCount: 10},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/restart-storm", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRestartStorm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RestartStormResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HighRestartPods != 1 {
		t.Errorf("expected 1 high restart pod, got %d", result.Summary.HighRestartPods)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestRestartStorm_Clustering(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-1", Namespace: "production"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "myapp:v2"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 5}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-2", Namespace: "production"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "myapp:v2"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 8}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-3", Namespace: "production"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "myapp:v2"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 3}}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/restart-storm", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRestartStorm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RestartStormResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.ClusteringDetected {
		t.Error("expected clustering to be detected")
	}
	if result.Summary.AffectedNS != 1 {
		t.Errorf("expected 1 affected namespace, got %d", result.Summary.AffectedNS)
	}
}

func TestRestartStorm_SameImageCascade(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-a", Namespace: "ns1"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "svc", Image: "bad-image:v3"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 4}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-b", Namespace: "ns2"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "svc", Image: "bad-image:v3"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 5}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-c", Namespace: "ns3"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "svc", Image: "bad-image:v3"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 6}}},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/restart-storm", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRestartStorm(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RestartStormResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	found := false
	for _, cp := range result.CascadePatterns {
		if cp.Pattern == "same-image" {
			found = true
		}
	}
	if !found {
		t.Error("expected same-image cascade pattern to be detected")
	}
}
