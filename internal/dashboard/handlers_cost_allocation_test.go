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

func TestCostAllocEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-allocation", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleCostAllocation(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result CostAllocationResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.TotalMonthlyCost != 0 {
		t.Errorf("expected 0 cost, got %f", result.TotalMonthlyCost)
	}
}

func TestCostAllocWithPods(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-pod", Namespace: "prod",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Image: "app:v1", Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
				}},
			}},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-allocation", clientset)
	w := httptest.NewRecorder()
	s.handleCostAllocation(w, req)
	var result CostAllocationResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.TotalMonthlyCost <= 0 {
		t.Error("expected positive cost")
	}
	if len(result.ByNamespace) == 0 {
		t.Error("expected namespace breakdown")
	}
	if result.ByNamespace[0].Namespace != "prod" {
		t.Errorf("expected prod, got %s", result.ByNamespace[0].Namespace)
	}
}

func TestCostAllocIdlePV(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{Name: "unused-pv"},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
			},
			Status: corev1.PersistentVolumeStatus{Phase: corev1.VolumeAvailable},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/cost-allocation", clientset)
	w := httptest.NewRecorder()
	s.handleCostAllocation(w, req)
	var result CostAllocationResult
	json.Unmarshal(w.Body.Bytes(), &result)
	found := false
	for _, ir := range result.IdleResources {
		if ir.Type == "unused-pv" {
			found = true
		}
	}
	if !found {
		t.Error("expected unused PV detection")
	}
}

func TestCostAllocRecommendations(t *testing.T) {
	result := CostAllocationResult{
		TotalMonthlyCost: 500,
		Summary: CostAllocSummary{
			IdlePercent: 45, IdleMonthly: 225, AvgCostPerPod: 3.5,
		},
		ByNamespace:   []CostAllocNS{{Namespace: "prod", MonthlyCost: 150, PctOfTotal: 30}},
		IdleResources: []IdleResource{{MonthlyCost: 15}},
	}
	recs := generateCostAllocRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundIdle := false
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r), "idle") {
			foundIdle = true
		}
	}
	if !foundIdle {
		t.Error("expected idle recommendation")
	}
}

func TestCostAllocSavingsOpps(t *testing.T) {
	result := CostAllocationResult{
		Summary:       CostAllocSummary{IdlePercent: 50, IdleMonthly: 200},
		ByNamespace:   []CostAllocNS{{Namespace: "expensive", MonthlyCost: 100}},
		IdleResources: []IdleResource{{MonthlyCost: 20}},
	}
	opps := generateSavingsOpps(result, 0.034, 0.004, 730)
	if len(opps) == 0 {
		t.Fatal("expected savings opportunities")
	}
	foundConsolidate := false
	for _, o := range opps {
		if o.Type == "consolidate" {
			foundConsolidate = true
		}
	}
	if !foundConsolidate {
		t.Error("expected consolidate opportunity")
	}
}
