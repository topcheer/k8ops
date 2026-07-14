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

func TestSaturation_UnboundedPods(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-unbounded", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						// No limits
					}},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSaturation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result SaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.UnboundedPods != 1 {
		t.Errorf("expected 1 unbounded pod, got %d", result.Summary.UnboundedPods)
	}
	found := false
	for _, r := range result.ThrottlingRisks {
		if r.Issue == "No resource limits set — unbounded resource consumption risk" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find unbounded risk")
	}
}

func TestSaturation_HighCPURatio(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-ratio", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("10m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					}},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSaturation(rec, req)

	var result SaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.HighCPULimitRatio != 1 {
		t.Errorf("expected 1 high CPU ratio, got %d", result.Summary.HighCPULimitRatio)
	}
}

func TestSaturation_OOMRisk(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-oom", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("100m"),
							// No memory limit
						},
					}},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSaturation(rec, req)

	var result SaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.OOMRiskPods < 1 {
		t.Errorf("expected at least 1 OOM risk pod, got %d", result.Summary.OOMRiskPods)
	}
}

func TestSaturation_HealthyPod(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-healthy", Namespace: "app-prod"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "c1", Image: "nginx", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					}},
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSaturation(rec, req)

	var result SaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.HealthScore < 95 {
		t.Errorf("expected high health score for healthy pod, got %d", result.HealthScore)
	}
}

func TestSaturation_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/scalability/saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleSaturation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result SaturationResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalPods != 0 {
		t.Errorf("expected 0 pods, got %d", result.Summary.TotalPods)
	}
}
