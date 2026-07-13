package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeLifecycleScore(t *testing.T) {
	tests := []struct {
		name     string
		s        NodeLifecycleSummary
		minScore int
		maxScore int
	}{
		{"all consistent", NodeLifecycleSummary{TotalNodes: 5, KernelVersions: 1, OSImages: 1}, 95, 100},
		{"kernel drift", NodeLifecycleSummary{TotalNodes: 10, KernelVersions: 3, KernelDrift: true}, 75, 85},
		{"os drift", NodeLifecycleSummary{TotalNodes: 10, OSImages: 4, OSImageDrift: true}, 80, 90},
		{"old nodes", NodeLifecycleSummary{TotalNodes: 10, NodesOlderThan180d: 2, NodesOlderThan90d: 3}, 80, 90},
		{"all bad", NodeLifecycleSummary{TotalNodes: 10, KernelVersions: 5, KernelDrift: true, OSImages: 4, OSImageDrift: true, NodesOlderThan180d: 3}, 0, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := nodeLifecycleScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestNodeLifecycleRecommendations(t *testing.T) {
	t.Run("all consistent", func(t *testing.T) {
		recs := nodeLifecycleRecommendations(NodeLifecycleSummary{TotalNodes: 5, KernelVersions: 1, OSImages: 1})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := nodeLifecycleRecommendations(NodeLifecycleSummary{
			TotalNodes: 10, KernelVersions: 3, KernelDrift: true,
			OSImages: 2, OSImageDrift: true, NodesOlderThan180d: 2, HasGPU: true, GPUNodes: 1,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestNodeLifecycleAuditCore(t *testing.T) {
	now := time.Now()

	nodes := []corev1.Node{
		// Node with kernel A, OS A, created recently
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1", CreationTimestamp: metav1.Time{Time: now.Add(-30 * 24 * time.Hour)}},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{KernelVersion: "5.15.0-91", OSImage: "Ubuntu 22.04.3 LTS", Architecture: "amd64"},
			},
		},
		// Node with kernel B (drift), OS A, created 100 days ago
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-2", CreationTimestamp: metav1.Time{Time: now.Add(-100 * 24 * time.Hour)}},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{KernelVersion: "5.15.0-89", OSImage: "Ubuntu 22.04.3 LTS", Architecture: "amd64"},
			},
		},
		// Node with kernel A, OS B (drift), created 200 days ago (old)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "node-3", CreationTimestamp: metav1.Time{Time: now.Add(-200 * 24 * time.Hour)}},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{KernelVersion: "5.15.0-91", OSImage: "Ubuntu 20.04.6 LTS", Architecture: "amd64"},
			},
		},
		// GPU node with kernel A, OS A
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gpu-node-1", CreationTimestamp: metav1.Time{Time: now.Add(-10 * 24 * time.Hour)}},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{KernelVersion: "5.15.0-91", OSImage: "Ubuntu 22.04.3 LTS", Architecture: "amd64"},
				Allocatable: corev1.ResourceList{
					"nvidia.com/gpu": resource.MustParse("2"),
				},
			},
		},
		// ARM node (arch diversity)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "arm-node-1", CreationTimestamp: metav1.Time{Time: now.Add(-20 * 24 * time.Hour)}},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{KernelVersion: "5.15.0-91", OSImage: "Ubuntu 22.04.3 LTS", Architecture: "arm64"},
			},
		},
	}

	result := nodeLifecycleAuditCore(nodes)

	if result.Summary.TotalNodes != 5 {
		t.Errorf("expected totalNodes=5, got %d", result.Summary.TotalNodes)
	}
	if result.Summary.KernelVersions != 2 {
		t.Errorf("expected kernelVersions=2, got %d", result.Summary.KernelVersions)
	}
	if !result.Summary.KernelDrift {
		t.Error("expected kernelDrift=true")
	}
	if result.Summary.OSImages != 2 {
		t.Errorf("expected osImages=2, got %d", result.Summary.OSImages)
	}
	if !result.Summary.OSImageDrift {
		t.Error("expected osImageDrift=true")
	}
	if result.Summary.Archs != 2 {
		t.Errorf("expected archs=2 (amd64+arm64), got %d", result.Summary.Archs)
	}
	if !result.Summary.HasGPU {
		t.Error("expected hasGPU=true")
	}
	if result.Summary.GPUNodes != 1 {
		t.Errorf("expected gpuNodes=1, got %d", result.Summary.GPUNodes)
	}
	if len(result.GPUNodes) != 1 {
		t.Errorf("expected 1 GPU node entry, got %d", len(result.GPUNodes))
	}
	if result.GPUNodes[0].GPUCount != 2 {
		t.Errorf("expected gpuCount=2, got %d", result.GPUNodes[0].GPUCount)
	}
	if result.Summary.NodesOlderThan90d < 1 {
		t.Errorf("expected nodesOlderThan90d>=1, got %d", result.Summary.NodesOlderThan90d)
	}
	if result.Summary.NodesOlderThan180d < 1 {
		t.Errorf("expected nodesOlderThan180d>=1, got %d", result.Summary.NodesOlderThan180d)
	}
	if len(result.ByKernelVersion) != 2 {
		t.Errorf("expected 2 kernel version entries, got %d", len(result.ByKernelVersion))
	}
	if len(result.ByOSImage) != 2 {
		t.Errorf("expected 2 OS image entries, got %d", len(result.ByOSImage))
	}
	if len(result.ByArch) != 2 {
		t.Errorf("expected 2 arch entries, got %d", len(result.ByArch))
	}
	if len(result.Recommendations) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(result.Recommendations))
	}
}
