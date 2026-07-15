package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestRollbackRiskEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollback-risk", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleRollbackRisk(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result RollbackRiskResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ReadinessScore != 100 {
		t.Errorf("expected 100, got %d", result.ReadinessScore)
	}
}

func TestRollbackRiskNoHistory(t *testing.T) {
	zero := int32(0)
	r := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &r, RevisionHistoryLimit: &zero,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "app"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "app"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "app:v1"}}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollback-risk", clientset)
	w := httptest.NewRecorder()
	s.handleRollbackRisk(w, req)
	var result RollbackRiskResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.NoHistory != 1 {
		t.Errorf("expected 1 no-history, got %d", result.Summary.NoHistory)
	}
	if len(result.Workloads) > 0 && result.Workloads[0].RiskLevel != "critical" {
		t.Errorf("expected critical, got %s", result.Workloads[0].RiskLevel)
	}
}

func TestRollbackRiskLatestTag(t *testing.T) {
	limit := int32(10)
	r := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &r, RevisionHistoryLimit: &limit,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: "nginx:latest"}}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollback-risk", clientset)
	w := httptest.NewRecorder()
	s.handleRollbackRisk(w, req)
	var result RollbackRiskResult
	json.Unmarshal(w.Body.Bytes(), &result)
	found := false
	for _, wl := range result.Workloads {
		if wl.Name == "web" && wl.HasBreakingChange {
			found = true
		}
	}
	if !found {
		t.Error("expected :latest flagged as breaking change")
	}
}

func TestRollbackRiskSafe(t *testing.T) {
	limit := int32(10)
	r := int32(3)
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "stable", Namespace: "prod",
				CreationTimestamp: metav1.Time{Time: time.Now().Add(-72 * time.Hour)},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &r, RevisionHistoryLimit: &limit,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "stable"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "stable"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "stable", Image: "app:v2.1.0"}}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/rollback-risk", clientset)
	w := httptest.NewRecorder()
	s.handleRollbackRisk(w, req)
	var result RollbackRiskResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.SafeRollback != 1 {
		t.Errorf("expected 1 safe, got %d", result.Summary.SafeRollback)
	}
}

func TestRollbackRiskRecommendations(t *testing.T) {
	result := RollbackRiskResult{
		Summary:        RollbackSummary{NoHistory: 2, LimitedHistory: 3, HighRollbackRisk: 1, SafeRollback: 5},
		ReadinessScore: 40,
	}
	recs := generateRollbackRecommendations(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundNoHistory := false
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r), "revisionhistorylimit") {
			foundNoHistory = true
		}
	}
	if !foundNoHistory {
		t.Error("expected no-history recommendation")
	}
}
