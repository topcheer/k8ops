package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestComplianceMapEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-map", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleComplianceMap(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result ComplianceMapResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Frameworks) != 3 {
		t.Errorf("expected 3 frameworks, got %d", len(result.Frameworks))
	}
}

func TestComplianceMapFrameworks(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-map", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleComplianceMap(w, req)
	var result ComplianceMapResult
	json.Unmarshal(w.Body.Bytes(), &result)
	names := map[string]bool{}
	for _, fw := range result.Frameworks {
		names[fw.Name] = true
	}
	if !names["SOC2 Type II"] {
		t.Error("expected SOC2 framework")
	}
	if !names["PCI-DSS 4.0"] {
		t.Error("expected PCI-DSS framework")
	}
	if !names["HIPAA"] {
		t.Error("expected HIPAA framework")
	}
}

func TestComplianceMapPrivilegedPods(t *testing.T) {
	priv := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "priv-pod", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "c", Image: "app:v1",
					SecurityContext: &corev1.SecurityContext{Privileged: &priv},
				}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-map", clientset)
	w := httptest.NewRecorder()
	s.handleComplianceMap(w, req)
	var result ComplianceMapResult
	json.Unmarshal(w.Body.Bytes(), &result)
	// Should detect privileged pods as failing control
	found := false
	for _, fc := range result.FailingControls {
		if strings.Contains(strings.ToLower(fc.Title), "privileged") {
			found = true
		}
	}
	if !found {
		t.Error("expected privileged container violation")
	}
}

func TestComplianceMapRecs(t *testing.T) {
	result := ComplianceMapResult{
		Frameworks: []FrameworkResult{
			{Name: "SOC2", PassRate: 60, Passing: 4, TotalControls: 7, Status: "partial"},
		},
		FailingControls: []ControlFinding{
			{Framework: "SOC2", Severity: "high", Remediation: "Fix it"},
		},
		OverallScore: 60,
	}
	recs := generateComplianceMapRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
}
