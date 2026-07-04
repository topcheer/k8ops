package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAuditPodSecurityPrivileged(t *testing.T) {
	priv := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "priv-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx:v1",
					SecurityContext: &corev1.SecurityContext{
						Privileged: &priv,
					},
				},
			},
		},
	}

	psp := auditPodSecurity(pod)

	found := false
	for _, f := range psp.Findings {
		if f.Check == SecCheckPrivileged && f.Severity == PodSecCritical {
			found = true
		}
	}
	if !found {
		t.Error("Expected critical privileged finding")
	}
	if psp.RiskScore < 25 {
		t.Errorf("Expected risk score >= 25, got %d", psp.RiskScore)
	}
}

func TestAuditPodSecurityHostAccess(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "host-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			HostNetwork: true,
			HostPID:     true,
			HostIPC:     true,
			Volumes: []corev1.Volume{
				{
					Name: "host",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{Path: "/"},
					},
				},
			},
			Containers: []corev1.Container{
				{Name: "app", Image: "app:v1"},
			},
		},
	}

	psp := auditPodSecurity(pod)

	checks := make(map[PodSecCheck]bool)
	for _, f := range psp.Findings {
		checks[f.Check] = true
	}

	for _, expected := range []PodSecCheck{SecCheckHostNetwork, SecCheckHostPID, SecCheckHostIPC, SecCheckHostPath} {
		if !checks[expected] {
			t.Errorf("Expected finding %s", expected)
		}
	}
}

func TestAuditPodSecurityRootAndPrivEsc(t *testing.T) {
	allowEsc := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "root-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "app:v1",
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &allowEsc,
					},
				},
			},
		},
	}

	psp := auditPodSecurity(pod)

	checks := make(map[PodSecCheck]bool)
	for _, f := range psp.Findings {
		checks[f.Check] = true
	}

	if !checks[SecCheckPrivEscalation] {
		t.Error("Expected privilege escalation finding")
	}
	if !checks[SecCheckRunAsRoot] {
		t.Error("Expected runs-as-root finding")
	}
}

func TestAuditPodSecurityCleanPod(t *testing.T) {
	nonRoot := true
	readOnly := true
	noEsc := false
	uid := int64(1000)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "clean-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &nonRoot,
				RunAsUser:    &uid,
			},
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "myapp@sha256:abc123def456",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
					SecurityContext: &corev1.SecurityContext{
						RunAsNonRoot:             &nonRoot,
						RunAsUser:                &uid,
						ReadOnlyRootFilesystem:   &readOnly,
						AllowPrivilegeEscalation: &noEsc,
					},
				},
			},
		},
	}

	psp := auditPodSecurity(pod)

	criticalCount := 0
	warningCount := 0
	for _, f := range psp.Findings {
		if f.Severity == PodSecCritical {
			criticalCount++
		}
		if f.Severity == PodSecWarning {
			warningCount++
		}
	}

	if criticalCount > 0 {
		t.Errorf("Expected 0 critical findings for clean pod, got %d", criticalCount)
	}
	if warningCount > 0 {
		t.Errorf("Expected 0 warning findings for clean pod, got %d", warningCount)
	}
	if psp.RiskScore > 5 {
		t.Errorf("Expected risk score <= 5 for clean pod, got %d", psp.RiskScore)
	}
}

func TestAuditPodSecurityImageChecks(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		expectCheck PodSecCheck
	}{
		{"latest", "nginx:latest", SecCheckImageLatest},
		{"no-tag", "nginx", SecCheckImageNoTag},
		{"digest", "nginx@sha256:abc123", ""}, // no finding expected
		{"pinned", "nginx:v1.2.3", ""},        // no finding expected
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			psp := PodSecPod{}
			auditImageSecurity(&psp, corev1.Container{Name: "c", Image: tt.image})

			found := false
			for _, f := range psp.Findings {
				if f.Check == tt.expectCheck {
					found = true
				}
			}

			if tt.expectCheck != "" && !found {
				t.Errorf("Expected finding %s for image %s", tt.expectCheck, tt.image)
			}
			if tt.expectCheck == "" && len(psp.Findings) > 0 {
				for _, f := range psp.Findings {
					if f.Check == SecCheckImageLatest || f.Check == SecCheckImageNoTag {
						t.Errorf("Did not expect image finding for %s, got %s", tt.image, f.Check)
					}
				}
			}
		})
	}
}

func TestAuditPodSecurityDangerousCaps(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "cap-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "app:v1",
					SecurityContext: &corev1.SecurityContext{
						Capabilities: &corev1.Capabilities{
							Add: []corev1.Capability{"SYS_ADMIN", "NET_ADMIN"},
						},
					},
				},
			},
		},
	}

	psp := auditPodSecurity(pod)

	capCount := 0
	for _, f := range psp.Findings {
		if f.Check == SecCheckCapabilities {
			capCount++
		}
	}
	if capCount != 2 {
		t.Errorf("Expected 2 dangerous capability findings, got %d", capCount)
	}
}

func TestAuditPodSecurityHostPort(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "port-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "app:v1",
					Ports: []corev1.ContainerPort{
						{ContainerPort: 80, HostPort: 80},
					},
				},
			},
		},
	}

	psp := auditPodSecurity(pod)

	found := false
	for _, f := range psp.Findings {
		if f.Check == SecCheckHostPort {
			found = true
		}
	}
	if !found {
		t.Error("Expected host-port finding")
	}
}

func TestAuditPodSecuritySecretEnv(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "app:v1",
					Env: []corev1.EnvVar{
						{
							Name: "DB_PASSWORD",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "db-secret"},
									Key:                  "password",
								},
							},
						},
					},
				},
			},
		},
	}

	psp := auditPodSecurity(pod)

	found := false
	for _, f := range psp.Findings {
		if f.Check == SecCheckSecretEnv {
			found = true
		}
	}
	if !found {
		t.Error("Expected secret-env finding")
	}
}

func TestPodKind(t *testing.T) {
	tests := []struct {
		name   string
		owners []metav1.OwnerReference
		expect string
	}{
		{"no-owner", nil, "Pod"},
		{"deployment", []metav1.OwnerReference{{Kind: "ReplicaSet"}}, "ReplicaSet"},
		{"daemonset", []metav1.OwnerReference{{Kind: "DaemonSet"}}, "DaemonSet"},
	}

	for _, tt := range tests {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test", OwnerReferences: tt.owners},
		}
		got := podKind(pod)
		if got != tt.expect {
			t.Errorf("podKind(%s) = %s, want %s", tt.name, got, tt.expect)
		}
	}
}

func TestPodSecRiskScoreOrdering(t *testing.T) {
	psp := PodSecPod{}
	psp.addFinding(PodSecCritical, SecCheckPrivileged, "c", "msg", "fix")
	psp.addFinding(PodSecWarning, SecCheckRunAsRoot, "c", "msg", "fix")
	psp.addFinding(PodSecInfo, SecCheckReadOnlyFS, "c", "msg", "fix")

	// 25 + 8 + 2 = 35
	if psp.RiskScore != 35 {
		t.Errorf("Expected risk score 35, got %d", psp.RiskScore)
	}
}
