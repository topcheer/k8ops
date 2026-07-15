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

func TestPortExposure_NoPorts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName:   "node-1",
				Containers: []corev1.Container{{Name: "app", Image: "nginx"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/port-exposure", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePortExposure(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result PortExposureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.TotalContainers != 1 {
		t.Errorf("expected 1 container, got %d", result.Summary.TotalContainers)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestPortExposure_HostPortConflict(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "web-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name:  "web",
					Image: "nginx",
					Ports: []corev1.ContainerPort{{ContainerPort: 80, HostPort: 8080, Protocol: corev1.ProtocolTCP}},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-xyz", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name:  "api",
					Image: "api-server",
					Ports: []corev1.ContainerPort{{ContainerPort: 8080, HostPort: 8080, Protocol: corev1.ProtocolTCP}},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/port-exposure", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePortExposure(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result PortExposureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithHostPort != 2 {
		t.Errorf("expected 2 containers with hostPort, got %d", result.Summary.WithHostPort)
	}
	if result.Summary.HostPortConflicts < 1 {
		t.Errorf("expected at least 1 host port conflict, got %d", result.Summary.HostPortConflicts)
	}
	if result.HealthScore >= 80 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestPortExposure_NamedPorts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name:  "app",
					Image: "myapp",
					Ports: []corev1.ContainerPort{
						{Name: "http", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
						{Name: "metrics", ContainerPort: 9090, Protocol: corev1.ProtocolTCP},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/port-exposure", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePortExposure(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result PortExposureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithNamedPorts != 1 {
		t.Errorf("expected 1 container with named ports, got %d", result.Summary.WithNamedPorts)
	}
	if result.Summary.WithUnnamedPorts != 0 {
		t.Errorf("expected 0 unnamed ports, got %d", result.Summary.WithUnnamedPorts)
	}
}

func TestPortExposure_UnnamedPorts(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-abc", Namespace: "default"},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{{
					Name:  "app",
					Image: "myapp",
					Ports: []corev1.ContainerPort{
						{ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
					},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/product/port-exposure", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePortExposure(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result PortExposureResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.WithUnnamedPorts != 1 {
		t.Errorf("expected 1 container with unnamed ports, got %d", result.Summary.WithUnnamedPorts)
	}
	if result.Summary.WithNamedPorts != 0 {
		t.Errorf("expected 0 named ports, got %d", result.Summary.WithNamedPorts)
	}
}
