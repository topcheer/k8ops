package dashboard

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestRHClassify(t *testing.T) {
	// Healthy
	dep := appsv1.Deployment{
		Spec:   appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 3, UpdatedReplicas: 3},
	}
	entry := RHEntry{Replicas: 3, ReadyReplicas: 3, UpdatedReplicas: 3}
	status, risk := rhClassify(dep, entry)
	if status != "healthy" || risk != "low" {
		t.Errorf("Expected healthy/low, got %s/%s", status, risk)
	}

	// Stuck: Progressing=False
	dep = appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 2,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded"},
			},
		},
	}
	entry = RHEntry{Replicas: 3, ReadyReplicas: 2}
	status, risk = rhClassify(dep, entry)
	if status != "stuck" || risk != "critical" {
		t.Errorf("Expected stuck/critical, got %s/%s", status, risk)
	}

	// Stuck: ReplicaFailure
	dep = appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue},
			},
		},
	}
	entry = RHEntry{Replicas: 3, ReadyReplicas: 0}
	status, _ = rhClassify(dep, entry)
	if status != "stuck" {
		t.Errorf("Expected stuck for ReplicaFailure, got %s", status)
	}

	// Paused
	dep = appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(3), Paused: true},
	}
	entry = RHEntry{Replicas: 3, ReadyReplicas: 3}
	status, risk = rhClassify(dep, entry)
	if status != "paused" || risk != "medium" {
		t.Errorf("Expected paused/medium, got %s/%s", status, risk)
	}

	// Progressing
	dep = appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "ReplicaSetUpdated"},
			},
		},
	}
	entry = RHEntry{Replicas: 3, ReadyReplicas: 1}
	status, _ = rhClassify(dep, entry)
	if status != "progressing" {
		t.Errorf("Expected progressing, got %s", status)
	}
}

func TestRHAssessRollback(t *testing.T) {
	if !rhAssessRollback(appsv1.Deployment{}) {
		t.Error("Expected rollback ready with default")
	}

	zero := int32(0)
	dep := appsv1.Deployment{Spec: appsv1.DeploymentSpec{RevisionHistoryLimit: &zero}}
	if rhAssessRollback(dep) {
		t.Error("Expected NOT ready with revisionHistoryLimit=0")
	}

	five := int32(5)
	dep = appsv1.Deployment{Spec: appsv1.DeploymentSpec{RevisionHistoryLimit: &five}}
	if !rhAssessRollback(dep) {
		t.Error("Expected ready with revisionHistoryLimit=5")
	}
}

func TestRHScore(t *testing.T) {
	if score := rhScore(RHSummary{}); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s := RHSummary{TotalDeployments: 10, Healthy: 10}
	if score := rhScore(s); score != 100 {
		t.Errorf("Expected 100, got %d", score)
	}

	s = RHSummary{
		TotalDeployments:  10,
		Stuck:             2,
		Paused:            1,
		NoRevisionHistory: 1,
		RecreateStrategy:  1,
	}
	if score := rhScore(s); score != 55 {
		t.Errorf("Expected 55, got %d", score)
	}
}

func TestRHRecs(t *testing.T) {
	s := RHSummary{
		Stuck:               1,
		Paused:              1,
		NoRevisionHistory:   2,
		RecreateStrategy:    1,
		LowProgressDeadline: 1,
		NoMinReadySeconds:   5,
		HealthScore:         40,
	}
	stuck := []RHEntry{
		{Namespace: "default", Name: "app1", Replicas: 3, ReadyReplicas: 1},
	}
	poor := []RHEntry{
		{Namespace: "default", Name: "app2"},
	}

	recs := rhRecs(s, stuck, poor)
	if len(recs) < 6 {
		t.Errorf("Expected at least 6 recommendations, got %d", len(recs))
	}
}

func TestRHRecsClean(t *testing.T) {
	s := RHSummary{TotalDeployments: 10, Healthy: 10}
	recs := rhRecs(s, nil, nil)
	if len(recs) == 0 {
		t.Error("Expected positive recommendation")
	}
}

func TestRHRiskRank(t *testing.T) {
	if rhRiskRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if rhRiskRank("high") != 1 {
		t.Error("Expected 1")
	}
}

func TestRHIssueRank(t *testing.T) {
	if rhIssueRank("critical") != 0 {
		t.Error("Expected 0")
	}
	if rhIssueRank("warning") != 1 {
		t.Error("Expected 1")
	}
}
