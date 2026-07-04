package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makePodSpecWithRefs(cms, secrets, pvcs []string) *corev1.PodSpec {
	spec := &corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name: "app",
				Env:  []corev1.EnvVar{},
			},
		},
	}

	for _, cm := range cms {
		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name: "cm-" + cm,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cm},
				},
			},
		})
	}

	for _, s := range secrets {
		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name: "sec-" + s,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: s},
			},
		})
	}

	for _, pvc := range pvcs {
		spec.Volumes = append(spec.Volumes, corev1.Volume{
			Name: "pvc-" + pvc,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvc},
			},
		})
	}

	return spec
}

func TestExtractConfigMapRefs(t *testing.T) {
	spec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
					},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name: "app",
				Env: []corev1.EnvVar{
					{
						Name: "DB_HOST",
						ValueFrom: &corev1.EnvVarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "db-config"},
								Key:                  "host",
							},
						},
					},
				},
				EnvFrom: []corev1.EnvFromSource{
					{
						ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "env-config"},
						},
					},
				},
			},
		},
		InitContainers: []corev1.Container{
			{
				Name: "init",
				EnvFrom: []corev1.EnvFromSource{
					{
						ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "init-config"},
						},
					},
				},
			},
		},
	}

	refs := extractConfigMapRefs(spec)

	if len(refs) != 4 {
		t.Fatalf("Expected 4 ConfigMap refs, got %d: %v", len(refs), refs)
	}

	expected := map[string]bool{"app-config": true, "db-config": true, "env-config": true, "init-config": true}
	for _, r := range refs {
		if !expected[r] {
			t.Errorf("Unexpected ConfigMap ref: %s", r)
		}
	}
}

func TestExtractConfigMapRefsDedup(t *testing.T) {
	spec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "v1",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "shared"},
					},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name: "c1",
				Env: []corev1.EnvVar{
					{
						Name: "X",
						ValueFrom: &corev1.EnvVarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "shared"},
								Key:                  "x",
							},
						},
					},
				},
			},
		},
	}

	refs := extractConfigMapRefs(spec)
	if len(refs) != 1 {
		t.Errorf("Expected 1 deduplicated ref, got %d", len(refs))
	}
}

func TestExtractSecretRefs(t *testing.T) {
	spec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{SecretName: "tls-cert"},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name: "app",
				Env: []corev1.EnvVar{
					{
						Name: "PASSWORD",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "db-pass"},
								Key:                  "password",
							},
						},
					},
				},
				EnvFrom: []corev1.EnvFromSource{
					{
						SecretRef: &corev1.SecretEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "app-secrets"},
						},
					},
				},
			},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "registry-cred"},
		},
	}

	refs := extractSecretRefs(spec)

	if len(refs) != 4 {
		t.Fatalf("Expected 4 Secret refs, got %d: %v", len(refs), refs)
	}

	expected := map[string]bool{"tls-cert": true, "db-pass": true, "app-secrets": true, "registry-cred": true}
	for _, r := range refs {
		if !expected[r] {
			t.Errorf("Unexpected Secret ref: %s", r)
		}
	}
}

func TestExtractPVCRefs(t *testing.T) {
	spec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name: "data",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "data-pvc"},
				},
			},
			{
				Name: "logs",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "logs-pvc"},
				},
			},
			{
				Name:         "empty",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
		},
	}

	refs := extractPVCRefs(spec)

	if len(refs) != 2 {
		t.Fatalf("Expected 2 PVC refs, got %d", len(refs))
	}
}

func TestExtractPVCRefsEmpty(t *testing.T) {
	spec := &corev1.PodSpec{
		Volumes: []corev1.Volume{
			{
				Name:         "empty",
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
		},
	}

	refs := extractPVCRefs(spec)
	if len(refs) != 0 {
		t.Errorf("Expected 0 PVC refs, got %d", len(refs))
	}
}

func TestLabelSelectorMatches(t *testing.T) {
	tests := []struct {
		name     string
		selector metav1.LabelSelector
		labels   map[string]string
		expect   bool
	}{
		{
			"exact-match",
			metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			map[string]string{"app": "web"},
			true,
		},
		{
			"no-match",
			metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			map[string]string{"app": "db"},
			false,
		},
		{
			"empty-selector",
			metav1.LabelSelector{},
			map[string]string{"app": "web"},
			false,
		},
		{
			"subset-labels",
			metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
			map[string]string{"app": "web", "env": "prod"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := labelSelectorMatches(tt.selector, tt.labels)
			if got != tt.expect {
				t.Errorf("Expected %v, got %v", tt.expect, got)
			}
		})
	}
}

func TestFindConfigMap(t *testing.T) {
	cms := &corev1.ConfigMapList{
		Items: []corev1.ConfigMap{
			{ObjectMeta: metav1.ObjectMeta{Name: "cm1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "cm2"}},
		},
	}

	if !findConfigMap(cms, "cm1") {
		t.Error("Expected to find cm1")
	}
	if findConfigMap(cms, "cm3") {
		t.Error("Should not find cm3")
	}
	if findConfigMap(nil, "cm1") {
		t.Error("nil list should return false")
	}
}

func TestFindSecret(t *testing.T) {
	secrets := &corev1.SecretList{
		Items: []corev1.Secret{
			{ObjectMeta: metav1.ObjectMeta{Name: "s1"}},
		},
	}

	if !findSecret(secrets, "s1") {
		t.Error("Expected to find s1")
	}
	if findSecret(secrets, "s2") {
		t.Error("Should not find s2")
	}
}

func TestFindPVC(t *testing.T) {
	pvcs := &corev1.PersistentVolumeClaimList{
		Items: []corev1.PersistentVolumeClaim{
			{ObjectMeta: metav1.ObjectMeta{Name: "p1"}},
		},
	}

	if !findPVC(pvcs, "p1") {
		t.Error("Expected to find p1")
	}
	if findPVC(pvcs, "p2") {
		t.Error("Should not find p2")
	}
}

func TestDependencyGraphSummary(t *testing.T) {
	g := DependencyGraph{
		Dependencies: []DepNode{
			{Kind: "ConfigMap", Name: "cm1"},
			{Kind: "Secret", Name: "s1"},
		},
		Dependents: []DepNode{
			{Kind: "Service", Name: "svc1"},
			{Kind: "Ingress", Name: "ing1"},
		},
	}

	g.Summary.TotalDependencies = len(g.Dependencies)
	g.Summary.TotalDependents = len(g.Dependents)
	g.Summary.BlastRadius = g.Summary.TotalDependencies + g.Summary.TotalDependents

	if g.Summary.BlastRadius != 4 {
		t.Errorf("Expected blast radius 4, got %d", g.Summary.BlastRadius)
	}
}
