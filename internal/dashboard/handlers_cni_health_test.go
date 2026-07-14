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

func TestCNIHealth_NoCNI(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/cni-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCNIHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result CNIHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.CNIType != "" {
		t.Errorf("expected no CNI type, got %s", result.Summary.CNIType)
	}
	if result.HealthScore > 75 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestCNIHealth_WithCalico(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Spec:       corev1.NodeSpec{PodCIDR: "10.244.1.0/24"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "calico-node-abc", Namespace: "kube-system"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "calico", Image: "docker.io/calico/node:v3.26"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/cni-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCNIHealth(rec, req)

	var result CNIHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.CNIType != "calico" {
		t.Errorf("expected calico, got %s", result.Summary.CNIType)
	}
	if result.Summary.NodesWithCNI != 1 {
		t.Errorf("expected 1 node with CNI, got %d", result.Summary.NodesWithCNI)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score with calico, got %d", result.HealthScore)
	}
}

func TestCNIHealth_NetworkUnavailable(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-bad"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeNetworkUnavailable, Status: corev1.ConditionTrue},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/cni-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCNIHealth(rec, req)

	var result CNIHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NetworkNotReady != 1 {
		t.Errorf("expected 1 network not ready, got %d", result.Summary.NetworkNotReady)
	}
}

func TestCNIHealth_NoPodCIDR(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-no-cidr"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/operations/cni-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCNIHealth(rec, req)

	var result CNIHealthResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NodesWithoutCNI != 1 {
		t.Errorf("expected 1 node without CNI, got %d", result.Summary.NodesWithoutCNI)
	}
}

func TestCNIHealth_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/operations/cni-health", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleCNIHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result CNIHealthResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalNodes != 0 {
		t.Errorf("expected 0 nodes, got %d", result.Summary.TotalNodes)
	}
}
