package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func boolPtr(b bool) *bool { return &b }

// int64Ptr is defined in handlers_deploy_audit_test.go — reuse it.

func TestSecDriftScore(t *testing.T) {
	tests := []struct {
		name     string
		s        SecDriftSummary
		minScore int
		maxScore int
	}{
		{"no containers", SecDriftSummary{}, 100, 100},
		{"all compliant", SecDriftSummary{TotalContainers: 10}, 90, 100},
		{"privileged", SecDriftSummary{TotalContainers: 10, Privileged: 2}, 60, 75},
		{"all caps", SecDriftSummary{TotalContainers: 10, HasAllCaps: 1}, 85, 92},
		{"allow priv esc", SecDriftSummary{TotalContainers: 10, AllowPrivEsc: 3}, 70, 80},
		{"all issues", SecDriftSummary{TotalContainers: 20, Privileged: 3, HasAllCaps: 2, AllowPrivEsc: 5, NoNonRoot: 10, NoCapDrop: 8, NoReadOnlyFS: 15}, 0, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := secDriftScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestSecDriftRecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := secDriftRecommendations(SecDriftSummary{})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := secDriftRecommendations(SecDriftSummary{
			Privileged: 2, HasAllCaps: 1, AllowPrivEsc: 3, NoCapDrop: 5,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestSecDriftAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		// Good pod with full security context
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "good-pod", Namespace: "secure-ns",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "app"}},
			},
			Spec: corev1.PodSpec{
				SecurityContext: &corev1.PodSecurityContext{
					RunAsNonRoot: boolPtr(true),
					RunAsUser:    int64Ptr(1000),
				},
				Containers: []corev1.Container{
					{
						Name: "app",
						SecurityContext: &corev1.SecurityContext{
							ReadOnlyRootFilesystem:   boolPtr(true),
							AllowPrivilegeEscalation: boolPtr(false),
							RunAsNonRoot:             boolPtr(true),
							RunAsUser:                int64Ptr(1000),
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
						},
					},
				},
			},
		},
		// Bad pod - privileged with all caps
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bad-pod", Namespace: "insecure-ns",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "bad-app"}},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "app",
						SecurityContext: &corev1.SecurityContext{
							Privileged: boolPtr(true),
							Capabilities: &corev1.Capabilities{
								Add: []corev1.Capability{"ALL"},
							},
						},
					},
				},
			},
		},
		// Medium risk pod - no security context at all
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "bare-pod", Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "app"},
				},
			},
		},
	}

	result := secDriftAuditCore(pods)

	if result.Summary.TotalPods != 3 {
		t.Errorf("expected totalPods=3, got %d", result.Summary.TotalPods)
	}
	if result.Summary.TotalContainers != 3 {
		t.Errorf("expected totalContainers=3, got %d", result.Summary.TotalContainers)
	}
	if result.Summary.Privileged != 1 {
		t.Errorf("expected privileged=1, got %d", result.Summary.Privileged)
	}
	if result.Summary.HasAllCaps != 1 {
		t.Errorf("expected hasAllCaps=1, got %d", result.Summary.HasAllCaps)
	}
	// bad-pod and bare-pod both have no readOnlyRootFilesystem
	if result.Summary.NoReadOnlyFS < 2 {
		t.Errorf("expected noReadOnlyFS>=2, got %d", result.Summary.NoReadOnlyFS)
	}
	if result.Summary.HighRiskPods < 1 {
		t.Errorf("expected highRiskPods>=1, got %d", result.Summary.HighRiskPods)
	}
	if len(result.Violations) < 5 {
		t.Errorf("expected at least 5 violations, got %d", len(result.Violations))
	}
	if len(result.ByNamespace) < 3 {
		t.Errorf("expected at least 3 namespace stats, got %d", len(result.ByNamespace))
	}
	if len(result.Recommendations) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(result.Recommendations))
	}
	// insecure-ns should be highest risk
	if result.ByNamespace[0].Namespace != "insecure-ns" {
		t.Errorf("expected insecure-ns first, got %s", result.ByNamespace[0].Namespace)
	}
}
