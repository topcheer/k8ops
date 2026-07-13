package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodePoolScore(t *testing.T) {
	tests := []struct {
		name     string
		s        NodePoolSummary
		minScore int
		maxScore int
	}{
		{"no nodes", NodePoolSummary{}, 100, 100},
		{"all healthy", NodePoolSummary{TotalNodes: 5, ReadyNodes: 5, HasAutoscaler: true}, 90, 100},
		{"not ready", NodePoolSummary{TotalNodes: 5, NotReadyNodes: 2, HasAutoscaler: true}, 60, 75},
		{"stale", NodePoolSummary{TotalNodes: 5, StaleNodes: 1}, 85, 95},
		{"cordoned", NodePoolSummary{TotalNodes: 5, CordonedNodes: 2, HasAutoscaler: true}, 90, 100},
		{"no autoscaler", NodePoolSummary{TotalNodes: 5, ReadyNodes: 5}, 85, 100},
		{"all bad", NodePoolSummary{TotalNodes: 10, NotReadyNodes: 3, StaleNodes: 2, CordonedNodes: 2, UnbalancedPools: 1}, 0, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := nodePoolScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestNodePoolRecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := nodePoolRecommendations(NodePoolSummary{TotalNodes: 5, ReadyNodes: 5, HasAutoscaler: true})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := nodePoolRecommendations(NodePoolSummary{
			NotReadyNodes: 2, StaleNodes: 1, CordonedNodes: 1, UnbalancedPools: 1,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
	t.Run("no autoscaler", func(t *testing.T) {
		recs := nodePoolRecommendations(NodePoolSummary{TotalNodes: 5, ReadyNodes: 5})
		found := false
		for _, r := range recs {
			if r != "" && r != "all nodes healthy — no node pool issues detected" {
				found = true
			}
		}
		if !found {
			t.Error("expected autoscaler recommendation when HasAutoscaler=false")
		}
	})
}

func TestNodePoolAuditCore(t *testing.T) {
	now := time.Now()

	nodes := []corev1.Node{
		// Ready worker node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-1",
				Labels: map[string]string{
					"node-role.kubernetes.io/worker": "",
					"topology.kubernetes.io/zone":    "us-east-1a",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Time{Time: now}},
				},
			},
		},
		// NotReady worker node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-2",
				Labels: map[string]string{
					"node-role.kubernetes.io/worker": "",
					"topology.kubernetes.io/zone":    "us-east-1b",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionFalse, LastHeartbeatTime: metav1.Time{Time: now}},
				},
			},
		},
		// Cordoned worker node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-3",
				Labels: map[string]string{
					"node-role.kubernetes.io/worker": "",
					"topology.kubernetes.io/zone":    "us-east-1a",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: true},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Time{Time: now}},
				},
			},
		},
		// Ready master node
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "master-1",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "",
					"topology.kubernetes.io/zone":           "us-east-1a",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Time{Time: now}},
				},
			},
		},
		// Stale node (heartbeat > 5min ago)
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-4",
				Labels: map[string]string{
					"node-role.kubernetes.io/worker": "",
					"topology.kubernetes.io/zone":    "us-east-1c",
				},
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue, LastHeartbeatTime: metav1.Time{Time: now.Add(-10 * time.Minute)}},
				},
			},
		},
	}

	autoscalerPods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster-autoscaler-abc", Namespace: "kube-system"}},
	}

	result := nodePoolAuditCore(nodes, autoscalerPods)

	if result.Summary.TotalNodes != 5 {
		t.Errorf("expected totalNodes=5, got %d", result.Summary.TotalNodes)
	}
	if result.Summary.ReadyNodes != 3 {
		t.Errorf("expected readyNodes=3 (worker-1, master-1, worker-4), got %d", result.Summary.ReadyNodes)
	}
	if result.Summary.NotReadyNodes != 1 {
		t.Errorf("expected notReadyNodes=1, got %d", result.Summary.NotReadyNodes)
	}
	if result.Summary.CordonedNodes != 1 {
		t.Errorf("expected cordonedNodes=1, got %d", result.Summary.CordonedNodes)
	}
	if result.Summary.StaleNodes != 1 {
		t.Errorf("expected staleNodes=1, got %d", result.Summary.StaleNodes)
	}
	if !result.Summary.HasAutoscaler {
		t.Error("expected hasAutoscaler=true")
	}
	if len(result.UnhealthyNodes) < 2 {
		t.Errorf("expected at least 2 unhealthy nodes, got %d", len(result.UnhealthyNodes))
	}
	if len(result.StaleNodes) < 1 {
		t.Errorf("expected at least 1 stale node, got %d", len(result.StaleNodes))
	}
	if len(result.ByZone) < 3 {
		t.Errorf("expected at least 3 zones, got %d", len(result.ByZone))
	}
	if len(result.Recommendations) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(result.Recommendations))
	}
}
