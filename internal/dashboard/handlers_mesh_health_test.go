package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMeshHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		s        MeshHealthSummary
		minScore int
		maxScore int
	}{
		{"no mesh", MeshHealthSummary{}, 48, 52},
		{"istio all covered", MeshHealthSummary{HasIstio: true, TotalPods: 10, PodsWithSidecar: 10, MTLSEnabled: 10}, 90, 100},
		{"some no sidecar", MeshHealthSummary{HasIstio: true, TotalPods: 10, PodsWithSidecar: 7, PodsWithoutSidecar: 3}, 90, 98},
		{"mtls disabled", MeshHealthSummary{HasIstio: true, TotalPods: 10, PodsWithSidecar: 10, MTLSDisabled: 2}, 75, 85},
		{"sidecar restarts", MeshHealthSummary{HasLinkerd: true, TotalPods: 10, PodsWithSidecar: 10, SidecarRestarts: 3}, 80, 90},
		{"all bad", MeshHealthSummary{HasIstio: true, TotalPods: 20, PodsWithoutSidecar: 10, MTLSDisabled: 5, SidecarRestarts: 5, MTLSUnknown: 20}, 0, 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := meshHealthScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestMeshHealthRecommendations(t *testing.T) {
	t.Run("no mesh", func(t *testing.T) {
		recs := meshHealthRecommendations(MeshHealthSummary{})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := meshHealthRecommendations(MeshHealthSummary{
			HasIstio: true, PodsWithoutSidecar: 3, MTLSDisabled: 2, MTLSUnknown: 5, SidecarRestarts: 1,
		})
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
	t.Run("all healthy", func(t *testing.T) {
		recs := meshHealthRecommendations(MeshHealthSummary{
			HasIstio: true, PodsWithoutSidecar: 0, MTLSDisabled: 0, SidecarRestarts: 0, MTLSUnknown: 0,
		})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
}

func TestMeshHealthAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		// Istio control plane (istiod)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "istiod-abc", Namespace: "istio-system"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "istio/pilot:1.22.0"},
			}},
		},
		// Pod with istio-proxy sidecar and mTLS enabled
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-pod-1", Namespace: "default",
				Annotations: map[string]string{"security.istio.io/tlsMode": "istio"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.27"},
				{Name: "istio-proxy", Image: "istio/proxyv2:1.22.0"},
			}},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", Ready: true},
					{Name: "istio-proxy", Ready: true, RestartCount: 0},
				},
			},
		},
		// Pod with istio-proxy sidecar but mTLS disabled
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-pod-2", Namespace: "production",
				Annotations: map[string]string{"security.istio.io/tlsMode": "disable"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.27"},
				{Name: "istio-proxy", Image: "istio/proxyv2:1.22.0"},
			}},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "istio-proxy", RestartCount: 0},
				},
			},
		},
		// Pod without sidecar (mesh installed, should be flagged)
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bare-pod", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.27"},
			}},
		},
		// Pod with high restart sidecar
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "unstable-pod", Namespace: "production",
				Annotations: map[string]string{"security.istio.io/tlsMode": "istio"},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app", Image: "nginx:1.27"},
				{Name: "istio-proxy", Image: "istio/proxyv2:1.22.0"},
			}},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "app", RestartCount: 0},
					{Name: "istio-proxy", RestartCount: 10},
				},
			},
		},
	}

	result := meshHealthAuditCore(pods)

	if !result.Summary.HasIstio {
		t.Error("expected hasIstio=true")
	}
	if result.Summary.TotalPods != 5 {
		t.Errorf("expected totalPods=5, got %d", result.Summary.TotalPods)
	}
	// 3 pods with sidecar (excluding istiod control plane)
	if result.Summary.PodsWithSidecar != 3 {
		t.Errorf("expected podsWithSidecar=3, got %d", result.Summary.PodsWithSidecar)
	}
	// bare-pod has no sidecar
	if result.Summary.PodsWithoutSidecar != 1 {
		t.Errorf("expected podsWithoutSidecar=1, got %d", result.Summary.PodsWithoutSidecar)
	}
	// 2 pods with mTLS enabled
	if result.Summary.MTLSEnabled != 2 {
		t.Errorf("expected mtlsEnabled=2, got %d", result.Summary.MTLSEnabled)
	}
	// 1 pod with mTLS disabled
	if result.Summary.MTLSDisabled != 1 {
		t.Errorf("expected mtlsDisabled=1, got %d", result.Summary.MTLSDisabled)
	}
	// 1 sidecar with high restarts
	if result.Summary.SidecarRestarts != 1 {
		t.Errorf("expected sidecarRestarts=1, got %d", result.Summary.SidecarRestarts)
	}
	if len(result.Issues) < 3 {
		t.Errorf("expected at least 3 issues, got %d", len(result.Issues))
	}
	if len(result.Recommendations) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(result.Recommendations))
	}
}
