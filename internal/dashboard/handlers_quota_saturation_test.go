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

func TestQuotaSaturation_NoQuotas(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/quota-saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleQuotaSaturation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result QuotaSaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSWithoutQuota != 1 {
		t.Errorf("expected 1 namespace without quota, got %d", result.Summary.NSWithoutQuota)
	}
}

func TestQuotaSaturation_HealthyQuota(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "production"}},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "production"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/quota-saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleQuotaSaturation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result QuotaSaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.NSWithQuota != 1 {
		t.Errorf("expected 1 namespace with quota, got %d", result.Summary.NSWithQuota)
	}
	if result.Summary.LowSaturation != 2 {
		t.Errorf("expected 2 low saturation quotas, got %d", result.Summary.LowSaturation)
	}
}

func TestQuotaSaturation_ExhaustedQuota(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "stressed"}},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "stressed"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/quota-saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleQuotaSaturation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result QuotaSaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.ExhaustedQuotas != 1 {
		t.Errorf("expected 1 exhausted quota, got %d", result.Summary.ExhaustedQuotas)
	}
	if result.HealthScore >= 95 {
		t.Errorf("expected reduced health score, got %d", result.HealthScore)
	}
}

func TestQuotaSaturation_NearExhaustion(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "growing"}},
		&corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Name: "quota", Namespace: "growing"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("10Gi"),
				},
				Used: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("9.5Gi"),
				},
			},
		},
	)

	req := newReqWithClients(http.MethodGet, "/api/scalability/quota-saturation", clientset)
	rec := httptest.NewRecorder()
	srv := &Server{}
	srv.handleQuotaSaturation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var result QuotaSaturationResult
	json.Unmarshal(rec.Body.Bytes(), &result)

	if result.Summary.CriticalSaturation != 1 {
		t.Errorf("expected 1 critical saturation, got %d", result.Summary.CriticalSaturation)
	}
}
