package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeMeshInjection_NoMesh(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "app"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
	}

	result := analyzeMeshInjection(pods, namespaces)

	if result.MeshType != "none" {
		t.Errorf("expected mesh type 'none', got %s", result.MeshType)
	}
	if result.Score != 100 {
		t.Errorf("expected score 100 (no mesh, no penalty), got %d", result.Score)
	}
}

func TestAnalyzeMeshInjection_IstioInjection(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "meshed", Namespace: "app"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app"},
					{Name: "istio-proxy", Image: "istio/proxyv2:1.20.0"},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "unmeshed", Namespace: "app"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "app", Labels: map[string]string{"istio-injection": "enabled"}}},
	}

	result := analyzeMeshInjection(pods, namespaces)

	if result.MeshType != "istio" {
		t.Errorf("expected mesh type istio, got %s", result.MeshType)
	}
	if !result.Summary.MeshDetected {
		t.Error("expected mesh detected")
	}
	if result.Summary.TotalPods != 2 {
		t.Errorf("expected 2 pods, got %d", result.Summary.TotalPods)
	}
	if result.Summary.InjectedPods != 1 {
		t.Errorf("expected 1 injected pod, got %d", result.Summary.InjectedPods)
	}
	if len(result.InjectionGaps) != 1 {
		t.Fatalf("expected 1 injection gap, got %d", len(result.InjectionGaps))
	}
	if result.InjectionGaps[0].Pod != "unmeshed" {
		t.Errorf("expected injection gap for 'unmeshed', got %s", result.InjectionGaps[0].Pod)
	}
}

func TestAnalyzeMeshInjection_OptedOutPod(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "opted-out", Namespace: "app",
				Annotations: map[string]string{"sidecar.istio.io/inject": "false"},
			},
			Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "app", Labels: map[string]string{"istio-injection": "enabled"}}},
	}

	result := analyzeMeshInjection(pods, namespaces)

	if result.Summary.OptedOutPods != 1 {
		t.Errorf("expected 1 opted out pod, got %d", result.Summary.OptedOutPods)
	}
	// Opted-out pods should NOT generate injection gap
	if len(result.InjectionGaps) != 0 {
		t.Errorf("expected 0 injection gaps for opted-out pod, got %d", len(result.InjectionGaps))
	}
}

func TestAnalyzeMeshInjection_NamespaceStats(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "meshed"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "istio-proxy"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "unmeshed"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "meshed", Labels: map[string]string{"istio-injection": "enabled"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "unmeshed"}},
	}

	result := analyzeMeshInjection(pods, namespaces)

	if len(result.NamespaceCoverage) < 2 {
		t.Fatalf("expected >=2 namespace stats, got %d", len(result.NamespaceCoverage))
	}

	foundMeshed := false
	foundUnmeshed := false
	for _, ns := range result.NamespaceCoverage {
		if ns.Namespace == "meshed" && ns.Status == "fully-meshed" {
			foundMeshed = true
		}
		if ns.Namespace == "unmeshed" && ns.Status == "unmeshed" {
			foundUnmeshed = true
		}
	}
	if !foundMeshed {
		t.Error("expected meshed namespace to be 'fully-meshed'")
	}
	if !foundUnmeshed {
		t.Error("expected unmeshed namespace to be 'unmeshed'")
	}
}

func TestAnalyzeMeshInjection_LinkerdDetection(t *testing.T) {
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "app", Annotations: map[string]string{"linkerd.io/inject": "enabled"}}},
	}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "app"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Name: "app"},
				{Name: "linkerd-proxy", Image: "ghcr.io/linkerd/proxy"},
			}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := analyzeMeshInjection(pods, namespaces)

	if result.MeshType != "linkerd" {
		t.Errorf("expected mesh type linkerd, got %s", result.MeshType)
	}
	if result.Summary.InjectedPods != 1 {
		t.Errorf("expected 1 injected pod, got %d", result.Summary.InjectedPods)
	}
	if result.Summary.InjectionRate != 100 {
		t.Errorf("expected 100%% injection rate, got %.1f%%", result.Summary.InjectionRate)
	}
}
