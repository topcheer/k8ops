package dashboard

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeReplicaDistribution_AllOnOneNode(t *testing.T) {
	replicas := int32(3)
	deploys := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "app", UID: "uid1"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	}
	// All 3 pods on same node, owned by dep1
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1", Kind: "ReplicaSet"}}}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1", Kind: "ReplicaSet"}}}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1", Kind: "ReplicaSet"}}}, Spec: corev1.PodSpec{NodeName: "node-a"}},
	}

	result := analyzeReplicaDistribution(deploys, nil, pods)

	if result.Summary.AtRiskCount != 1 {
		t.Errorf("expected 1 at-risk workload, got %d", result.Summary.AtRiskCount)
	}
	if len(result.AtRiskWorkloads) != 1 {
		t.Fatalf("expected 1 at-risk entry, got %d", len(result.AtRiskWorkloads))
	}
	if result.AtRiskWorkloads[0].RiskType != "AllPodsOnSingleNode" {
		t.Errorf("expected AllPodsOnSingleNode, got %s", result.AtRiskWorkloads[0].RiskType)
	}
}

func TestAnalyzeReplicaDistribution_GoodSpread(t *testing.T) {
	replicas := int32(3)
	deploys := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "app", UID: "uid1"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1"}}}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1"}}}, Spec: corev1.PodSpec{NodeName: "node-b"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1"}}}, Spec: corev1.PodSpec{NodeName: "node-c"}},
	}

	result := analyzeReplicaDistribution(deploys, nil, pods)

	if result.Summary.GoodSpread != 1 {
		t.Errorf("expected 1 good spread, got %d", result.Summary.GoodSpread)
	}
	if result.Summary.AtRiskCount != 0 {
		t.Errorf("expected 0 at-risk, got %d", result.Summary.AtRiskCount)
	}
}

func TestAnalyzeReplicaDistribution_MissingAntiAffinity(t *testing.T) {
	replicas := int32(3)
	deploys := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "dep1", Namespace: "app"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	}
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1"}}}, Spec: corev1.PodSpec{NodeName: "node-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1"}}}, Spec: corev1.PodSpec{NodeName: "node-b"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "app", OwnerReferences: []metav1.OwnerReference{{Name: "dep1"}}}, Spec: corev1.PodSpec{NodeName: "node-c"}},
	}

	result := analyzeReplicaDistribution(deploys, nil, pods)

	if result.Summary.NoAntiAffinity != 1 {
		t.Errorf("expected 1 no-anti-affinity, got %d", result.Summary.NoAntiAffinity)
	}
}

func TestAnalyzeReplicaDistribution_SingleReplica(t *testing.T) {
	replicas := int32(1)
	deploys := []appsv1.Deployment{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "single", Namespace: "app"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	}

	result := analyzeReplicaDistribution(deploys, nil, nil)

	if result.Summary.SingleReplica != 1 {
		t.Errorf("expected 1 single-replica workload, got %d", result.Summary.SingleReplica)
	}
}

func TestHasPodAntiAffinity(t *testing.T) {
	if hasPodAntiAffinity(nil) {
		t.Error("expected false for nil affinity")
	}
	aff := &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{}},
		},
	}
	if !hasPodAntiAffinity(aff) {
		t.Error("expected true for anti-affinity with required terms")
	}
}
