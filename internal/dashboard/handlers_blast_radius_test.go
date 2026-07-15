package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeBlastRadius_LowRiskPod(t *testing.T) {
	nonRoot := true
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "safe", Namespace: "app"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app", SecurityContext: &corev1.SecurityContext{RunAsNonRoot: &nonRoot, AllowPrivilegeEscalation: boolPtr(false)}},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := analyzeBlastRadius(pods)

	if result.Summary.TotalPods != 1 {
		t.Errorf("expected 1 pod, got %d", result.Summary.TotalPods)
	}
	if result.Summary.LowRiskPods != 1 {
		t.Errorf("expected 1 low risk pod, got %d", result.Summary.LowRiskPods)
	}
	if result.Score < 90 {
		t.Errorf("expected score >= 90, got %d", result.Score)
	}
}

func TestAnalyzeBlastRadius_PrivilegedPod(t *testing.T) {
	priv := true
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "danger", Namespace: "app"},
			Spec: corev1.PodSpec{
				HostNetwork: true,
				HostPID:     true,
				Containers: []corev1.Container{
					{Name: "app", SecurityContext: &corev1.SecurityContext{Privileged: &priv}},
				},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := analyzeBlastRadius(pods)

	if result.Summary.PrivilegedPods != 1 {
		t.Errorf("expected 1 privileged pod, got %d", result.Summary.PrivilegedPods)
	}
	if result.Summary.HostNetworkPods != 1 {
		t.Errorf("expected 1 hostNetwork pod, got %d", result.Summary.HostNetworkPods)
	}
	if len(result.HighRiskPods) != 1 {
		t.Fatalf("expected 1 high-risk pod, got %d", len(result.HighRiskPods))
	}
	if result.HighRiskPods[0].RiskLevel != "critical" {
		t.Errorf("expected critical risk level, got %s", result.HighRiskPods[0].RiskLevel)
	}
}

func TestAnalyzeBlastRadius_HostPathContainerRuntime(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "docker-access", Namespace: "app"},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "docker-sock",
						VolumeSource: corev1.VolumeSource{
							HostPath: &corev1.HostPathVolumeSource{Path: "/var/run/docker.sock"},
						},
					},
				},
				Containers: []corev1.Container{{Name: "app"}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := analyzeBlastRadius(pods)

	// Check all attack vectors for hostPath detection
	foundHostPath := false
	foundRuntime := false
	for _, av := range result.AttackVectors {
		if av.Vector == "hostPath" {
			foundHostPath = true
		}
		if av.Vector == "hostPath:containerRuntime" {
			foundRuntime = true
		}
	}
	if !foundHostPath {
		t.Error("expected hostPath vector detected")
	}
	if !foundRuntime {
		t.Error("expected hostPath:containerRuntime vector detected")
	}
}

func TestAnalyzeBlastRadius_NamespaceStats(t *testing.T) {
	nonRoot := true
	priv := true
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "safe", Namespace: "prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{RunAsNonRoot: &nonRoot}}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "danger", Namespace: "prod"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", SecurityContext: &corev1.SecurityContext{Privileged: &priv}}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := analyzeBlastRadius(pods)

	if len(result.ByNamespace) != 1 {
		t.Fatalf("expected 1 namespace stat, got %d", len(result.ByNamespace))
	}
	if result.ByNamespace[0].Namespace != "prod" {
		t.Errorf("expected prod namespace, got %s", result.ByNamespace[0].Namespace)
	}
	if result.ByNamespace[0].PodCount != 2 {
		t.Errorf("expected 2 pods, got %d", result.ByNamespace[0].PodCount)
	}
	if result.ByNamespace[0].HighRisk != 1 {
		t.Errorf("expected 1 high-risk pod in namespace, got %d", result.ByNamespace[0].HighRisk)
	}
}

func TestVectorSeverity(t *testing.T) {
	if vectorSeverity("privileged") != "critical" {
		t.Error("expected critical for privileged")
	}
	if vectorSeverity("hostNetwork") != "high" {
		t.Error("expected high for hostNetwork")
	}
	if vectorSeverity("hostPath") != "medium" {
		t.Error("expected medium for hostPath")
	}
}
