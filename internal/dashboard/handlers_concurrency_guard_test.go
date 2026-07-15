package dashboard

import (
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestAnalyzeDeploymentConcurrency_NoActiveRollouts(t *testing.T) {
	deps := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:          3,
				UpdatedReplicas:   3,
				ReadyReplicas:     3,
				AvailableReplicas: 3,
			},
		},
	}

	result := analyzeDeploymentConcurrency(deps, nil, nil)

	if result.Summary.ActiveRollouts != 0 {
		t.Errorf("expected 0 active rollouts, got %d", result.Summary.ActiveRollouts)
	}
	if !result.SafeToDeploy {
		t.Error("expected safe to deploy")
	}
	if result.Score < 95 {
		t.Errorf("expected score >= 95, got %d", result.Score)
	}
}

func TestAnalyzeDeploymentConcurrency_ActiveRollout(t *testing.T) {
	surge := intstr.FromInt(2)
	unavail := intstr.FromInt(0)
	deps := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "rolling-dep", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(5),
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &surge,
						MaxUnavailable: &unavail,
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:          6,
				UpdatedReplicas:   2,
				ReadyReplicas:     5,
				AvailableReplicas: 5,
			},
		},
	}

	result := analyzeDeploymentConcurrency(deps, nil, nil)

	if result.Summary.ActiveRollouts != 1 {
		t.Errorf("expected 1 active rollout, got %d", result.Summary.ActiveRollouts)
	}
	if result.SafeToDeploy {
		t.Error("expected NOT safe to deploy")
	}
	if len(result.Blockers) == 0 {
		t.Error("expected at least 1 blocker")
	}
	if result.Summary.TotalSurgePods != 2 {
		t.Errorf("expected 2 surge pods, got %d", result.Summary.TotalSurgePods)
	}
}

func TestAnalyzeDeploymentConcurrency_NamespaceCollision(t *testing.T) {
	deps := []appsv1.Deployment{}
	for i := 0; i < 4; i++ {
		deps = append(deps, appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("dep%d", i), Namespace: "shared"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(3),
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        4,
				UpdatedReplicas: 1, // actively rolling
			},
		})
	}

	result := analyzeDeploymentConcurrency(deps, nil, nil)

	// 4 rollouts in same namespace => namespace collision
	foundCollision := false
	for _, c := range result.CollisionRisks {
		if c.Type == "NamespaceConcurrency" {
			foundCollision = true
		}
	}
	if !foundCollision {
		t.Error("expected namespace concurrency collision risk")
	}
	// 1 namespace collision => -10 => score=90
	if result.Score > 90 {
		t.Errorf("expected score <= 90 due to collisions, got %d", result.Score)
	}
}

func TestAnalyzeDeploymentConcurrency_SurgeBudgetHigh(t *testing.T) {
	surge := intstr.FromInt(10)
	deps := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "big-dep", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Replicas: int32Ptr(5),
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge: &surge,
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        5,
				UpdatedReplicas: 5,
			},
		},
	}

	result := analyzeDeploymentConcurrency(deps, nil, nil)

	// Surge = 10, desired = 5, maxPossible = 15, ratio = 3.0 => high
	if result.SurgeBudget.RiskLevel != "high" {
		t.Errorf("expected high surge risk, got %s", result.SurgeBudget.RiskLevel)
	}
	if result.SurgeBudget.SurgeRatio < 2.0 {
		t.Errorf("expected surge ratio >= 2.0, got %.1f", result.SurgeBudget.SurgeRatio)
	}
}
