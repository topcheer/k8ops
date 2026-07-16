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

func TestSchedulingIntelEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scheduling-intel", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleSchedulingIntel(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result SchedulingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

func TestSchedulingIntelWithNodes(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scheduling-intel", clientset)
	w := httptest.NewRecorder()
	s.handleSchedulingIntel(w, req)

	var result SchedulingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.TotalNodes != 1 {
		t.Errorf("expected 1 node, got %d", result.Summary.TotalNodes)
	}
	if len(result.ByNode) != 1 {
		t.Errorf("expected 1 node analysis, got %d", len(result.ByNode))
	}
	// Empty node should be underutilized
	if result.ByNode[0].Status != "underutilized" {
		t.Errorf("expected underutilized, got %s", result.ByNode[0].Status)
	}
	// Should be able to fit large pods
	if result.ByNode[0].LargestFitCPU == "none" {
		t.Error("empty node should be able to fit pods")
	}
}

func TestSchedulingIntelFragileNode(t *testing.T) {
	// Node with very little allocatable left
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "packed"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10m"),
					corev1.ResourceMemory: resource.MustParse("32Mi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "packed"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scheduling-intel", clientset)
	w := httptest.NewRecorder()
	s.handleSchedulingIntel(w, req)

	var result SchedulingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Node should be fragile (can't fit even nano pod)
	if len(result.ByNode) > 0 && result.ByNode[0].Status != "fragile" {
		t.Errorf("expected fragile node, got %s", result.ByNode[0].Status)
	}
}

func TestSchedulingIntelRecommendations(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("32Gi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scheduling-intel", clientset)
	w := httptest.NewRecorder()
	s.handleSchedulingIntel(w, req)

	var result SchedulingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Recommendations) == 0 {
		t.Error("should generate recommendations")
	}
}

func TestSchedulingIntelScore(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scheduling-intel", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleSchedulingIntel(w, req)

	var result SchedulingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.SchedulingScore < 0 || result.SchedulingScore > 100 {
		t.Errorf("score should be 0-100, got %d", result.SchedulingScore)
	}
	validGrades := map[string]bool{"A": true, "B": true, "C": true, "D": true, "F": true}
	if !validGrades[result.Grade] {
		t.Errorf("invalid grade: %s", result.Grade)
	}
}

func TestSchedulingIntelStrandedResources(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "fragile"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("15m"),
					corev1.ResourceMemory: resource.MustParse("50Mi"),
					corev1.ResourcePods:   resource.MustParse("110"),
				},
				Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "fragile"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/scheduling-intel", clientset)
	w := httptest.NewRecorder()
	s.handleSchedulingIntel(w, req)

	var result SchedulingIntelResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.FragileNodes) == 0 {
		t.Error("should detect fragile node")
	}
	if len(result.StrandedResources) == 0 {
		t.Error("should detect stranded resources")
	}
}
