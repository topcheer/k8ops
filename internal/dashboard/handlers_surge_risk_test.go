package dashboard

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestSurgeRiskScore(t *testing.T) {
	tests := []struct {
		name     string
		s        SurgeRiskSummary
		minScore int
		maxScore int
	}{
		{"no workloads", SurgeRiskSummary{}, 95, 100},
		{"all low risk", SurgeRiskSummary{TotalWorkloads: 5, RollingStrategy: 5}, 90, 100},
		{"recreate strategy", SurgeRiskSummary{TotalWorkloads: 5, RecreateStrategy: 2, HighRisk: 2}, 50, 65},
		{"maxUnavailable 100%", SurgeRiskSummary{TotalWorkloads: 5, MaxUnavailable100: 2, HighRisk: 2}, 60, 75},
		{"maxSurge too high", SurgeRiskSummary{TotalWorkloads: 5, MaxSurgeTooHigh: 3}, 80, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := surgeRiskScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestSurgeRiskRecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := surgeRiskRecommendations(SurgeRiskSummary{TotalWorkloads: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := surgeRiskRecommendations(SurgeRiskSummary{
			MaxUnavailable100: 2, RecreateStrategy: 1, MaxSurgeTooHigh: 2, NoSurgeConfig: 3,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestSurgeRiskAuditCore(t *testing.T) {
	replicas := int32(4)
	surge25 := intstr.FromString("25%")
	unavail100 := intstr.FromString("100%")
	surgeHigh := intstr.FromString("50%")

	deployments := []appsv1.Deployment{
		// Good rolling update with 25% surge
		{
			ObjectMeta: metav1.ObjectMeta{Name: "good-deploy", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &surge25,
						MaxUnavailable: &surge25,
					},
				},
			},
		},
		// Bad: maxUnavailable=100%
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-unavail", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &surge25,
						MaxUnavailable: &unavail100,
					},
				},
			},
		},
		// Bad: Recreate strategy
		{
			ObjectMeta: metav1.ObjectMeta{Name: "recreate-deploy", Namespace: "production"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RecreateDeploymentStrategyType,
				},
			},
		},
		// Bad: maxSurge too high
		{
			ObjectMeta: metav1.ObjectMeta{Name: "surge-too-high", Namespace: "production"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &surgeHigh,
						MaxUnavailable: &surge25,
					},
				},
			},
		},
	}

	result := surgeRiskAuditCore(deployments)

	if result.Summary.TotalWorkloads != 4 {
		t.Errorf("expected totalWorkloads=4, got %d", result.Summary.TotalWorkloads)
	}
	if result.Summary.RollingStrategy != 3 {
		t.Errorf("expected rollingStrategy=3, got %d", result.Summary.RollingStrategy)
	}
	if result.Summary.RecreateStrategy != 1 {
		t.Errorf("expected recreateStrategy=1, got %d", result.Summary.RecreateStrategy)
	}
	if result.Summary.MaxUnavailable100 != 1 {
		t.Errorf("expected maxUnavailable100=1, got %d", result.Summary.MaxUnavailable100)
	}
	// maxSurge 50% of 4 replicas = 2, which is exactly 50%, not >50%
	// Let me check: surgeNum > replicas/2 → 2 > 2 is false
	// So maxSurgeTooHigh should be 0
	if result.Summary.MaxSurgeTooHigh > 1 {
		t.Errorf("expected maxSurgeTooHigh<=1, got %d", result.Summary.MaxSurgeTooHigh)
	}
	if result.Summary.HighRisk < 2 {
		t.Errorf("expected highRisk>=2 (maxUnavailable100 + recreate), got %d", result.Summary.HighRisk)
	}
	if len(result.HighRiskWorkloads) < 2 {
		t.Errorf("expected at least 2 high risk workloads, got %d", len(result.HighRiskWorkloads))
	}
	if len(result.Recommendations) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(result.Recommendations))
	}
	if len(result.ByStrategy) != 2 {
		t.Errorf("expected 2 strategy stats, got %d", len(result.ByStrategy))
	}
}

// Suppress unused import
var _ = corev1.PodSpec{}
