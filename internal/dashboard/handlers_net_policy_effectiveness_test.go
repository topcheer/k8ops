package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNetPolicyEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/net-policy-effectiveness", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleNetPolicyEffectiveness(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var result NetPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.ScannedAt.IsZero() {
		t.Error("ScannedAt should not be zero")
	}
}

func TestNetPolicyNoPolicies(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/net-policy-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleNetPolicyEffectiveness(w, req)

	var result NetPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Summary.NamespacesWithoutNP == 0 {
		t.Error("Should detect namespace without network policies")
	}
	if len(result.UnprotectedNS) == 0 {
		t.Error("Should have unprotected namespace entry")
	}
}

func TestNetPolicyWithDefaultDeny(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&netv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "default-deny", Namespace: "default"},
			Spec: netv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{},
				PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeIngress, netv1.PolicyTypeEgress},
			},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/net-policy-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleNetPolicyEffectiveness(w, req)

	var result NetPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Summary.NamespacesWithNP == 0 {
		t.Error("Should detect namespace with network policy")
	}
	if result.Summary.DenyAllPolicies == 0 {
		t.Error("Should detect deny-all policy (empty ingress+egress)")
	}
	if result.IsolationScore <= 0 {
		t.Error("Should have positive isolation score with deny-all policy")
	}
}

func TestNetPolicySystemNSExcluded(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&netv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "np", Namespace: "kube-system"},
			Spec:       netv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/net-policy-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleNetPolicyEffectiveness(w, req)

	var result NetPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result.Summary.TotalNamespaces != 0 {
		t.Errorf("kube-system should be excluded, got %d", result.Summary.TotalNamespaces)
	}
}

func TestNetPolicyRecommendations(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/net-policy-effectiveness", clientset)
	w := httptest.NewRecorder()
	s.handleNetPolicyEffectiveness(w, req)

	var result NetPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Recommendations) == 0 {
		t.Error("Should generate recommendations")
	}
}

func TestNetPolicyZeroTrust(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/net-policy-effectiveness", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleNetPolicyEffectiveness(w, req)

	var result NetPolicyResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	validLevels := map[string]bool{"none": true, "low": true, "moderate": true, "high": true}
	if !validLevels[result.ZeroTrustLevel] {
		t.Errorf("invalid zero trust level: %s", result.ZeroTrustLevel)
	}
}
