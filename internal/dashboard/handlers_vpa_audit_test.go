package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestVPAAudit_NotInstalled(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/vpa-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleVPAAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result VPAAuditResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// VPA is not installed (fake clientset doesn't have VPA CRD)
	if !result.Summary.VPANotInstalled {
		t.Error("expected VPA not installed to be true")
	}
	if result.HealthScore > 85 {
		t.Errorf("expected reduced health score when VPA not installed, got %d", result.HealthScore)
	}
}

func TestVPAAudit_OOMWorkloads(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		// Pod with OOM kill
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-oom-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "app-deploy-abc"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					}},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "c1",
						RestartCount: 5,
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason: "OOMKilled",
							},
						},
					},
				},
			},
		},
		// Pod without OOM
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-healthy-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "app-good-def"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					}},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c1", RestartCount: 0},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/vpa-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleVPAAudit(rec, req)

	var result VPAAuditResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	// Should find at least 1 workload with OOM
	foundOOM := false
	for _, tw := range result.TargetWorkloads {
		if tw.HasOOMKill {
			foundOOM = true
			if tw.Severity != "high" {
				t.Errorf("expected high severity for OOM with restarts, got %s", tw.Severity)
			}
		}
	}
	if !foundOOM {
		t.Error("expected to find at least one workload with OOM kill")
	}
}

func TestVPAAudit_HealthScore(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "StatefulSet", Name: "app-stateful"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					}},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "c1", RestartCount: 0},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/vpa-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleVPAAudit(rec, req)

	var result VPAAuditResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	// No OOM, no high restarts → relatively high score (but VPA not installed deducts 20)
	if result.HealthScore < 70 {
		t.Errorf("expected health score >= 70 for clean workloads, got %d", result.HealthScore)
	}
}

func TestVPAAudit_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/scalability/vpa-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleVPAAudit(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result VPAAuditResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(result.TargetWorkloads) != 0 {
		t.Errorf("expected 0 target workloads, got %d", len(result.TargetWorkloads))
	}
}

func TestVPAAudit_Recommendations(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-oom",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "app-deploy"},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx:1.25"},
				},
			},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "c1",
						RestartCount: 4,
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								Reason: "OOMKilled",
							},
						},
					},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/vpa-audit", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleVPAAudit(rec, req)

	var result VPAAuditResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if len(result.Recommendations) == 0 {
		t.Error("expected at least one recommendation")
	}

	foundOOMRec := false
	for _, rec := range result.Recommendations {
		if strings.Contains(rec, "OOMKilled") {
			foundOOMRec = true
		}
	}
	if !foundOOMRec {
		t.Error("expected recommendation mentioning OOMKilled workloads")
	}
}
