package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCostWasteScore(t *testing.T) {
	tests := []struct {
		name     string
		s        CostWasteSummary
		minScore int
		maxScore int
	}{
		{"no pods", CostWasteSummary{}, 95, 100},
		{"all efficient", CostWasteSummary{TotalPods: 10, WastePercent: 2}, 85, 100},
		{"some idle", CostWasteSummary{TotalPods: 10, IdlePods: 3, WastePercent: 30}, 60, 80},
		{"idle namespaces", CostWasteSummary{TotalPods: 10, IdleNamespaces: 2}, 70, 85},
		{"over-provisioned", CostWasteSummary{TotalPods: 10, OverProvPods: 3}, 85, 95},
		{"all bad", CostWasteSummary{TotalPods: 20, IdlePods: 10, IdleNamespaces: 3, OverProvPods: 5, WastePercent: 50}, 0, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := costWasteScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestCostWasteRecommendations(t *testing.T) {
	t.Run("all efficient", func(t *testing.T) {
		recs := costWasteRecommendations(CostWasteSummary{TotalPods: 10, WastePercent: 2})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := costWasteRecommendations(CostWasteSummary{
			IdlePods: 3, IdleNamespaces: 1, OverProvPods: 2, WastePercent: 25,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestCostWasteAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		// Normal pod with reasonable requests
		{
			ObjectMeta: metav1.ObjectMeta{Name: "normal-pod", Namespace: "app-ns"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			}},
		},
		// Idle pod (very low requests)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "idle-pod", Namespace: "app-ns"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("50m"),
							corev1.ResourceMemory: resource.MustParse("64Mi"),
						},
					},
				},
			}},
		},
		// Over-provisioned pod (high CPU request)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "greedy-pod", Namespace: "app-ns"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("8"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			}},
		},
		// Pod in idle namespace (all pods idle)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "idle-ns-pod", Namespace: "idle-ns"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"),
							corev1.ResourceMemory: resource.MustParse("32Mi"),
						},
					},
				},
			}},
		},
	}

	result := costWasteAuditCore(pods)

	if result.Summary.TotalPods != 4 {
		t.Errorf("expected totalPods=4, got %d", result.Summary.TotalPods)
	}
	if result.Summary.IdlePods < 2 {
		t.Errorf("expected idlePods>=2, got %d", result.Summary.IdlePods)
	}
	if result.Summary.OverProvPods < 1 {
		t.Errorf("expected overProvPods>=1, got %d", result.Summary.OverProvPods)
	}
	if result.Summary.IdleNamespaces < 1 {
		t.Errorf("expected idleNamespaces>=1, got %d", result.Summary.IdleNamespaces)
	}
	if len(result.IdleResources) < 2 {
		t.Errorf("expected at least 2 idle resources, got %d", len(result.IdleResources))
	}
	if len(result.OverProvisioned) < 1 {
		t.Errorf("expected at least 1 over-provisioned entry, got %d", len(result.OverProvisioned))
	}
	if len(result.ByNamespace) < 2 {
		t.Errorf("expected at least 2 namespace stats, got %d", len(result.ByNamespace))
	}
	if len(result.Recommendations) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(result.Recommendations))
	}
}
