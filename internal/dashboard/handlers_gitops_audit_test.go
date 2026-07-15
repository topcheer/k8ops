package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestGitOpsEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/gitops-audit", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleGitOpsAudit(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result GitOpsAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.HealthScore >= 100 {
		t.Errorf("expected < 100 with no GitOps, got %d", result.HealthScore)
	}
}

func TestGitOpsArgoCDDetection(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "argocd-server-abc", Namespace: "argocd"},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "server", Ready: true, Image: "argocd:v2.7.0"}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/gitops-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGitOpsAudit(w, req)
	var result GitOpsAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result.Summary.HasArgoCD {
		t.Error("expected ArgoCD detection")
	}
	found := false
	for _, tool := range result.Tools {
		if tool.Type == "argocd" && tool.Healthy {
			found = true
		}
	}
	if !found {
		t.Error("expected healthy argocd tool")
	}
}

func TestGitOpsFluxDetection(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "flux-controller-xyz", Namespace: "flux-system"},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "c", Ready: true}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/gitops-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGitOpsAudit(w, req)
	var result GitOpsAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if !result.Summary.HasFlux {
		t.Error("expected Flux detection")
	}
}

func TestGitOpsHelmReleases(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.myapp.v1", Namespace: "prod",
				Labels: map[string]string{"owner": "helm", "name": "myapp"},
			},
			Data: map[string][]byte{"release": []byte("data")},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/gitops-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGitOpsAudit(w, req)
	var result GitOpsAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.TotalReleases == 0 {
		t.Error("expected Helm release detection")
	}
	if !result.Summary.HasHelm {
		t.Error("expected Helm controller flag")
	}
}

func TestGitOpsManagedWorkloads(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "managed-app", Namespace: "prod",
				Annotations: map[string]string{"argocd.argoproj.io/application": "my-app"},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "m"}},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "manual-app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/deployment/gitops-audit", clientset)
	w := httptest.NewRecorder()
	s.handleGitOpsAudit(w, req)
	var result GitOpsAuditResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.AutoSyncEnabled != 1 {
		t.Errorf("expected 1 auto-sync, got %d", result.Summary.AutoSyncEnabled)
	}
}

func TestGitOpsAuditRecs(t *testing.T) {
	result := GitOpsAuditResult{
		Summary: GitOpsAuditSummary{
			HasArgoCD: false, HasFlux: false, HasHelm: false,
			TotalReleases: 5,
		},
	}
	recs := generateGitOpsRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundNoGitOps := false
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r), "no gitops") {
			foundNoGitOps = true
		}
	}
	if !foundNoGitOps {
		t.Error("expected no-gitops recommendation")
	}
}
