package dashboard

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// int32Ptr is defined in handlers_deploy_audit_test.go — reuse it.

func TestReplicaAvailScore(t *testing.T) {
	tests := []struct {
		name     string
		s        ReplicaAvailSummary
		minScore int
		maxScore int
	}{
		{"no workloads", ReplicaAvailSummary{}, 100, 100},
		{"all healthy", ReplicaAvailSummary{TotalWorkloads: 5, TotalDesired: 10, TotalReady: 10, HealthyWorkloads: 5}, 95, 100},
		{"partial gap", ReplicaAvailSummary{TotalWorkloads: 5, TotalDesired: 10, TotalReady: 8, GapWorkloads: 2}, 70, 85},
		{"zero ready", ReplicaAvailSummary{TotalWorkloads: 3, TotalDesired: 10, TotalReady: 5, ZeroReady: 2, GapWorkloads: 2}, 15, 55},
		{"all down", ReplicaAvailSummary{TotalWorkloads: 3, TotalDesired: 10, TotalReady: 0, ZeroReady: 3, GapWorkloads: 3}, 0, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := replicaAvailScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestAssessReplicaRisk(t *testing.T) {
	tests := []struct {
		name      string
		desired   int32
		ready     int32
		updated   int32
		wantRisk  string
		wantIssue bool
	}{
		{"scaled to 0", 0, 0, 0, "none", false},
		{"all ready", 5, 5, 5, "none", false},
		{"partial ready", 5, 3, 3, "high", true},
		{"zero ready", 5, 0, 0, "critical", true},
		{"rollout in progress", 5, 5, 3, "medium", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			risk, issue := assessReplicaRisk(tt.desired, tt.ready, tt.updated)
			if risk != tt.wantRisk {
				t.Errorf("risk = %q, want %q", risk, tt.wantRisk)
			}
			if (issue != "") != tt.wantIssue {
				t.Errorf("issue = %q, wantIssue = %v", issue, tt.wantIssue)
			}
		})
	}
}

func TestReplicaAvailRecommendations(t *testing.T) {
	t.Run("zero ready", func(t *testing.T) {
		recs := replicaAvailRecommendations(ReplicaAvailSummary{ZeroReady: 2, GapWorkloads: 3, TotalDesired: 10, TotalReady: 5})
		if len(recs) < 2 {
			t.Errorf("expected at least 2 recommendations, got %d", len(recs))
		}
	})
	t.Run("all healthy", func(t *testing.T) {
		recs := replicaAvailRecommendations(ReplicaAvailSummary{HealthyWorkloads: 5, TotalWorkloads: 5, TotalDesired: 10, TotalReady: 10})
		if len(recs) == 0 {
			t.Error("expected recommendations for healthy state")
		}
	})
}

func TestReplicaAvailAuditCore(t *testing.T) {
	deployments := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "healthy-deploy", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 3, AvailableReplicas: 3, UpdatedReplicas: 3, Replicas: 3},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "partial-deploy", Namespace: "default"},
			Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(5)},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 3, AvailableReplicas: 3, UpdatedReplicas: 3, Replicas: 5},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "down-deploy", Namespace: "production"},
			Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(2)},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 0, AvailableReplicas: 0, UpdatedReplicas: 0, Replicas: 0},
		},
	}

	statefulSets := []appsv1.StatefulSet{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "good-sts", Namespace: "default"},
			Spec:       appsv1.StatefulSetSpec{Replicas: int32Ptr(3)},
			Status:     appsv1.StatefulSetStatus{ReadyReplicas: 3, UpdatedReplicas: 3, Replicas: 3},
		},
	}

	daemonSets := []appsv1.DaemonSet{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ds-rolling", Namespace: "kube-system"},
			Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 4, NumberReady: 4, UpdatedNumberScheduled: 2},
		},
	}

	result := replicaAvailAuditCore(deployments, statefulSets, daemonSets)

	if result.Summary.TotalWorkloads != 5 {
		t.Errorf("expected totalWorkloads=5, got %d", result.Summary.TotalWorkloads)
	}
	if result.Summary.HealthyWorkloads != 2 {
		t.Errorf("expected healthyWorkloads=2, got %d", result.Summary.HealthyWorkloads)
	}
	if result.Summary.GapWorkloads != 3 {
		t.Errorf("expected gapWorkloads=3, got %d", result.Summary.GapWorkloads)
	}
	if result.Summary.ZeroReady != 1 {
		t.Errorf("expected zeroReady=1, got %d", result.Summary.ZeroReady)
	}
	if len(result.CriticalGaps) != 1 {
		t.Errorf("expected 1 critical gap, got %d", len(result.CriticalGaps))
	}
	if len(result.ByNamespace) < 3 {
		t.Errorf("expected at least 3 namespace stats, got %d", len(result.ByNamespace))
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected recommendations")
	}
	// ds-rolling has all ready but only 2/4 updated -> medium risk
	foundRolling := false
	for _, e := range result.ByWorkload {
		if e.Name == "ds-rolling" && e.RiskLevel == "medium" {
			foundRolling = true
		}
	}
	if !foundRolling {
		t.Error("expected ds-rolling to have medium risk (rollout in progress)")
	}
}

func TestFormatDurationAge(t *testing.T) {
	// Test with zero time
	if age := formatDurationAge(time.Time{}); age != "unknown" {
		t.Errorf("expected 'unknown' for zero time, got %s", age)
	}
	// Test with recent time (should be in minutes or hours)
	age := formatDurationAge(time.Now().Add(-30 * time.Minute))
	if age == "unknown" {
		t.Error("expected non-unknown age for 30min ago")
	}
}

// Suppress unused import warnings
var _ = corev1.PodSpec{}
var _ = types.UID("")
