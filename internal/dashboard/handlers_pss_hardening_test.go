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

func TestPSSHardening_PrivilegedContainer(t *testing.T) {
	privileged := true
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-priv-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "app-priv-abc"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "priv-container",
						Image: "nginx:1.25",
						SecurityContext: &corev1.SecurityContext{
							Privileged: &privileged,
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/pss-hardening", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePSSHardening(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result PSSHardeningResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.PrivilegedContainers != 1 {
		t.Errorf("expected 1 privileged container, got %d", result.Summary.PrivilegedContainers)
	}
	found := false
	for _, p := range result.PrivilegedPods {
		if p.Container == "priv-container" && p.Severity == "critical" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find privileged container with critical severity")
	}
	if result.HealthScore > 80 {
		t.Errorf("expected reduced health score for privileged container, got %d", result.HealthScore)
	}
}

func TestPSSHardening_NoSecurityContext(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-nosec-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "app-nosec"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25"}, // no security context
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/pss-hardening", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePSSHardening(rec, req)

	var result PSSHardeningResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	// No seccomp = should be flagged
	if result.Summary.PodsNoSeccomp < 1 {
		t.Errorf("expected at least 1 pod without seccomp, got %d", result.Summary.PodsNoSeccomp)
	}
	// No readOnlyRootFilesystem
	if result.Summary.PodsNoReadOnlyRootFS < 1 {
		t.Errorf("expected at least 1 pod without readOnlyRootFS, got %d", result.Summary.PodsNoReadOnlyRootFS)
	}
}

func TestPSSHardening_HostNamespace(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-host-pod"},
			Spec: corev1.PodSpec{
				HostPID:     true,
				HostNetwork: true,
				Containers:  []corev1.Container{{Name: "c1", Image: "nginx"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/pss-hardening", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePSSHardening(rec, req)

	var result PSSHardeningResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.PodsWithHostPID != 1 {
		t.Errorf("expected 1 pod with hostPID, got %d", result.Summary.PodsWithHostPID)
	}
	if result.Summary.PodsWithHostNetwork != 1 {
		t.Errorf("expected 1 pod with hostNetwork, got %d", result.Summary.PodsWithHostNetwork)
	}
}

func TestPSSHardening_Capabilities(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-caps-pod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "c1",
						Image: "nginx:1.25",
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{"NET_ADMIN", "SYS_TIME"},
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/pss-hardening", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePSSHardening(rec, req)

	var result PSSHardeningResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.ContainersWithAddCaps != 1 {
		t.Errorf("expected 1 container with add caps, got %d", result.Summary.ContainersWithAddCaps)
	}
}

func TestPSSHardening_FullyHardened(t *testing.T) {
	falseVal := false
	trueVal := true
	seccompRuntime := corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-hardened-pod"},
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					SeccompProfile: &seccompRuntime,
				},
				Containers: []corev1.Container{
					{
						Name:  "c1",
						Image: "nginx:1.25",
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &falseVal,
							ReadOnlyRootFilesystem:   &trueVal,
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/security/pss-hardening", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handlePSSHardening(rec, req)

	var result PSSHardeningResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Fully hardened pod should have high score
	if result.HealthScore < 95 {
		t.Errorf("expected high health score for hardened pod, got %d", result.HealthScore)
	}
	if result.Summary.PrivilegedContainers != 0 {
		t.Errorf("expected 0 privileged containers, got %d", result.Summary.PrivilegedContainers)
	}
}
