package dashboard

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDORAScore(t *testing.T) {
	tests := []struct {
		name     string
		s        DORASummary
		minScore int
		maxScore int
	}{
		{"no deploys", DORASummary{}, 100, 100},
		{"all successful", DORASummary{TotalDeployments: 10, SuccessfulDeploys: 10, RollingUpdates: 10}, 95, 100},
		{"some failures", DORASummary{TotalDeployments: 10, FailedDeploys: 2, RecreateDeploys: 1, AvgReplicaLag: 1}, 65, 80},
		{"many failures", DORASummary{TotalDeployments: 10, FailedDeploys: 8, RecreateDeploys: 5, AvgReplicaLag: 3}, 0, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := doraScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestDORALevel(t *testing.T) {
	tests := []struct {
		name string
		s    DORASummary
		want string
	}{
		{"no deploys", DORASummary{}, "unknown"},
		{"elite", DORASummary{TotalDeployments: 10, SuccessfulDeploys: 9, FailedDeploys: 1, ChangeFailureRate: 0.10}, "elite"},
		{"high", DORASummary{TotalDeployments: 10, SuccessfulDeploys: 8, FailedDeploys: 2, ChangeFailureRate: 0.20}, "high"},
		{"medium", DORASummary{TotalDeployments: 10, SuccessfulDeploys: 7, FailedDeploys: 3, ChangeFailureRate: 0.30}, "medium"},
		{"low", DORASummary{TotalDeployments: 10, SuccessfulDeploys: 6, FailedDeploys: 4, ChangeFailureRate: 0.40}, "low"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := doraLevel(tt.s)
			if got != tt.want {
				t.Errorf("doraLevel() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDORARecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := doraRecommendations(DORASummary{TotalDeployments: 5, SuccessfulDeploys: 5, RollingUpdates: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := doraRecommendations(DORASummary{
			FailedDeploys:     3,
			RecreateDeploys:   2,
			ChangeFailureRate: 0.25,
			AvgReplicaLag:     2,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestFormatAge(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{2 * time.Hour, "2h"},
		{48 * time.Hour, "2d"},
	}
	for _, tt := range tests {
		got := formatAge(tt.d)
		if got != tt.want {
			t.Errorf("formatAge(%v) = %s, want %s", tt.d, got, tt.want)
		}
	}
}

func TestDORAAuditCore(t *testing.T) {
	now := time.Now()
	replicas := int32(3)

	// Create test deployments
	deployments := []appsv1.Deployment{
		// Successful rolling update
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-success", Namespace: "prod", CreationTimestamp: metav1.Time{Time: now.Add(-2 * time.Hour)}},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
					RollingUpdate: &appsv1.RollingUpdateDeployment{
						MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
						MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					},
				},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        3,
				ReadyReplicas:   3,
				UpdatedReplicas: 3,
			},
		},
		// Failed deploy
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dep-failed", Namespace: "dev", CreationTimestamp: metav1.Time{Time: now.Add(-1 * time.Hour)}},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			},
			Status: appsv1.DeploymentStatus{
				Replicas:        3,
				ReadyReplicas:   0,
				UpdatedReplicas: 0,
			},
		},
	}

	statefulSets := []appsv1.StatefulSet{
		// Successful statefulset
		{
			ObjectMeta: metav1.ObjectMeta{Name: "sts-success", Namespace: "prod", CreationTimestamp: metav1.Time{Time: now.Add(-30 * time.Minute)}},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
			},
			Status: appsv1.StatefulSetStatus{
				Replicas:        3,
				ReadyReplicas:   3,
				UpdatedReplicas: 3,
			},
		},
	}

	result := doraAuditCore(deployments, statefulSets, now)

	if result.Summary.TotalDeployments != 3 {
		t.Errorf("expected totalDeployments=3, got %d", result.Summary.TotalDeployments)
	}
	if result.Summary.SuccessfulDeploys != 2 {
		t.Errorf("expected successfulDeploys=2, got %d", result.Summary.SuccessfulDeploys)
	}
	if result.Summary.FailedDeploys != 1 {
		t.Errorf("expected failedDeploys=1, got %d", result.Summary.FailedDeploys)
	}
	if result.Summary.RecreateDeploys != 1 {
		t.Errorf("expected recreateDeploys=1, got %d", result.Summary.RecreateDeploys)
	}
	if result.Summary.RollingUpdates != 2 {
		t.Errorf("expected rollingUpdates=2, got %d", result.Summary.RollingUpdates)
	}
	if result.Summary.ChangeFailureRate < 0.30 || result.Summary.ChangeFailureRate > 0.35 {
		t.Errorf("expected changeFailureRate ~0.33, got %.2f", result.Summary.ChangeFailureRate)
	}
	if len(result.RecentDeploys) != 3 {
		t.Errorf("expected 3 recent deploys, got %d", len(result.RecentDeploys))
	}
	// Check sorted by time (most recent first)
	if result.RecentDeploys[0].Name != "sts-success" {
		t.Errorf("expected first deploy to be sts-success (most recent), got %s", result.RecentDeploys[0].Name)
	}
	if len(result.ByNamespace) < 2 {
		t.Errorf("expected at least 2 namespace stats, got %d", len(result.ByNamespace))
	}
	if len(result.Recommendations) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(result.Recommendations))
	}
	// Level should not be elite due to 33% failure rate
	if result.Level == "elite" {
		t.Error("expected level to not be elite with 33% failure rate")
	}
}

// Ensure corev1 import is used
var _ corev1.ObjectReference
