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

func TestRuntimeThreat_NoDetector(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "app-prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/runtime-threat", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeThreat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result RuntimeThreatResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalDetectors != 0 {
		t.Errorf("expected 0 detectors, got %d", result.Summary.TotalDetectors)
	}
	if result.HealthScore > 75 {
		t.Errorf("expected reduced health score without detectors, got %d", result.HealthScore)
	}
}

func TestRuntimeThreat_WithFalco(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "falco-system"}},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "falco", Namespace: "falco-system"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "falco", Image: "falcosecurity/falco:0.37"}},
					},
				},
			},
			Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/runtime-threat", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeThreat(rec, req)

	var result RuntimeThreatResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if !result.Summary.HasFalco {
		t.Error("expected Falco to be detected")
	}
	if result.Summary.TotalDetectors != 1 {
		t.Errorf("expected 1 detector, got %d", result.Summary.TotalDetectors)
	}
}

func TestRuntimeThreat_PrivilegedPod(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-priv-pod", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx", SecurityContext: &corev1.SecurityContext{Privileged: &privileged}},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/runtime-threat", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeThreat(rec, req)

	var result RuntimeThreatResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.PrivilegedPods != 1 {
		t.Errorf("expected 1 privileged pod, got %d", result.Summary.PrivilegedPods)
	}
	found := false
	for _, ap := range result.AnomalousPods {
		if ap.Severity == "high" {
			for _, a := range ap.Anomalies {
				if a == "Container c1 is privileged" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected to find privileged container anomaly with high severity")
	}
}

func TestRuntimeThreat_HighRestarts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-crash-pod", Namespace: "app-prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c1", RestartCount: 10},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/runtime-threat", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeThreat(rec, req)

	var result RuntimeThreatResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HighRestartPods != 1 {
		t.Errorf("expected 1 high restart pod, got %d", result.Summary.HighRestartPods)
	}
}

func TestRuntimeThreat_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/security/runtime-threat", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleRuntimeThreat(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result RuntimeThreatResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalDetectors != 0 {
		t.Errorf("expected 0 detectors, got %d", result.Summary.TotalDetectors)
	}
}
