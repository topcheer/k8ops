package dashboard

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestAuditDeploymentAllChecks(t *testing.T) {
	// A deployment with every possible misconfiguration
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bad-app",
			Namespace:         "default",
			CreationTimestamp: metav1.Time{},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             int32Ptr(1),
			RevisionHistoryLimit: int32Ptr(1), // too low
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType, // causes downtime
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: int64Ptr(5), // too short
					Containers: []corev1.Container{
						{
							Name:            "app",
							Image:           "nginx:latest",
							ImagePullPolicy: corev1.PullIfNotPresent, // wrong for :latest
							// No probes, no resources, no security context
						},
					},
				},
			},
		},
	}

	wa := auditDeployment(dep)

	// Should have many findings
	if len(wa.Findings) < 8 {
		t.Errorf("Expected at least 8 findings for fully misconfigured deployment, got %d", len(wa.Findings))
	}

	// Verify specific checks
	checks := make(map[string]bool)
	for _, f := range wa.Findings {
		checks[f.Check] = true
	}

	expectedChecks := []string{
		"missing-liveness-probe",
		"missing-readiness-probe",
		"missing-resource-limits",
		"missing-resource-requests",
		"latest-tag-without-always",
		"insufficient-revision-history",
		"recreate-strategy",
		"short-grace-period",
		"runs-as-root",
		"missing-prestop-hook",
	}
	for _, check := range expectedChecks {
		if !checks[check] {
			t.Errorf("Expected finding %q to be present", check)
		}
	}

	// Score should be high (bad)
	if wa.Score < 60 {
		t.Errorf("Expected score >= 60 for fully misconfigured deployment, got %d", wa.Score)
	}
}

func TestAuditDeploymentCleanConfig(t *testing.T) {
	// A deployment with best-practice configuration
	runAsNonRoot := true
	readOnlyRoot := true
	allowPrivEsc := false

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "good-app",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:             int32Ptr(3),
			RevisionHistoryLimit: int32Ptr(10),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: int64Ptr(30),
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot:   &runAsNonRoot,
						SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
					},
					Containers: []corev1.Container{
						{
							Name:            "app",
							Image:           "myapp:v1.2.3", // pinned tag
							ImagePullPolicy: corev1.PullIfNotPresent,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("250m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{Path: "/health", Port: intstr.FromInt(8080)},
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{Path: "/ready", Port: intstr.FromInt(8080)},
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             &runAsNonRoot,
								ReadOnlyRootFilesystem:   &readOnlyRoot,
								AllowPrivilegeEscalation: &allowPrivEsc,
							},
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{Command: []string{"sleep", "5"}},
								},
							},
						},
					},
				},
			},
		},
	}

	wa := auditDeployment(dep)

	// Clean deployment should have very few findings (maybe some info-level ones)
	criticalCount := 0
	warningCount := 0
	for _, f := range wa.Findings {
		if f.Severity == DeployAuditCritical {
			criticalCount++
		}
		if f.Severity == DeployAuditWarning {
			warningCount++
		}
	}

	if criticalCount > 0 {
		t.Errorf("Expected 0 critical findings for clean deployment, got %d", criticalCount)
	}
	if warningCount > 0 {
		t.Errorf("Expected 0 warning findings for clean deployment, got %d", warningCount)
	}
	if wa.Score > 10 {
		t.Errorf("Expected score <= 10 for clean deployment, got %d", wa.Score)
	}
}

func TestAuditStatefulSetOnDelete(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(3),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.OnDeleteStatefulSetStrategyType,
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "db",
							Image: "postgres:14",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(5432)}},
							},
						},
					},
				},
			},
		},
	}

	wa := auditStatefulSet(sts)

	foundOnDelete := false
	for _, f := range wa.Findings {
		if f.Check == "ondelete-strategy" {
			foundOnDelete = true
		}
	}
	if !foundOnDelete {
		t.Error("Expected ondelete-strategy finding for StatefulSet with OnDelete strategy")
	}
}

func TestAuditStatefulSetPartitionedRollout(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
		Spec: appsv1.StatefulSetSpec{
			Replicas: int32Ptr(5),
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: int32Ptr(3),
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "db",
							Image: "postgres:14",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(5432)}},
							},
						},
					},
				},
			},
		},
	}

	wa := auditStatefulSet(sts)

	foundPartition := false
	for _, f := range wa.Findings {
		if f.Check == "partitioned-rollout" {
			foundPartition = true
		}
	}
	if !foundPartition {
		t.Error("Expected partitioned-rollout finding")
	}
}

func TestAuditDaemonSetBasic(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "kube-system"},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "agent",
							Image: "fluentd:latest",
							// No resources, no probes
						},
					},
				},
			},
		},
	}

	wa := auditDaemonSet(ds)

	if wa.Kind != "DaemonSet" {
		t.Errorf("Expected kind DaemonSet, got %s", wa.Kind)
	}
	if len(wa.Findings) == 0 {
		t.Error("Expected findings for misconfigured DaemonSet")
	}
}

func TestDeployAuditPrivilegedContainer(t *testing.T) {
	priv := true
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "privileged-app", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "app:v1",
							SecurityContext: &corev1.SecurityContext{
								Privileged: &priv,
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
							LivenessProbe:  &corev1.Probe{},
							ReadinessProbe: &corev1.Probe{},
						},
					},
				},
			},
		},
	}

	wa := auditDeployment(dep)

	foundPrivileged := false
	for _, f := range wa.Findings {
		if f.Check == "privileged-container" && f.Severity == DeployAuditCritical {
			foundPrivileged = true
		}
	}
	if !foundPrivileged {
		t.Error("Expected critical privileged-container finding")
	}
}

func TestDeployAuditRevisionHistoryDefaults(t *testing.T) {
	// Nil revisionHistoryLimit should default to 10 (Kubernetes default)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "default-rev", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "app:v1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
							LivenessProbe:  &corev1.Probe{},
							ReadinessProbe: &corev1.Probe{},
						},
					},
				},
			},
		},
	}

	wa := auditDeployment(dep)

	// With default revision history (10), should not have revision history findings
	for _, f := range wa.Findings {
		if f.Category == CatRevisionHistory {
			t.Errorf("Did not expect revision-history finding with default limit, got: %s - %s", f.Check, f.Message)
		}
	}
}

func TestDeployAuditImagePolicyChecks(t *testing.T) {
	tests := []struct {
		name          string
		image         string
		pullPolicy    corev1.PullPolicy
		expectFinding bool
		expectCheck   string
	}{
		{
			name:          "latest-tag-IfNotPresent",
			image:         "nginx:latest",
			pullPolicy:    corev1.PullIfNotPresent,
			expectFinding: true,
			expectCheck:   "latest-tag-without-always",
		},
		{
			name:          "latest-tag-Always",
			image:         "nginx:latest",
			pullPolicy:    corev1.PullAlways,
			expectFinding: false,
		},
		{
			name:          "latest-tag-default-policy",
			image:         "nginx:latest",
			pullPolicy:    "", // defaults to Always for :latest
			expectFinding: false,
		},
		{
			name:          "pinned-tag-Always",
			image:         "nginx:v1.2.3",
			pullPolicy:    corev1.PullAlways,
			expectFinding: true,
			expectCheck:   "pinned-tag-with-always",
		},
		{
			name:          "pinned-tag-IfNotPresent",
			image:         "nginx:v1.2.3",
			pullPolicy:    corev1.PullIfNotPresent,
			expectFinding: false,
		},
		{
			name:          "digest-image",
			image:         "nginx@sha256:abc123def456789",
			pullPolicy:    corev1.PullIfNotPresent,
			expectFinding: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wa := DeployAuditWorkload{}
			auditImagePullPolicy(&wa, corev1.Container{
				Name:            "test",
				Image:           tt.image,
				ImagePullPolicy: tt.pullPolicy,
			})

			found := false
			for _, f := range wa.Findings {
				if tt.expectCheck != "" && f.Check == tt.expectCheck {
					found = true
				}
			}

			if tt.expectFinding && !found {
				t.Errorf("Expected finding %q for image=%s policy=%s", tt.expectCheck, tt.image, tt.pullPolicy)
			}
			if !tt.expectFinding && len(wa.Findings) > 0 {
				t.Errorf("Expected no finding for image=%s policy=%s, but got %d findings", tt.image, tt.pullPolicy, len(wa.Findings))
			}
		})
	}
}

func TestDeployAuditScoreOrdering(t *testing.T) {
	// Critical findings should score higher than warnings, which score higher than info
	wa := DeployAuditWorkload{}
	wa.addFinding(DeployAuditInfo, CatLifecycle, "test-info", "info", "fix")
	wa.addFinding(DeployAuditWarning, CatProbes, "test-warn", "warn", "fix")
	wa.addFinding(DeployAuditCritical, CatSecurity, "test-crit", "crit", "fix")

	// 20 + 8 + 2 = 30
	if wa.Score != 30 {
		t.Errorf("Expected score 30, got %d", wa.Score)
	}
}

func TestDeployAuditLongGracePeriod(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "long-grpc", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: int64Ptr(600),
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "app:v1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
							LivenessProbe:  &corev1.Probe{},
							ReadinessProbe: &corev1.Probe{},
						},
					},
				},
			},
		},
	}

	wa := auditDeployment(dep)

	foundLongGrpc := false
	for _, f := range wa.Findings {
		if f.Check == "long-grace-period" {
			foundLongGrpc = true
		}
	}
	if !foundLongGrpc {
		t.Error("Expected long-grace-period finding for grace period > 300s")
	}
}

func TestDeployAuditExcessiveRevisionHistory(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "many-revs", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas:             int32Ptr(1),
			RevisionHistoryLimit: int32Ptr(50),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "app:v1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
							LivenessProbe:  &corev1.Probe{},
							ReadinessProbe: &corev1.Probe{},
						},
					},
				},
			},
		},
	}

	wa := auditDeployment(dep)

	foundExcessive := false
	for _, f := range wa.Findings {
		if f.Check == "excessive-revision-history" {
			foundExcessive = true
		}
	}
	if !foundExcessive {
		t.Error("Expected excessive-revision-history finding for limit > 20")
	}
}

func TestDeployAuditAggressiveMaxUnavailable(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "aggressive", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(2),
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "100%"},
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "app:v1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
							},
							LivenessProbe:  &corev1.Probe{},
							ReadinessProbe: &corev1.Probe{},
						},
					},
				},
			},
		},
	}

	wa := auditDeployment(dep)

	foundAggressive := false
	for _, f := range wa.Findings {
		if f.Check == "aggressive-max-unavailable" {
			foundAggressive = true
		}
	}
	if !foundAggressive {
		t.Error("Expected aggressive-max-unavailable finding")
	}
}

// --- Test helpers ---

func int32Ptr(i int32) *int32 { return &i }
func int64Ptr(i int64) *int64 { return &i }
