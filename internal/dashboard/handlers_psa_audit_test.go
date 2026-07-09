package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPSAScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  PSAAuditSummary
		minScore int
		maxScore int
	}{
		{
			name: "all enforced restricted",
			summary: PSAAuditSummary{
				TotalNamespaces:    10,
				UserNamespaces:     8,
				Enforced:           10,
				RestrictedEnforced: 8,
				BaselineEnforced:   2,
				HasAuditMode:       10,
				HasWarnMode:        10,
			},
			minScore: 75,
			maxScore: 100,
		},
		{
			name: "none enforced",
			summary: PSAAuditSummary{
				TotalNamespaces:   10,
				UserNamespaces:    8,
				NotEnforced:       10,
				PrivilegedAllowed: 10,
			},
			minScore: 0,
			maxScore: 20,
		},
		{
			name: "partial enforcement",
			summary: PSAAuditSummary{
				TotalNamespaces:    10,
				UserNamespaces:     8,
				Enforced:           5,
				RestrictedEnforced: 3,
				BaselineEnforced:   2,
				HasAuditMode:       5,
				ViolationCount:     3,
			},
			minScore: 25,
			maxScore: 65,
		},
		{
			name: "no user namespaces",
			summary: PSAAuditSummary{
				TotalNamespaces: 5,
				UserNamespaces:  0,
				Enforced:        5,
			},
			minScore: 95,
			maxScore: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := psaScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestGetPSALevel(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		prefix      string
		want        PSAEnforcementLevel
	}{
		{"enforce restricted", map[string]string{"pod-security.kubernetes.io/enforce": "restricted"}, psaEnforcePrefix, PSALevelRestricted},
		{"enforce baseline", map[string]string{"pod-security.kubernetes.io/enforce": "baseline"}, psaEnforcePrefix, PSALevelBaseline},
		{"enforce privileged", map[string]string{"pod-security.kubernetes.io/enforce": "privileged"}, psaEnforcePrefix, PSALevelPrivileged},
		{"missing label", map[string]string{}, psaEnforcePrefix, PSALevelNone},
		{"invalid value", map[string]string{"pod-security.kubernetes.io/enforce": "custom"}, psaEnforcePrefix, PSALevelNone},
		{"empty value", map[string]string{"pod-security.kubernetes.io/enforce": ""}, psaEnforcePrefix, PSALevelNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getPSALevel(tt.annotations, tt.prefix)
			if got != tt.want {
				t.Errorf("getPSALevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsDangerousCapability(t *testing.T) {
	tests := []struct {
		cap  string
		want bool
	}{
		{"CAP_SYS_ADMIN", true},
		{"SYS_ADMIN", true},
		{"sys_admin", true},
		{"CAP_NET_ADMIN", true},
		{"NET_ADMIN", true},
		{"CAP_CHOWN", false},
		{"CHOWN", false},
		{"CAP_FOWNER", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isDangerousCapability(tt.cap)
		if got != tt.want {
			t.Errorf("isDangerousCapability(%q) = %v, want %v", tt.cap, got, tt.want)
		}
	}
}

func TestCheckPSAViolationsBaseline(t *testing.T) {
	privileged := true
	hostNet := true

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			HostNetwork: hostNet,
			Containers: []corev1.Container{
				{
					Name: "app",
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
				},
			},
		},
	}

	violations := checkPSAViolations(pod, PSALevelBaseline)
	if len(violations) < 2 {
		t.Errorf("expected at least 2 violations (privileged + hostNetwork), got %d", len(violations))
	}

	// Check that privileged violation exists
	foundPriv := false
	foundHostNet := false
	for _, v := range violations {
		if v.category == "privileged" {
			foundPriv = true
		}
		if v.category == "host-network" {
			foundHostNet = true
		}
	}
	if !foundPriv {
		t.Error("expected privileged violation")
	}
	if !foundHostNet {
		t.Error("expected host-network violation")
	}
}

func TestCheckPSAViolationsRestricted(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restricted-pod",
			Namespace: "secure-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "app",
					SecurityContext: nil, // no security context at all
				},
			},
		},
	}

	violations := checkPSAViolations(pod, PSALevelRestricted)

	// Should find: runs-as-root, privilege-escalation, capabilities-not-dropped, missing-seccomp
	found := map[string]bool{}
	for _, v := range violations {
		found[v.category] = true
	}

	if !found["runs-as-root"] {
		t.Error("expected runs-as-root violation")
	}
	if !found["privilege-escalation"] {
		t.Error("expected privilege-escalation violation")
	}
	if !found["capabilities-not-dropped"] {
		t.Error("expected capabilities-not-dropped violation")
	}
	if !found["missing-seccomp"] {
		t.Error("expected missing-seccomp violation")
	}
}

func TestCheckPSAViolationsClean(t *testing.T) {
	nonRoot := true
	privEscFalse := false
	runAsUser := int64(1000)
	all := corev1.Capability("ALL")
	seccomp := &corev1.SeccompProfile{
		Type: corev1.SeccompProfileTypeRuntimeDefault,
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clean-pod",
			Namespace: "restricted-ns",
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot:   &nonRoot,
				RunAsUser:      &runAsUser,
				SeccompProfile: seccomp,
			},
			Containers: []corev1.Container{
				{
					Name: "app",
					SecurityContext: &corev1.SecurityContext{
						RunAsNonRoot:             &nonRoot,
						RunAsUser:                &runAsUser,
						AllowPrivilegeEscalation: &privEscFalse,
						SeccompProfile:           seccomp,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{all},
						},
					},
				},
			},
		},
	}

	violations := checkPSAViolations(pod, PSALevelRestricted)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations for clean pod, got %d: %+v", len(violations), violations)
	}
}

func TestAssessPSARisk(t *testing.T) {
	tests := []struct {
		enforce PSAEnforcementLevel
		isSys   bool
		violat  int
		want    string
	}{
		{PSALevelNone, false, 0, "critical"},
		{PSALevelPrivileged, false, 0, "high"},
		{PSALevelBaseline, false, 0, "low"},
		{PSALevelBaseline, false, 2, "medium"},
		{PSALevelBaseline, false, 5, "high"},
		{PSALevelRestricted, false, 0, "low"},
		{PSALevelRestricted, false, 1, "medium"},
		{PSALevelNone, true, 0, "low"},
		{PSALevelRestricted, true, 2, "medium"},
	}

	for _, tt := range tests {
		got := assessPSARisk(tt.enforce, tt.isSys, tt.violat)
		if got != tt.want {
			t.Errorf("assessPSARisk(%v, sys=%v, viol=%d) = %q, want %q", tt.enforce, tt.isSys, tt.violat, got, tt.want)
		}
	}
}

func TestPSARecommendations(t *testing.T) {
	t.Run("all good", func(t *testing.T) {
		r := &PSAAuditResult{
			Summary: PSAAuditSummary{
				NotEnforced:        0,
				ViolationCount:     0,
				RestrictedEnforced: 5,
				UserNamespaces:     5,
				HasAuditMode:       5,
				Enforced:           5,
			},
		}
		recs := psaRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})

	t.Run("issues found", func(t *testing.T) {
		r := &PSAAuditResult{
			Summary: PSAAuditSummary{
				NotEnforced:    3,
				ViolationCount: 5,
				HasAuditMode:   2,
				Enforced:       3,
			},
		}
		recs := psaRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
