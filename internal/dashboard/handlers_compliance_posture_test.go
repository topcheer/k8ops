package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestCompliancePostureEmptyCluster(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-posture", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handleCompliancePosture(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result ComplianceFrameworkResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
	// Empty cluster = all controls pass (no violations)
	if result.OverallScore != 100 {
		t.Errorf("empty cluster should score 100, got %d", result.OverallScore)
	}
}

func TestCompliancePosturePrivileged(t *testing.T) {
	priv := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "c", Image: "nginx:latest",
							SecurityContext: &corev1.SecurityContext{Privileged: &priv}},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-posture", clientset)
	w := httptest.NewRecorder()

	s.handleCompliancePosture(w, req)

	var result ComplianceFrameworkResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should find AC-1 failure
	foundAC1 := false
	for _, cr := range result.ControlResults {
		if cr.ID == "AC-1" && cr.Status == "fail" {
			foundAC1 = true
		}
	}
	if !foundAC1 {
		t.Error("Should detect AC-1 failure (privileged containers)")
	}
	if result.OverallScore >= 100 {
		t.Error("Should not have perfect score with violations")
	}
}

func TestCompliancePostureFrameworks(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-posture", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handleCompliancePosture(w, req)

	var result ComplianceFrameworkResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.Frameworks) != 5 {
		t.Errorf("expected 5 frameworks, got %d", len(result.Frameworks))
	}

	fwNames := map[string]bool{}
	for _, fw := range result.Frameworks {
		fwNames[fw.Framework] = true
		if fw.FullName == "" {
			t.Errorf("framework %s should have full name", fw.Framework)
		}
	}

	for _, expected := range []string{"SOC2", "PCI-DSS", "HIPAA", "NIST", "GDPR"} {
		if !fwNames[expected] {
			t.Errorf("framework %s missing", expected)
		}
	}
}

func TestCompliancePostureLatestTag(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "c", Image: "nginx:latest"},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-posture", clientset)
	w := httptest.NewRecorder()

	s.handleCompliancePosture(w, req)

	var result ComplianceFrameworkResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should detect SC-1 failure (latest tag)
	foundSC1 := false
	for _, cr := range result.ControlResults {
		if cr.ID == "SC-1" && cr.Status == "fail" {
			foundSC1 = true
		}
	}
	if !foundSC1 {
		t.Error("Should detect SC-1 failure (latest tag images)")
	}
}

func TestCompliancePostureSystemNSExcluded(t *testing.T) {
	priv := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "dns", Namespace: "kube-system"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "c", Image: "nginx:latest",
							SecurityContext: &corev1.SecurityContext{Privileged: &priv}},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-posture", clientset)
	w := httptest.NewRecorder()

	s.handleCompliancePosture(w, req)

	var result ComplianceFrameworkResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// kube-system should be excluded — no violations
	if result.OverallScore != 100 {
		t.Errorf("kube-system violations should be excluded, got score %d", result.OverallScore)
	}
}

func TestCompliancePostureRemediationPlan(t *testing.T) {
	priv := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{
						{Name: "c", Image: "nginx:latest",
							SecurityContext: &corev1.SecurityContext{Privileged: &priv},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("100m"),
								},
							},
						},
					}},
				},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-posture", clientset)
	w := httptest.NewRecorder()

	s.handleCompliancePosture(w, req)

	var result ComplianceFrameworkResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(result.RemediationPlan) == 0 {
		t.Error("Should have remediation plan for violations")
	}

	// First item should be priority 1
	if len(result.RemediationPlan) > 0 && result.RemediationPlan[0].Priority != 1 {
		t.Error("First remediation item should be priority 1")
	}
}

func TestCompliancePostureSorted(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/compliance-posture", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()

	s.handleCompliancePosture(w, req)

	var result ComplianceFrameworkResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Frameworks sorted by score ascending (worst first)
	for i := 1; i < len(result.Frameworks); i++ {
		if result.Frameworks[i].Score < result.Frameworks[i-1].Score {
			t.Error("frameworks should be sorted by score ascending")
		}
	}

	// Controls: failures before passes
	for i := 1; i < len(result.ControlResults); i++ {
		if result.ControlResults[i-1].Status == "pass" && result.ControlResults[i].Status == "fail" {
			t.Error("failed controls should come before passing ones")
		}
	}
}

func TestGetFrameworkFullName(t *testing.T) {
	if getFrameworkFullName("SOC2") != "SOC 2 Type II" {
		t.Error("SOC2 should map to full name")
	}
	if getFrameworkFullName("PCI-DSS") != "PCI-DSS v4.0" {
		t.Error("PCI-DSS should map to full name")
	}
	if getFrameworkFullName("unknown") != "unknown" {
		t.Error("unknown should return as-is")
	}
}
