package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInitContainerScore(t *testing.T) {
	tests := []struct {
		name     string
		s        InitContainerSummary
		minScore int
		maxScore int
	}{
		{"no init containers", InitContainerSummary{}, 100, 100},
		{"all healthy", InitContainerSummary{TotalInitContainers: 5, PodsWithInit: 3}, 95, 100},
		{"missing resources", InitContainerSummary{TotalInitContainers: 5, MissingResources: 2}, 70, 85},
		{"missing limits", InitContainerSummary{TotalInitContainers: 5, MissingLimits: 3}, 85, 95},
		{"high risk", InitContainerSummary{TotalInitContainers: 5, HighRisk: 3}, 70, 80},
		{"all issues", InitContainerSummary{TotalInitContainers: 10, MissingResources: 3, MissingLimits: 5, ExcessiveRetries: 2, HighRisk: 2}, 0, 50},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := initContainerScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestInitContainerRecommendations(t *testing.T) {
	t.Run("no issues", func(t *testing.T) {
		recs := initContainerRecommendations(InitContainerSummary{PodsWithInit: 5, HighRisk: 0})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation for healthy state")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := initContainerRecommendations(InitContainerSummary{
			PodsWithInit: 5, MissingResources: 3, MissingLimits: 2, ExcessiveRetries: 1,
		})
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}

func TestInitContainerAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		// Good init container with resources and limits
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "good-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "app-deploy"},
				},
			},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name:  "init-db",
						Image: "busybox:1.36",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("64Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
				},
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx:1.27"},
				},
			},
		},
		// Pod with missing resource requests
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-req-pod",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "StatefulSet", Name: "app-sts"},
				},
			},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{
						Name:  "init-config",
						Image: "busybox:1.36",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
				},
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx:1.27"},
				},
			},
		},
		// Pod with too many init containers
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "many-inits-pod",
				Namespace: "production",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "Deployment", Name: "complex-app"},
				},
			},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{
					{Name: "init-1", Image: "busybox:1.36", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					}},
					{Name: "init-2", Image: "busybox:1.36", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					}},
					{Name: "init-3", Image: "busybox:1.36", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					}},
					{Name: "init-4", Image: "busybox:1.36", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					}},
					{Name: "init-5", Image: "busybox:1.36", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					}},
					{Name: "init-6", Image: "busybox:1.36", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					}},
					{Name: "init-7", Image: "busybox:1.36", Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("32Mi")},
						Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					}},
				},
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx:1.27"},
				},
			},
		},
		// Pod with no init containers
		{
			ObjectMeta: metav1.ObjectMeta{Name: "no-init-pod", Namespace: "default"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", Image: "nginx:1.27"},
				},
			},
		},
	}

	result := initContainerAuditCore(pods)

	if result.Summary.TotalPods != 4 {
		t.Errorf("expected totalPods=4, got %d", result.Summary.TotalPods)
	}
	if result.Summary.PodsWithInit != 3 {
		t.Errorf("expected podsWithInit=3, got %d", result.Summary.PodsWithInit)
	}
	if result.Summary.TotalInitContainers != 9 {
		t.Errorf("expected totalInitContainers=9, got %d", result.Summary.TotalInitContainers)
	}
	if result.Summary.MissingResources != 1 {
		t.Errorf("expected missingResources=1, got %d", result.Summary.MissingResources)
	}
	if result.Summary.MissingLimits != 0 {
		t.Errorf("expected missingLimits=0, got %d", result.Summary.MissingLimits)
	}
	if result.Summary.HighRisk < 2 {
		t.Errorf("expected highRisk>=2, got %d", result.Summary.HighRisk)
	}
	if len(result.Issues) < 2 {
		t.Errorf("expected at least 2 issues, got %d", len(result.Issues))
	}
	if len(result.ByNamespace) < 2 {
		t.Errorf("expected at least 2 namespace stats, got %d", len(result.ByNamespace))
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected recommendations")
	}
}

func TestPodOwnerInfo(t *testing.T) {
	t.Run("with owner", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pod",
				OwnerReferences: []metav1.OwnerReference{
					{Kind: "ReplicaSet", Name: "rs-123"},
				},
			},
		}
		name, typ := podOwnerInfo(pod)
		if name != "rs-123" || typ != "ReplicaSet" {
			t.Errorf("got (%s, %s), want (rs-123, ReplicaSet)", name, typ)
		}
	})
	t.Run("no owner", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "standalone-pod"},
		}
		name, typ := podOwnerInfo(pod)
		if name != "standalone-pod" || typ != "Pod" {
			t.Errorf("got (%s, %s), want (standalone-pod, Pod)", name, typ)
		}
	})
}
