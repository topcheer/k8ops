package dashboard

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDSScore(t *testing.T) {
	tests := []struct {
		name     string
		s        DSSummary
		minScore int
		maxScore int
	}{
		{"no DS", DSSummary{}, 100, 100},
		{"all healthy", DSSummary{TotalDaemonSets: 5, TotalNodes: 10, RollingUpdate: 5}, 95, 100},
		{"missing nodes", DSSummary{TotalDaemonSets: 5, MissingNodes: 3}, 80, 90},
		{"many issues", DSSummary{TotalDaemonSets: 5, MissingNodes: 5, StaleRevisions: 3, OnDeleteStrategy: 2, NoTolerations: 1}, 50, 70},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := dsScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestDSRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		recs := dsRecommendations(DSSummary{TotalDaemonSets: 5, TotalNodes: 10, MissingNodes: 0, StaleRevisions: 0, OnDeleteStrategy: 0})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := dsRecommendations(DSSummary{
			MissingNodes:     3,
			OnDeleteStrategy: 2,
			StaleRevisions:   5,
			NoTolerations:    1,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestIsNodeSchedulable(t *testing.T) {
	if !isNodeSchedulable(&corev1.Node{}) {
		t.Error("default node should be schedulable")
	}
	if isNodeSchedulable(&corev1.Node{Spec: corev1.NodeSpec{Unschedulable: true}}) {
		t.Error("unschedulable node should not be schedulable")
	}
	if isNodeSchedulable(&corev1.Node{Spec: corev1.NodeSpec{Taints: []corev1.Taint{{Key: "node.kubernetes.io/unschedulable"}}}}) {
		t.Error("node with unschedulable taint should not be schedulable")
	}
}

func TestDSAuditCore(t *testing.T) {
	daemonSets := []appsv1.DaemonSet{
		// Healthy DaemonSet with RollingUpdate
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ds-healthy", Namespace: "kube-system"},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					Type: appsv1.RollingUpdateDaemonSetStrategyType,
				},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers:  []corev1.Container{{Image: "nginx:1.25"}},
						Tolerations: []corev1.Toleration{{Key: "node-role.kubernetes.io/control-plane"}},
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				CurrentNumberScheduled: 3,
				UpdatedNumberScheduled: 3,
				NumberReady:            3,
				NumberAvailable:        3,
			},
		},
		// Degraded DaemonSet with missing nodes
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ds-degraded", Namespace: "monitoring"},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
					Type: appsv1.OnDeleteDaemonSetStrategyType,
				},
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Image: "prom/node-exporter:latest"}},
					},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 3,
				CurrentNumberScheduled: 2,
				UpdatedNumberScheduled: 1,
				NumberReady:            2,
			},
		},
	}

	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-3"}},
	}

	// Create pods for ds-healthy on all 3 nodes
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "ds-healthy-pod-1", Namespace: "kube-system",
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds-healthy"}}},
			Spec: corev1.PodSpec{NodeName: "node-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ds-healthy-pod-2", Namespace: "kube-system",
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds-healthy"}}},
			Spec: corev1.PodSpec{NodeName: "node-2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ds-healthy-pod-3", Namespace: "kube-system",
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds-healthy"}}},
			Spec: corev1.PodSpec{NodeName: "node-3"}},
		// ds-degraded only has 2 pods
		{ObjectMeta: metav1.ObjectMeta{Name: "ds-degraded-pod-1", Namespace: "monitoring",
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds-degraded"}}},
			Spec: corev1.PodSpec{NodeName: "node-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ds-degraded-pod-2", Namespace: "monitoring",
			OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds-degraded"}}},
			Spec: corev1.PodSpec{NodeName: "node-2"}},
	}

	result := dsAuditCore(daemonSets, nodes, pods)

	if result.Summary.TotalDaemonSets != 2 {
		t.Errorf("expected totalDaemonSets=2, got %d", result.Summary.TotalDaemonSets)
	}
	if result.Summary.TotalNodes != 3 {
		t.Errorf("expected totalNodes=3, got %d", result.Summary.TotalNodes)
	}
	if result.Summary.OnDeleteStrategy != 1 {
		t.Errorf("expected onDeleteStrategy=1, got %d", result.Summary.OnDeleteStrategy)
	}
	if result.Summary.RollingUpdate != 1 {
		t.Errorf("expected rollingUpdate=1, got %d", result.Summary.RollingUpdate)
	}
	if result.Summary.NoTolerations != 1 {
		t.Errorf("expected noTolerations=1, got %d", result.Summary.NoTolerations)
	}
	if result.Summary.MissingNodes != 1 {
		t.Errorf("expected missingNodes=1, got %d", result.Summary.MissingNodes)
	}
	if result.Summary.StaleRevisions != 1 {
		t.Errorf("expected staleRevisions=1, got %d", result.Summary.StaleRevisions)
	}
	// Should have node gaps for ds-degraded on node-3
	if len(result.NodeGaps) < 1 {
		t.Errorf("expected at least 1 node gap, got %d", len(result.NodeGaps))
	}
	// ds-healthy should be healthy, ds-degraded should be degraded
	statusMap := make(map[string]string)
	for _, entry := range result.ByDaemonSet {
		statusMap[entry.Name] = entry.Status
	}
	if statusMap["ds-healthy"] != "healthy" {
		t.Errorf("expected ds-healthy status=healthy, got %s", statusMap["ds-healthy"])
	}
	if statusMap["ds-degraded"] != "degraded" {
		t.Errorf("expected ds-degraded status=degraded, got %s", statusMap["ds-degraded"])
	}
	if len(result.Recommendations) < 4 {
		t.Errorf("expected at least 4 recommendations, got %d", len(result.Recommendations))
	}
}
