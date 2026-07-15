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

func TestTerminationAudit_NoTerminations(t *testing.T) {
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

	req := newReqWithClients(http.MethodGet, "/api/deployment/termination-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleTerminationAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result TerminationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TerminatedPods != 0 {
		t.Errorf("expected 0 terminated pods, got %d", result.Summary.TerminatedPods)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestTerminationAudit_OOMKilled(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "app",
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 137,
								Reason:   "OOMKilled",
								Message:  "container exceeded memory limit",
							},
						},
						RestartCount: 3,
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/termination-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleTerminationAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result TerminationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.OOMKilledCount != 1 {
		t.Errorf("expected 1 OOMKilled, got %d", result.Summary.OOMKilledCount)
	}
	if len(result.OOMKilledPods) != 1 {
		t.Errorf("expected 1 OOMKilled pod entry, got %d", len(result.OOMKilledPods))
	}
	if result.HealthScore >= 95 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestTerminationAudit_NonZeroExit(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-xyz", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "worker", Image: "worker"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "worker",
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 1,
								Reason:   "Error",
								Message:  "connection refused",
							},
						},
						RestartCount: 2,
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/termination-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleTerminationAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result TerminationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NonZeroExitCount != 1 {
		t.Errorf("expected 1 non-zero exit, got %d", result.Summary.NonZeroExitCount)
	}
	if result.Summary.WithTermMsg != 1 {
		t.Errorf("expected 1 with term message, got %d", result.Summary.WithTermMsg)
	}
}

func TestTerminationAudit_HighRestarts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "crashloop-pod", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "app"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "app",
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 143,
								Signal:   15,
								Reason:   "Terminated",
							},
						},
						RestartCount: 8,
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/termination-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleTerminationAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result TerminationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HighRestartCount != 1 {
		t.Errorf("expected 1 high restart pod, got %d", result.Summary.HighRestartCount)
	}
	if result.Summary.SignalKilled != 1 {
		t.Errorf("expected 1 signal killed, got %d", result.Summary.SignalKilled)
	}
}
