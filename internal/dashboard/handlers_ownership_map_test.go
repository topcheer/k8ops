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

func TestOwnershipMapEmptyCluster(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/ownership-map", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleOwnershipMap(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result OwnershipMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

func TestOwnershipMapWithLabels(t *testing.T) {
	replicas := int32(2)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web", Namespace: "default",
				Labels:      map[string]string{"app": "web", "team": "platform", "version": "v1"},
				Annotations: map[string]string{"contact": "platform@team.io"},
			},
			Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/ownership-map", clientset)
	w := httptest.NewRecorder()
	s.handleOwnershipMap(w, req)

	var result OwnershipMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.TotalWorkloads != 1 {
		t.Errorf("expected 1 workload, got %d", result.Summary.TotalWorkloads)
	}
	if result.Summary.WithOwnerLabel != 1 {
		t.Errorf("expected 1 with owner, got %d", result.Summary.WithOwnerLabel)
	}
	if result.Summary.WithoutOwnerLabel != 0 {
		t.Errorf("expected 0 without owner, got %d", result.Summary.WithoutOwnerLabel)
	}
	if result.Summary.CoveragePct < 99 {
		t.Errorf("expected 100%% coverage, got %.0f%%", result.Summary.CoveragePct)
	}
	if result.Summary.UniqueTeams != 1 {
		t.Errorf("expected 1 unique team, got %d", result.Summary.UniqueTeams)
	}
	if result.LabelCoverage.AppLabel != 100 {
		t.Errorf("expected 100%% app label, got %d%%", result.LabelCoverage.AppLabel)
	}
	if result.LabelCoverage.TeamLabel != 100 {
		t.Errorf("expected 100%% team label, got %d%%", result.LabelCoverage.TeamLabel)
	}
}

func TestOwnershipMapOrphaned(t *testing.T) {
	replicas := int32(5)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "default"}, // No labels!
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/ownership-map", clientset)
	w := httptest.NewRecorder()
	s.handleOwnershipMap(w, req)

	var result OwnershipMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.WithoutOwnerLabel != 1 {
		t.Errorf("expected 1 orphaned, got %d", result.Summary.WithoutOwnerLabel)
	}
	if len(result.OrphanedWorkloads) == 0 {
		t.Error("should have orphaned workload entry")
	}
	if len(result.OrphanedWorkloads) > 0 {
		ow := result.OrphanedWorkloads[0]
		if ow.Name != "orphan" {
			t.Errorf("expected orphan, got %s", ow.Name)
		}
		if ow.Replicas != 5 {
			t.Errorf("expected 5 replicas, got %d", ow.Replicas)
		}
		if ow.Severity != "high" {
			t.Errorf("expected high severity (5 replicas), got %s", ow.Severity)
		}
	}
}

func TestOwnershipMapNSTeamLabel(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   "team-ns",
			Labels: map[string]string{"team": "backend"},
		}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "team-ns"}, // No workload labels but NS has team
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/ownership-map", clientset)
	w := httptest.NewRecorder()
	s.handleOwnershipMap(w, req)

	var result OwnershipMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should count as owned because namespace has team label
	if result.Summary.WithOwnerLabel != 1 {
		t.Errorf("expected 1 owned (via NS label), got %d", result.Summary.WithOwnerLabel)
	}
}

func TestOwnershipMapSystemNSExcluded(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: "kube-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/ownership-map", clientset)
	w := httptest.NewRecorder()
	s.handleOwnershipMap(w, req)

	var result OwnershipMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("kube-system should be excluded, got %d", result.Summary.TotalWorkloads)
	}
}

func TestOwnershipMapByNamespace(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-a"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "ns-b"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "a1", Namespace: "ns-a",
				Labels: map[string]string{"team": "x"}},
			Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "a2", Namespace: "ns-a"}, // orphaned
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "b1", Namespace: "ns-b",
				Labels: map[string]string{"team": "y"}},
			Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/ownership-map", clientset)
	w := httptest.NewRecorder()
	s.handleOwnershipMap(w, req)

	var result OwnershipMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// ns-a: 50% coverage (1/2), should be "partial" or "orphaned"
	nsAFound := false
	for _, ns := range result.ByNamespace {
		if ns.Namespace == "ns-a" {
			nsAFound = true
			if ns.TotalWorkloads != 2 {
				t.Errorf("ns-a should have 2 workloads, got %d", ns.TotalWorkloads)
			}
			if ns.CoveragePct > 50.1 || ns.CoveragePct < 49.9 {
				t.Errorf("ns-a coverage should be 50%%, got %.0f%%", ns.CoveragePct)
			}
		}
	}
	if !nsAFound {
		t.Error("ns-a should be in ByNamespace")
	}
}

func TestOwnershipMapRecommendations(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/product/ownership-map", clientset)
	w := httptest.NewRecorder()
	s.handleOwnershipMap(w, req)

	var result OwnershipMapResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Recommendations) == 0 {
		t.Error("should generate recommendations for orphaned workloads")
	}
}

func TestContainsNS(t *testing.T) {
	s := []string{"a", "b", "c"}
	if !containsNS(s, "b") {
		t.Error("should find b")
	}
	if containsNS(s, "d") {
		t.Error("should not find d")
	}
}
