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

func TestDeployQuota_NoQuota(t *testing.T) {
	replicas := int32(1)
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app-deploy", Namespace: "app-prod"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/quota-impact", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployQuota(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result DeployQuotaResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSWithoutQuota < 1 {
		t.Errorf("expected at least 1 namespace without quota, got %d", result.Summary.NSWithoutQuota)
	}
}

func TestDeployQuota_OverQuota(t *testing.T) {
	cpuQuota := resource.MustParse("1000m")
	cpuUsed := resource.MustParse("1100m")
	memQuota := resource.MustParse("1Gi")

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "app-prod"},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU:    cpuQuota,
					corev1.ResourceMemory: memQuota,
				},
			},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU:    cpuQuota,
					corev1.ResourceMemory: memQuota,
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU:    cpuUsed,
					corev1.ResourceMemory: resource.MustParse("100Mi"),
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/quota-impact", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployQuota(rec, req)

	var result DeployQuotaResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSOverQuota < 1 {
		t.Errorf("expected at least 1 namespace over quota, got %d", result.Summary.NSOverQuota)
	}
}

func TestDeployQuota_NearLimit(t *testing.T) {
	cpuQuota := resource.MustParse("1000m")
	cpuUsed := resource.MustParse("850m")

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "app-prod"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU: cpuQuota,
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU: cpuUsed,
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/quota-impact", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployQuota(rec, req)

	var result DeployQuotaResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSNearQuotaLimit < 1 {
		t.Errorf("expected at least 1 namespace near limit, got %d", result.Summary.NSNearQuotaLimit)
	}
}

func TestDeployQuota_Headroom(t *testing.T) {
	cpuQuota := resource.MustParse("10000m")
	cpuUsed := resource.MustParse("1000m")

	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app-prod"}},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "app-prod"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU: cpuQuota,
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU: cpuUsed,
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/deployment/quota-impact", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployQuota(rec, req)

	var result DeployQuotaResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSWithHeadroom < 1 {
		t.Errorf("expected at least 1 namespace with headroom, got %d", result.Summary.NSWithHeadroom)
	}
	if result.HealthScore < 90 {
		t.Errorf("expected high health score with headroom, got %d", result.HealthScore)
	}
}

func TestDeployQuota_EmptyCluster(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset()

	req := newReqWithClients(http.MethodGet, "/api/deployment/quota-impact", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleDeployQuota(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result DeployQuotaResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalNamespaces != 0 {
		t.Errorf("expected 0 namespaces, got %d", result.Summary.TotalNamespaces)
	}
}
