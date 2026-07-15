package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeExtResourceHealth_NoExtResources(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node1"},
			Status: corev1.NodeStatus{
				Capacity:    corev1.ResourceList{"cpu": resource.MustParse("4"), "memory": resource.MustParse("16Gi"), "pods": resource.MustParse("110")},
				Allocatable: corev1.ResourceList{"cpu": resource.MustParse("4"), "memory": resource.MustParse("16Gi"), "pods": resource.MustParse("110")},
			},
		},
	}

	result := analyzeExtResourceHealth(nodes, nil)

	if result.Summary.TotalExtendedResources != 0 {
		t.Errorf("expected 0 extended resources, got %d", result.Summary.TotalExtendedResources)
	}
	if result.Summary.NodesWithDevices != 0 {
		t.Errorf("expected 0 nodes with devices, got %d", result.Summary.NodesWithDevices)
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected at least one recommendation")
	}
}

func TestAnalyzeExtResourceHealth_WithGPU(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "gpu-node1",
				Labels: map[string]string{"nvidia.com/gpu.product": "A100-SXM4-40GB", "nvidia.com/gpu.driver_version": "535.129.03"},
			},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					"cpu":            resource.MustParse("8"),
					"memory":         resource.MustParse("64Gi"),
					"pods":           resource.MustParse("110"),
					"nvidia.com/gpu": resource.MustParse("4"),
				},
				Allocatable: corev1.ResourceList{
					"cpu":            resource.MustParse("8"),
					"memory":         resource.MustParse("64Gi"),
					"pods":           resource.MustParse("110"),
					"nvidia.com/gpu": resource.MustParse("2"), // 2 allocated
				},
			},
		},
	}

	result := analyzeExtResourceHealth(nodes, nil)

	if result.Summary.TotalExtendedResources != 1 {
		t.Fatalf("expected 1 extended resource, got %d", result.Summary.TotalExtendedResources)
	}
	if result.ExtendedResources[0].Name != "nvidia.com/gpu" {
		t.Errorf("expected nvidia.com/gpu, got %s", result.ExtendedResources[0].Name)
	}
	if result.ExtendedResources[0].Capacity != 4 {
		t.Errorf("expected capacity 4, got %d", result.ExtendedResources[0].Capacity)
	}
	if result.ExtendedResources[0].Allocated != 2 {
		t.Errorf("expected allocated 2, got %d", result.ExtendedResources[0].Allocated)
	}
	if len(result.GpuHealth) != 1 {
		t.Fatalf("expected 1 GPU node, got %d", len(result.GpuHealth))
	}
	if result.GpuHealth[0].Model != "A100-SXM4-40GB" {
		t.Errorf("expected GPU model A100-SXM4-40GB, got %s", result.GpuHealth[0].Model)
	}
	if !result.GpuHealth[0].Healthy {
		t.Error("expected GPU node to be healthy (has available GPUs)")
	}
}

func TestAnalyzeExtResourceHealth_GPUFullyAllocated(t *testing.T) {
	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gpu-full"},
			Status: corev1.NodeStatus{
				Capacity: corev1.ResourceList{
					"cpu":            resource.MustParse("8"),
					"memory":         resource.MustParse("64Gi"),
					"pods":           resource.MustParse("110"),
					"nvidia.com/gpu": resource.MustParse("2"),
				},
				Allocatable: corev1.ResourceList{
					"cpu":            resource.MustParse("8"),
					"memory":         resource.MustParse("64Gi"),
					"pods":           resource.MustParse("110"),
					"nvidia.com/gpu": resource.MustParse("0"), // all allocated
				},
			},
		},
	}

	result := analyzeExtResourceHealth(nodes, nil)

	// Should have a GPUFullyAllocated issue
	found := false
	for _, iss := range result.Issues {
		if iss.Type == "GPUFullyAllocated" {
			found = true
		}
	}
	if !found {
		t.Error("expected GPUFullyAllocated issue")
	}
	if result.GpuHealth[0].Healthy {
		t.Error("expected GPU node to be unhealthy (fully allocated)")
	}
}

func TestAnalyzeExtResourceHealth_DevicePluginPod(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nvidia-device-plugin-abc", Namespace: "kube-system"},
			Spec:       corev1.PodSpec{NodeName: "node1"},
			Status: corev1.PodStatus{
				Phase:             corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{RestartCount: 0}},
			},
		},
	}

	result := analyzeExtResourceHealth(nil, pods)

	if result.Summary.TotalDevicePlugins != 1 {
		t.Errorf("expected 1 device plugin, got %d", result.Summary.TotalDevicePlugins)
	}
	if result.Summary.HealthyDevicePlugins != 1 {
		t.Errorf("expected 1 healthy device plugin, got %d", result.Summary.HealthyDevicePlugins)
	}
	if len(result.DevicePlugins) != 1 {
		t.Fatalf("expected 1 device plugin entry, got %d", len(result.DevicePlugins))
	}
	if !result.DevicePlugins[0].Ready {
		t.Error("expected device plugin to be ready")
	}
}

func TestIsExtendedResource(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"cpu", false},
		{"memory", false},
		{"pods", false},
		{"ephemeral-storage", false},
		{"hugepages-2Mi", false},
		{"nvidia.com/gpu", true},
		{"amd.com/gpu", true},
		{"intel.com/rdma", true},
		{"example.com/custom-resource", true},
	}
	for _, tt := range tests {
		got := isExtendedResource(tt.name)
		if got != tt.want {
			t.Errorf("isExtendedResource(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
