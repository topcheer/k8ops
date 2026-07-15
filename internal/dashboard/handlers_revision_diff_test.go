package dashboard

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeRevisionDiff_HealthyDeployment(t *testing.T) {
	replicas := int32(3)
	runAsNonRoot := true
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "healthy-app", Namespace: "prod",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "3"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{},
							},
							ReadinessProbe: &corev1.Probe{},
							LivenessProbe:  &corev1.Probe{},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: &runAsNonRoot,
							},
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{UpdatedReplicas: 3},
	}

	result := analyzeRevisionDiff([]appsv1.Deployment{dep})

	if result.Score < 95 {
		t.Errorf("expected score >= 95 for healthy deployment, got %d", result.Score)
	}
}

func TestAnalyzeRevisionDiff_MissingProbes(t *testing.T) {
	replicas := int32(1)
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-probes", Namespace: "default",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", SecurityContext: &corev1.SecurityContext{RunAsNonRoot: boolPtr(true)}},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{UpdatedReplicas: 1},
	}

	result := analyzeRevisionDiff([]appsv1.Deployment{dep})

	// Should detect missing readiness + liveness probes
	foundReadiness := false
	foundLiveness := false
	for _, wd := range result.WorkloadDiffs {
		for _, c := range wd.Changes {
			if c.Type == "MissingProbe" && c.Field == "container[app].readinessProbe" {
				foundReadiness = true
			}
			if c.Type == "MissingProbe" && c.Field == "container[app].livenessProbe" {
				foundLiveness = true
			}
		}
	}
	if !foundReadiness {
		t.Error("expected missing readiness probe detection")
	}
	if !foundLiveness {
		t.Error("expected missing liveness probe detection")
	}
}

func TestAnalyzeRevisionDiff_PrivilegedContainer(t *testing.T) {
	replicas := int32(1)
	privileged := true
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "priv-app", Namespace: "default",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "2"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
							},
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{UpdatedReplicas: 1},
	}

	result := analyzeRevisionDiff([]appsv1.Deployment{dep})

	if result.Summary.BreakingChangeCount == 0 {
		t.Error("expected at least 1 breaking change for privileged container")
	}
	if result.Score >= 82 {
		t.Errorf("expected score < 82 for privileged container, got %d", result.Score)
	}
}

func TestAnalyzeRevisionDiff_RecreateStrategy(t *testing.T) {
	replicas := int32(1)
	runAsNonRoot := true
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "recreate-app", Namespace: "default",
			Annotations: map[string]string{"deployment.kubernetes.io/revision": "1"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "app",
							SecurityContext: &corev1.SecurityContext{RunAsNonRoot: &runAsNonRoot},
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{UpdatedReplicas: 1},
	}

	result := analyzeRevisionDiff([]appsv1.Deployment{dep})

	foundBreaking := false
	for _, bc := range result.BreakingChanges {
		if bc.Change == "Recreate strategy" {
			foundBreaking = true
		}
	}
	if !foundBreaking {
		t.Error("expected breaking change for Recreate strategy")
	}
}

func TestRevRiskLevelRank(t *testing.T) {
	if revRiskLevelRank("critical") != 4 {
		t.Error("expected rank 4 for critical")
	}
	if revRiskLevelRank("high") != 3 {
		t.Error("expected rank 3 for high")
	}
	if revRiskLevelRank("medium") != 2 {
		t.Error("expected rank 2 for medium")
	}
	if revRiskLevelRank("low") != 1 {
		t.Error("expected rank 1 for low")
	}
}
