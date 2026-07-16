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

func TestObsCoverageEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/obs-coverage", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleObsCoverage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result ObsCoverageResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

func TestObsCoverageBlind(t *testing.T) {
	replicas := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "blind-app", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/obs-coverage", clientset)
	w := httptest.NewRecorder()
	s.handleObsCoverage(w, req)

	var result ObsCoverageResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.BlindCount != 1 {
		t.Errorf("expected 1 blind, got %d", result.Summary.BlindCount)
	}
	if result.Summary.SignalQuality == "excellent" {
		t.Error("should not be excellent with blind workloads")
	}
	if len(result.BlindWorkloads) == 0 {
		t.Error("should have blind workload")
	}
}

func TestObsCoverageWithMetrics(t *testing.T) {
	replicas := int32(2)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "monitored", Namespace: "default",
				Annotations: map[string]string{
					"prometheus.io/scrape": "true",
					"prometheus.io/port":   "9090",
					"runbook.io/url":       "https://wiki.example.com/runbook",
				},
			},
			Spec: appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/obs-coverage", clientset)
	w := httptest.NewRecorder()
	s.handleObsCoverage(w, req)

	var result ObsCoverageResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if result.Summary.BlindCount != 0 {
		t.Errorf("expected 0 blind, got %d", result.Summary.BlindCount)
	}
	if result.Summary.WithMetrics != 1 {
		t.Errorf("expected 1 with metrics, got %d", result.Summary.WithMetrics)
	}
	if result.Summary.WithRunbook != 1 {
		t.Errorf("expected 1 with runbook, got %d", result.Summary.WithRunbook)
	}
}

func TestObsCoverageSystemNSExcluded(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: "kube-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/obs-coverage", clientset)
	w := httptest.NewRecorder()
	s.handleObsCoverage(w, req)

	var result ObsCoverageResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Summary.TotalWorkloads != 0 {
		t.Errorf("kube-system should be excluded, got %d", result.Summary.TotalWorkloads)
	}
}

func TestObsCoverageSignalQuality(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/obs-coverage", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleObsCoverage(w, req)

	var result ObsCoverageResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	validQualities := map[string]bool{
		"excellent": true, "good": true, "fair": true, "poor": true, "critical": true,
	}
	if !validQualities[result.Summary.SignalQuality] {
		t.Errorf("invalid signal quality: %s", result.Summary.SignalQuality)
	}
}

func TestObsCoverageRecommendations(t *testing.T) {
	replicas := int32(5)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "blind", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/operations/obs-coverage", clientset)
	w := httptest.NewRecorder()
	s.handleObsCoverage(w, req)

	var result ObsCoverageResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Recommendations) == 0 {
		t.Error("should generate recommendations")
	}
}
