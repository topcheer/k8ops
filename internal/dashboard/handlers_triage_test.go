package dashboard

import (
	"encoding/json"
		"fmt"
	"net/http"
	"net/http/httptest"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// TestTriageCrashLoop verifies crash loop cluster detection.
func TestTriageCrashLoop(t *testing.T) {
	var objs []runtime.Object
	objs = append(objs, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	})
	// Create 5 crash-looping pods in same namespace
	for i := 0; i < 5; i++ {
		objs = append(objs, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("crash-%d", i), Namespace: "broken"},
			Spec: corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{Name: "c", Image: "app:v1"}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{RestartCount: 10},
				},
			},
		})
	}
	clientset := k8sfake.NewSimpleClientset(objs...)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/triage", clientset)
	w := httptest.NewRecorder()

	s.handleTriage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result TriageResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	found := false
	for _, inc := range result.Incidents {
		if inc.Category == "crash-loop" && inc.Priority == "P1" {
			found = true
			if len(inc.Workloads) < 3 {
				t.Errorf("expected >= 3 workloads, got %d", len(inc.Workloads))
			}
		}
	}
	if !found {
		t.Error("expected crash-loop incident for 5 restarting pods")
	}
	if result.Summary.P1Incidents == 0 {
		t.Error("expected P1 incidents > 0")
	}
}

// TestTriageNodePressure verifies P0 detection for node pressure + pending pods.
func TestTriageNodePressure(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "pressured"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue},
				},
			},
		},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p2"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p3"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p4"}, Status: corev1.PodStatus{Phase: corev1.PodPending}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/triage", clientset)
	w := httptest.NewRecorder()

	s.handleTriage(w, req)

	var result TriageResult
	json.Unmarshal(w.Body.Bytes(), &result)

	if result.Summary.P0Incidents == 0 {
		t.Error("expected P0 incidents for node pressure + pending pods")
	}
	if result.Priority != "P0-critical" {
		t.Errorf("expected P0-critical, got %s", result.Priority)
	}
	if result.HealthScore > 70 {
		t.Errorf("expected low health score, got %d", result.HealthScore)
	}
}

// TestTriageImagePull verifies image pull failure detection.
func TestTriageImagePull(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "registry.example.com/missing:v1"}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/triage", clientset)
	w := httptest.NewRecorder()

	s.handleTriage(w, req)

	var result TriageResult
	json.Unmarshal(w.Body.Bytes(), &result)

	found := false
	for _, inc := range result.Incidents {
		if inc.Category == "image-failure" {
			found = true
		}
	}
	if !found {
		t.Error("expected image-failure incident")
	}
}

// TestTriageEmptyCluster verifies handler with healthy cluster.
func TestTriageEmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/triage", clientset)
	w := httptest.NewRecorder()

	s.handleTriage(w, req)

	var result TriageResult
	json.Unmarshal(w.Body.Bytes(), &result)

	if result.Priority != "P3-routine" {
		t.Errorf("expected P3-routine for healthy cluster, got %s", result.Priority)
	}
	if result.HealthScore != 100 {
		t.Errorf("expected health score 100, got %d", result.HealthScore)
	}
}

// TestTriageActionPlan verifies action items are generated.
func TestTriageActionPlan(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
					{Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/triage", clientset)
	w := httptest.NewRecorder()

	s.handleTriage(w, req)

	var result TriageResult
	json.Unmarshal(w.Body.Bytes(), &result)

	// Should have at least proactive long-term actions
	if len(result.LongTermFixes) == 0 {
		t.Error("expected long-term fixes")
	}

	// Verify action structure
	for _, a := range result.ActionPlan {
		if a.Effort == "" || a.Impact == "" {
			t.Errorf("action item missing effort or impact: %+v", a)
		}
	}
}

// TestTruncate verifies string truncation.
func TestTruncateStr(t *testing.T) {
	if truncateStr("short", 10) != "short" {
		t.Error("short string should not be truncated")
	}
	result := truncateStr("a-very-long-image-name-that-exceeds-limit", 20)
	if len(result) > 20 {
		t.Errorf("truncated string too long: %d", len(result))
	}
	if result[len(result)-3:] != "..." {
		t.Error("expected ellipsis at end")
	}
}
