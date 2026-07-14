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

func TestNodeDrainReadiness_SafeToDrain(t *testing.T) {
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
		// Deployment pod (movable)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-abc", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "app-rs"},
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/node-drain-readiness", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeDrainReadiness(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NodeDrainReadinessResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.SafeToDrain != 1 {
		t.Errorf("expected 1 safe-to-drain node, got %d", result.Summary.SafeToDrain)
	}
	if result.HealthScore < 80 {
		t.Errorf("expected high health score, got %d", result.HealthScore)
	}
}

func TestNodeDrainReadiness_StatefulSet(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "mysql-0", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "StatefulSet", Name: "mysql"},
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-1"},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/node-drain-readiness", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeDrainReadiness(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NodeDrainReadinessResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.StatefulPodsOnNode != 1 {
		t.Errorf("expected 1 stateful pod, got %d", result.Summary.StatefulPodsOnNode)
	}
	if result.Summary.RiskyToDrain != 1 {
		t.Errorf("expected 1 risky-to-drain node, got %d", result.Summary.RiskyToDrain)
	}
	if result.HealthScore >= 100 {
		t.Errorf("expected reduced health score with stateful pod, got %d", result.HealthScore)
	}
}

func TestNodeDrainReadiness_BarePod(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		// Bare pod (no owner reference)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "standalone-pod", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/node-drain-readiness", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeDrainReadiness(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NodeDrainReadinessResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.BarePods != 1 {
		t.Errorf("expected 1 bare pod, got %d", result.Summary.BarePods)
	}
	if result.Summary.DangerousToDrain != 1 {
		t.Errorf("expected 1 dangerous-to-drain node, got %d", result.Summary.DangerousToDrain)
	}
	if result.HealthScore >= 80 {
		t.Errorf("expected low health score with bare pod, got %d", result.HealthScore)
	}
}

func TestNodeDrainReadiness_CordonedNode(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Spec:       corev1.NodeSpec{Unschedulable: true},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-1", Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "app-rs"},
				},
			},
			Spec: corev1.PodSpec{NodeName: "node-2"},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/node-drain-readiness", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleNodeDrainReadiness(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result NodeDrainReadinessResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.Cordoned != 1 {
		t.Errorf("expected 1 cordoned node, got %d", result.Summary.Cordoned)
	}
	if result.Summary.SafeToDrain != 1 {
		t.Errorf("expected 1 safe-to-drain node, got %d", result.Summary.SafeToDrain)
	}
}
