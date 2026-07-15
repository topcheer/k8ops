package dashboard

import (
	"context"
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

// TestCarbonFootprintEmptyCluster verifies empty cluster behavior.
func TestCarbonFootprintEmptyCluster(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/carbon-footprint", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleCarbonFootprint(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result CarbonFootprintResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result.Summary.TotalNodes != 0 {
		t.Errorf("expected 0 nodes, got %d", result.Summary.TotalNodes)
	}
}

// TestCarbonFootprintWithNodes verifies power estimation from nodes.
func TestCarbonFootprintWithNodes(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-1",
				Labels: map[string]string{"topology.kubernetes.io/region": "us-west-2"},
			},
			Spec: corev1.NodeSpec{ProviderID: "aws:///us-west-2a/i-12345"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3500m"),
					corev1.ResourceMemory: resource.MustParse("14Gi"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod",
				Namespace: "prod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "my-app"},
				},
			},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "app:v1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/carbon-footprint", clientset)
	w := httptest.NewRecorder()
	s.handleCarbonFootprint(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result CarbonFootprintResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect region
	if !strings.Contains(result.Summary.Region, "us-west-2") {
		t.Errorf("expected region to contain us-west-2, got %s", result.Summary.Region)
	}

	// us-west-2 is low carbon (~120 gCO2/kWh)
	if result.Summary.CarbonIntensity != 120 {
		t.Errorf("expected carbon intensity 120, got %f", result.Summary.CarbonIntensity)
	}

	// Should have positive power
	if result.Summary.TotalPowerKW <= 0 {
		t.Error("expected positive power consumption")
	}

	// Should have CO2 emissions
	if result.Summary.MonthlyCO2Kg <= 0 {
		t.Error("expected positive monthly CO2")
	}

	// Should detect the workload
	if result.Summary.TotalWorkloads != 1 {
		t.Errorf("expected 1 workload, got %d", result.Summary.TotalWorkloads)
	}

	// Should have namespace stats
	if len(result.ByNamespace) == 0 {
		t.Error("expected namespace stats")
	}

	// Should have the workload in results
	found := false
	for _, wl := range result.ByWorkload {
		if wl.Name == "my-app" {
			found = true
			if wl.MonthlyCO2Kg <= 0 {
				t.Error("expected positive CO2 for workload")
			}
		}
	}
	if !found {
		t.Error("expected to find my-app workload")
	}
}

// TestCarbonFootprintHighCarbonRegion verifies high carbon region detection.
func TestCarbonFootprintHighCarbonRegion(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-1",
				Labels: map[string]string{"topology.kubernetes.io/region": "ap-south-1"},
			},
			Spec: corev1.NodeSpec{ProviderID: "aws:///ap-south-1a/i-12345"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1500m"),
					corev1.ResourceMemory: resource.MustParse("7Gi"),
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/carbon-footprint", clientset)
	w := httptest.NewRecorder()
	s.handleCarbonFootprint(w, req)

	var result CarbonFootprintResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// ap-south-1 is high carbon (700 gCO2/kWh)
	if result.Summary.CarbonIntensity != 700 {
		t.Errorf("expected carbon intensity 700, got %f", result.Summary.CarbonIntensity)
	}

	// Should have relocate opportunity for high carbon region
	hasRelocate := false
	for _, opp := range result.Opportunities {
		if opp.Type == "relocate" {
			hasRelocate = true
		}
	}
	if !hasRelocate {
		t.Error("expected relocate opportunity for high carbon region")
	}
}

// TestCarbonFootprintWastedResources verifies waste detection.
func TestCarbonFootprintWastedResources(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		// Large node with very little usage
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "big-node"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("32"),
					corev1.ResourceMemory: resource.MustParse("128Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("30000m"),
					corev1.ResourceMemory: resource.MustParse("120Gi"),
				},
			},
		},
		// Only 1 small pod using minimal resources
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tiny-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "big-node",
				Containers: []corev1.Container{
					{
						Name:  "app",
						Image: "app:v1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/carbon-footprint", clientset)
	w := httptest.NewRecorder()
	s.handleCarbonFootprint(w, req)

	var result CarbonFootprintResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should detect significant waste
	if result.Summary.WastedPowerKW <= 0 {
		t.Error("expected positive wasted power for under-utilized node")
	}

	// Should have consolidate opportunity
	hasConsolidate := false
	for _, opp := range result.Opportunities {
		if opp.Type == "consolidate" {
			hasConsolidate = true
		}
	}
	if !hasConsolidate {
		t.Error("expected consolidate opportunity for wasted resources")
	}
}

// TestEstimateIdlePowerPerNode verifies power estimation by node size.
func TestEstimateIdlePowerPerNode(t *testing.T) {
	nodes := []corev1.Node{
		{
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
		},
	}

	power := estimateIdlePowerPerNode(nodes)
	if power <= 0 {
		t.Errorf("expected positive idle power, got %f", power)
	}
	// 4 cores + 16GB should be ~120 + 16 = ~136W
	if power < 100 || power > 200 {
		t.Errorf("expected idle power 100-200W for medium node, got %f", power)
	}
}

// TestEstimateIdlePowerLargeNode verifies large node power estimation.
func TestEstimateIdlePowerLargeNode(t *testing.T) {
	nodes := []corev1.Node{
		{
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("32"),
					corev1.ResourceMemory: resource.MustParse("128Gi"),
				},
			},
		},
	}

	power := estimateIdlePowerPerNode(nodes)
	// 32 cores + 128GB should be ~250 + 256 = ~506W
	if power < 400 {
		t.Errorf("expected idle power >=400W for large node, got %f", power)
	}
}

// TestExtractRegionFromProviderID verifies region extraction.
func TestExtractRegionFromProviderID(t *testing.T) {
	tests := []struct {
		providerID string
		prefix     string
		expected   string
	}{
		{"aws:///us-east-1a/i-1234567890abcdef0", "aws://", "us-east-1"},
		{"aws:///eu-west-1b/i-abc123", "aws://", "eu-west-1"},
		{"aws:///ap-south-1c/i-xyz", "aws://", "ap-south-1"},
	}

	for _, tt := range tests {
		t.Run(tt.providerID, func(t *testing.T) {
			got := extractRegionFromProviderID(tt.providerID, tt.prefix)
			if got != tt.expected {
				t.Errorf("extractRegionFromProviderID(%q, %q) = %q, want %q", tt.providerID, tt.prefix, got, tt.expected)
			}
		})
	}
}

// TestIsRegionLike verifies region name detection.
func TestIsRegionLike(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"us-east-1", true},
		{"eu-west-1", true},
		{"ap-south-1", true},
		{"us-central1", true},
		{"europe-west1", true},
		{"node-1", false},
		{"abc", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isRegionLike(tt.input)
			if got != tt.expected {
				t.Errorf("isRegionLike(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// TestGenerateCarbonRecommendations verifies recommendation generation.
func TestGenerateCarbonRecommendations(t *testing.T) {
	result := CarbonFootprintResult{
		Summary: CarbonSummary{
			TotalPowerKW:     2.5,
			MonthlyEnergyKWh: 1800,
			MonthlyCO2Kg:     612,
			Region:           "AWS ap-south-1",
			CarbonIntensity:  700,
			WastedCO2KgMonth: 150,
		},
		ByWorkload: []CarbonWorkload{
			{Name: "big-app", Namespace: "prod", MonthlyCO2Kg: 50, Replicas: 3},
		},
		ByNamespace: []CarbonNSStat{
			{Namespace: "prod", PctClusterTotal: 45},
		},
		GreenScore: 35,
	}

	recs := generateCarbonRecommendations(result)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	foundWaste := false
	foundRegion := false
	foundTopConsumer := false
	for _, r := range recs {
		lower := strings.ToLower(r)
		if strings.Contains(lower, "wasted") {
			foundWaste = true
		}
		if strings.Contains(lower, "carbon intensity") {
			foundRegion = true
		}
		if strings.Contains(lower, "big-app") {
			foundTopConsumer = true
		}
	}
	if !foundWaste {
		t.Error("expected waste recommendation")
	}
	if !foundRegion {
		t.Error("expected region carbon intensity recommendation")
	}
	if !foundTopConsumer {
		t.Error("expected top consumer recommendation")
	}
}

// TestCarbonFootprintGPUNode verifies GPU power estimation.
func TestCarbonFootprintGPUNode(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "gpu-node"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("8"),
					corev1.ResourceMemory: resource.MustParse("64Gi"),
					"nvidia.com/gpu":      resource.MustParse("2"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("7500m"),
					corev1.ResourceMemory: resource.MustParse("60Gi"),
					"nvidia.com/gpu":      resource.MustParse("2"),
				},
			},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/carbon-footprint", clientset)
	w := httptest.NewRecorder()
	s.handleCarbonFootprint(w, req)

	var result CarbonFootprintResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// GPU nodes should have higher idle power
	if result.Summary.IdlePowerKW <= 0.2 {
		t.Errorf("expected higher idle power for GPU node, got %f kW", result.Summary.IdlePowerKW)
	}
}

// TestCarbonFootprintNamespaceAttribution verifies per-namespace breakdown.
func TestCarbonFootprintNamespaceAttribution(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("3500m"),
					corev1.ResourceMemory: resource.MustParse("14Gi"),
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-a",
				Namespace: "team-a",
			},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Name:  "c",
						Image: "app:v1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("500m"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod-b",
				Namespace: "team-b",
			},
			Spec: corev1.PodSpec{
				NodeName: "node-1",
				Containers: []corev1.Container{
					{
						Name:  "c",
						Image: "app:v1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("1000m"),
							},
						},
					},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	s := &Server{}
	req := newReqWithClients("GET", "/api/scalability/carbon-footprint", clientset)
	w := httptest.NewRecorder()
	s.handleCarbonFootprint(w, req)

	var result CarbonFootprintResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Should have 2 namespace entries
	if len(result.ByNamespace) != 2 {
		t.Errorf("expected 2 namespace entries, got %d", len(result.ByNamespace))
	}

	// team-b should have higher CO2 (more CPU requested)
	var teamA, teamB float64
	for _, ns := range result.ByNamespace {
		if ns.Namespace == "team-a" {
			teamA = ns.MonthlyCO2Kg
		}
		if ns.Namespace == "team-b" {
			teamB = ns.MonthlyCO2Kg
		}
	}
	if teamB <= teamA {
		t.Errorf("expected team-b (%.2f) > team-a (%.2f) CO2", teamB, teamA)
	}
}

// Ensure context import is used
var _ = context.Background
