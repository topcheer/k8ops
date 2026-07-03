package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

// --- Helpers ---

func bottleneckTestReq(objects ...runtime.Object) (*Server, *http.Request) {
	clientset := k8sfake.NewSimpleClientset(objects...)
	req := newReqWithClients(http.MethodGet, "/api/scaling/bottlenecks", clientset)
	return &Server{}, req
}

func makeBottleneckNode(name string, schedulable bool, conditions ...corev1.NodeCondition) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       corev1.NodeSpec{Unschedulable: !schedulable},
		Status: corev1.NodeStatus{
			Conditions: append([]corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			}, conditions...),
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("3.5"),
				corev1.ResourceMemory: resource.MustParse("14Gi"),
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}
}

// --- Node capacity tests ---

func TestBottleneck_EmptyCluster(t *testing.T) {
	srv, req := bottleneckTestReq()
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.Bottlenecks) != 0 {
		t.Errorf("expected 0 bottlenecks, got %d", len(result.Bottlenecks))
	}
}

func TestBottleneck_NodeCordoned(t *testing.T) {
	node := makeBottleneckNode("worker-1", false)

	srv, req := bottleneckTestReq(node)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckNodeSchedulable && b.Resource == "worker-1" {
			found = true
		}
	}
	if !found {
		t.Error("expected unschedulable node bottleneck not found")
	}
	if result.ClusterSummary.UnschedulableNodes != 1 {
		t.Errorf("expected 1 unschedulable node, got %d", result.ClusterSummary.UnschedulableNodes)
	}
}

func TestBottleneck_NodeMemoryPressure(t *testing.T) {
	node := makeBottleneckNode("stressed-node", true, corev1.NodeCondition{
		Type: corev1.NodeMemoryPressure, Status: corev1.ConditionTrue,
		Message: "kubelet is running out of memory",
	})

	srv, req := bottleneckTestReq(node)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckNodePressure && b.Resource == "stressed-node" {
			found = true
			if b.Impact != ImpactHigh {
				t.Errorf("expected high impact for memory pressure, got %s", b.Impact)
			}
		}
	}
	if !found {
		t.Error("expected node pressure bottleneck not found")
	}
}

func TestBottleneck_NodeDiskPressure(t *testing.T) {
	node := makeBottleneckNode("disk-full", true, corev1.NodeCondition{
		Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue,
	})

	srv, req := bottleneckTestReq(node)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckNodePressure && b.Resource == "disk-full" {
			found = true
		}
	}
	if !found {
		t.Error("expected disk pressure bottleneck not found")
	}
}

// --- Pod capacity tests ---

func TestBottleneck_PodCapacityCritical(t *testing.T) {
	node := makeBottleneckNode("node-1", true)
	// Create 100 pods (>90% of 110 capacity)
	var pods []runtime.Object
	pods = append(pods, node)
	for i := 0; i < 100; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%d", i), Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
		})
	}

	srv, req := bottleneckTestReq(pods...)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckNodeSchedulable && b.Resource == "cluster" {
			found = true
			if b.Impact != ImpactCritical {
				t.Errorf("expected critical for >90%% pod capacity, got %s", b.Impact)
			}
		}
	}
	if !found {
		t.Error("expected critical pod capacity bottleneck not found")
	}
}

func TestBottleneck_PodCapacityWarning(t *testing.T) {
	node := makeBottleneckNode("node-1", true)
	// 85 pods = ~77% of 110 capacity (between 75-90%)
	var pods []runtime.Object
	pods = append(pods, node)
	for i := 0; i < 85; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%d", i), Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
		})
	}

	srv, req := bottleneckTestReq(pods...)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckNodeSchedulable && b.Resource == "cluster" {
			found = true
			if b.Impact != ImpactHigh {
				t.Errorf("expected high for >75%% pod capacity, got %s", b.Impact)
			}
		}
	}
	if !found {
		t.Error("expected high pod capacity bottleneck not found")
	}
}

func TestBottleneck_PodCapacityHealthy(t *testing.T) {
	node := makeBottleneckNode("node-1", true)
	// 10 pods = ~9% of 110 capacity
	var pods []runtime.Object
	pods = append(pods, node)
	for i := 0; i < 10; i++ {
		pods = append(pods, &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pod-%d", i), Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "nginx"}}},
		})
	}

	srv, req := bottleneckTestReq(pods...)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckNodeSchedulable && b.Resource == "cluster" {
			t.Error("should not have cluster capacity bottleneck at 9%% usage")
		}
	}
}

// --- Resource quota tests ---

func TestBottleneck_ResourceQuotaCritical(t *testing.T) {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "compute-quota", Namespace: "app"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("10"),
				corev1.ResourceRequestsMemory: resource.MustParse("20Gi"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse("9.5"),  // 95%
				corev1.ResourceRequestsMemory: resource.MustParse("10Gi"), // 50%
			},
		},
	}

	srv, req := bottleneckTestReq(quota)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckResourceQuota && b.Namespace == "app" {
			found = true
			if b.Impact != ImpactCritical {
				t.Errorf("expected critical for 95%% quota, got %s", b.Impact)
			}
		}
	}
	if !found {
		t.Error("expected resource quota bottleneck not found")
	}
}

func TestBottleneck_ResourceQuotaWarning(t *testing.T) {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "compute-quota", Namespace: "app"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("10"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("8"), // 80%
			},
		},
	}

	srv, req := bottleneckTestReq(quota)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckResourceQuota {
			found = true
			if b.Impact != ImpactModerate {
				t.Errorf("expected moderate for 80%% quota, got %s", b.Impact)
			}
		}
	}
	if !found {
		t.Error("expected resource quota bottleneck at 80%% not found")
	}
}

func TestBottleneck_ResourceQuotaHealthy(t *testing.T) {
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "compute-quota", Namespace: "app"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("10"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceRequestsCPU: resource.MustParse("3"), // 30%
			},
		},
	}

	srv, req := bottleneckTestReq(quota)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckResourceQuota {
			t.Error("should not have quota bottleneck at 30%% usage")
		}
	}
}

// --- HPA tests ---

func TestBottleneck_HPAAtMaxReplicas(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "prod"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 5,
			MinReplicas: ptrInt32Ptr(1),
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			DesiredReplicas: 5, // at max
			CurrentReplicas: 5,
		},
	}

	srv, req := bottleneckTestReq(hpa)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckHPAStuck && b.Resource == "api-hpa" {
			found = true
			if b.Impact != ImpactHigh {
				t.Errorf("expected high impact, got %s", b.Impact)
			}
		}
	}
	if !found {
		t.Error("expected HPA at max bottleneck not found")
	}
}

func TestBottleneck_HPABelowMaxNotBottleneck(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy-hpa", Namespace: "prod"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 10,
			MinReplicas: ptrInt32Ptr(2),
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			DesiredReplicas: 3,
			CurrentReplicas: 3,
			CurrentMetrics: []autoscalingv2.MetricStatus{
				{Type: autoscalingv2.ResourceMetricSourceType},
			},
		},
	}

	srv, req := bottleneckTestReq(hpa)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckHPAStuck && b.Resource == "healthy-hpa" {
			t.Error("HPA below max should not be a bottleneck")
		}
	}
}

func TestBottleneck_HPANoMetrics(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "stuck-hpa", Namespace: "prod"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			MaxReplicas: 5,
			MinReplicas: ptrInt32Ptr(1),
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			DesiredReplicas: 2,
			CurrentReplicas: 2,
			CurrentMetrics:  nil, // no metrics!
		},
	}

	srv, req := bottleneckTestReq(hpa)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckHPAStuck && b.Resource == "stuck-hpa" && b.Detail != "" {
			if containsStr(b.Detail, "no current metrics") {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected HPA with no metrics bottleneck not found")
	}
}

// --- PDB tests ---

func TestBottleneck_PDBZeroDisruptions(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "strict-pdb", Namespace: "prod"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 0,
			CurrentHealthy:     2,
			DesiredHealthy:     2,
		},
	}

	srv, req := bottleneckTestReq(pdb)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckPDBBlocking && b.Resource == "strict-pdb" {
			found = true
			if b.Impact != ImpactHigh {
				t.Errorf("expected high impact for PDB with 0 disruptions, got %s", b.Impact)
			}
		}
	}
	if !found {
		t.Error("expected PDB blocking bottleneck not found")
	}
}

func TestBottleneck_PDBAllowsDisruptionsNotBottleneck(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "healthy-pdb", Namespace: "prod"},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 2,
			CurrentHealthy:     5,
			DesiredHealthy:     3,
		},
	}

	srv, req := bottleneckTestReq(pdb)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckPDBBlocking && b.Resource == "healthy-pdb" {
			t.Error("PDB allowing disruptions should not be a bottleneck")
		}
	}
}

// --- Summary tests ---

func TestBottleneck_ClusterSummary(t *testing.T) {
	node1 := makeBottleneckNode("node-1", true)
	node2 := makeBottleneckNode("node-2", true)
	node3 := makeBottleneckNode("node-3", false) // cordoned

	srv, req := bottleneckTestReq(node1, node2, node3)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	if result.ClusterSummary.TotalNodes != 3 {
		t.Errorf("expected 3 total nodes, got %d", result.ClusterSummary.TotalNodes)
	}
	if result.ClusterSummary.SchedulableNodes != 2 {
		t.Errorf("expected 2 schedulable nodes, got %d", result.ClusterSummary.SchedulableNodes)
	}
	if result.ClusterSummary.UnschedulableNodes != 1 {
		t.Errorf("expected 1 unschedulable node, got %d", result.ClusterSummary.UnschedulableNodes)
	}
	if result.ClusterSummary.PodCapacity != 330 { // 3 * 110
		t.Errorf("expected pod capacity 330, got %d", result.ClusterSummary.PodCapacity)
	}
}

func TestBottleneck_SummaryCounts(t *testing.T) {
	// Create multiple bottlenecks: cordoned node + HPA at max + PDB blocking
	node := makeBottleneckNode("cordoned-node", false)
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "max-hpa", Namespace: "prod"},
		Spec:       autoscalingv2.HorizontalPodAutoscalerSpec{MaxReplicas: 3},
		Status:     autoscalingv2.HorizontalPodAutoscalerStatus{DesiredReplicas: 3},
	}
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "strict-pdb", Namespace: "prod"},
		Status:     policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 0},
	}

	srv, req := bottleneckTestReq(node, hpa, pdb)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)

	if result.Summary.Total < 3 {
		t.Errorf("expected at least 3 bottlenecks, got %d", result.Summary.Total)
	}
	if result.Summary.ByType[string(BottleneckNodeSchedulable)] < 1 {
		t.Errorf("expected at least 1 node-schedulable, got %d", result.Summary.ByType[string(BottleneckNodeSchedulable)])
	}
	if result.Summary.ByType[string(BottleneckHPAStuck)] < 1 {
		t.Errorf("expected at least 1 hpa-stuck, got %d", result.Summary.ByType[string(BottleneckHPAStuck)])
	}
	// Node cordon = moderate, HPA = high, PDB = high → blocking = 2
	if result.Summary.Blocking < 2 {
		t.Errorf("expected at least 2 blocking bottlenecks, got %d", result.Summary.Blocking)
	}
}

func TestBottleneck_ImpactRank(t *testing.T) {
	if bottleneckImpactRank(ImpactCritical) != 0 {
		t.Error("critical should rank 0")
	}
	if bottleneckImpactRank(ImpactHigh) != 1 {
		t.Error("high should rank 1")
	}
	if bottleneckImpactRank(ImpactModerate) != 2 {
		t.Error("moderate should rank 2")
	}
	if bottleneckImpactRank(ImpactLow) != 3 {
		t.Error("low should rank 3")
	}
}

// --- Storage tests ---

func TestBottleneck_StoragePressure(t *testing.T) {
	// Create >500Gi of PVCs in a namespace
	var pvcs []runtime.Object
	for i := 0; i < 6; i++ {
		pvcs = append(pvcs, &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("pvc-%d", i), Namespace: "data"},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("100Gi"),
					},
				},
			},
			Status: corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		})
	}
	// Add a node to avoid empty cluster issues
	pvcs = append(pvcs, makeBottleneckNode("node-1", true))

	srv, req := bottleneckTestReq(pvcs...)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	found := false
	for _, b := range result.Bottlenecks {
		if b.Type == BottleneckStorageExhaust && b.Resource == "data" {
			found = true
		}
	}
	if !found {
		t.Error("expected storage exhaust bottleneck for >500Gi namespace not found")
	}
}

// --- Sorting test ---

func TestBottleneck_Sorting(t *testing.T) {
	// Critical (quota at 95%) + Moderate (node cordoned)
	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Name: "q1", Namespace: "ns1"},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{corev1.ResourceRequestsCPU: resource.MustParse("10")},
			Used: corev1.ResourceList{corev1.ResourceRequestsCPU: resource.MustParse("9.5")},
		},
	}
	node := makeBottleneckNode("cordoned", false)

	srv, req := bottleneckTestReq(quota, node)
	rr := httptest.NewRecorder()
	srv.handleScalingBottlenecks(rr, req)

	var result ScalingBottleneckResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.Bottlenecks) < 2 {
		t.Fatalf("expected at least 2 bottlenecks, got %d", len(result.Bottlenecks))
	}
	// Critical should be sorted first
	if result.Bottlenecks[0].Impact != ImpactCritical {
		t.Errorf("expected critical first, got %s", result.Bottlenecks[0].Impact)
	}
}

// --- Utility ---

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrInner(s, substr))
}

func containsStrInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Suppress unused import for appsv1 (used in helper patterns)
var _ appsv1.Deployment
