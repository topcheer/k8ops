package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClassifyNodeCapacityType(t *testing.T) {
	tests := []struct {
		name string
		node corev1.Node
		want string
	}{
		{
			"karpenter spot",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"karpenter.sh/capacity-type": "spot"}}},
			"spot",
		},
		{
			"karpenter on-demand",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"karpenter.sh/capacity-type": "on-demand"}}},
			"on-demand",
		},
		{
			"gce preemptible",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"cloud.google.com/gke-preemptible": "true"}}},
			"spot",
		},
		{
			"azure spot",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"kubernetes.azure.com/scalesetpriority": "spot"}}},
			"spot",
		},
		{
			"generic spot label",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"spot-instance": "true"}}},
			"spot",
		},
		{
			"plain node",
			corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{corev1.LabelInstanceType: "m5.large"}}},
			"on-demand",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyNodeCapacityType(&tt.node)
			if got != tt.want {
				t.Errorf("classifyNodeCapacityType() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestSpotReadinessScore(t *testing.T) {
	tests := []struct {
		name     string
		s        SpotReadinessSummary
		minScore int
		maxScore int
	}{
		{"no nodes", SpotReadinessSummary{}, 100, 100},
		{"spot with protection", SpotReadinessSummary{TotalNodes: 10, SpotNodes: 5, SpotPercentage: 50, SpotInterruption: true}, 95, 100},
		{"spot without handler", SpotReadinessSummary{TotalNodes: 10, SpotNodes: 5, SpotPercentage: 50, SpotInterruption: false}, 80, 90},
		{"spot with critical", SpotReadinessSummary{TotalNodes: 10, SpotNodes: 5, CriticalOnSpot: 3, SpotInterruption: true}, 60, 75},
		{"no spot", SpotReadinessSummary{TotalNodes: 10, SpotNodes: 0}, 90, 95},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := spotReadinessScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestSpotReadinessRecommendations(t *testing.T) {
	t.Run("no spot", func(t *testing.T) {
		recs := spotReadinessRecommendations(SpotReadinessSummary{TotalNodes: 10, SpotNodes: 0})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("healthy spot", func(t *testing.T) {
		recs := spotReadinessRecommendations(SpotReadinessSummary{
			TotalNodes: 10, SpotNodes: 3, SpotPercentage: 30,
			SpotInterruption: true, CriticalOnSpot: 0, WorkloadsOnSpot: 5,
		})
		if len(recs) < 2 {
			t.Errorf("expected at least 2 recommendations, got %d", len(recs))
		}
	})
	t.Run("at risk", func(t *testing.T) {
		recs := spotReadinessRecommendations(SpotReadinessSummary{
			TotalNodes: 10, SpotNodes: 3, SpotPercentage: 30,
			SpotInterruption: false, CriticalOnSpot: 5, WorkloadsOnSpot: 8,
			HasSpotToleration: 0, HasSpotAntiAffin: 0,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestSpotReadinessAuditCore(t *testing.T) {
	now := time.Now()

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "spot-node-1",
				Labels: map[string]string{
					"karpenter.sh/capacity-type": "spot",
					corev1.LabelInstanceType:     "m5.large",
					corev1.LabelTopologyZone:     "us-east-1a",
				},
				CreationTimestamp: metav1.Time{Time: now.Add(-48 * time.Hour)},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "on-demand-node-1",
				Labels: map[string]string{
					"karpenter.sh/capacity-type": "on-demand",
					corev1.LabelInstanceType:     "m5.xlarge",
					corev1.LabelTopologyZone:     "us-east-1b",
				},
				CreationTimestamp: metav1.Time{Time: now.Add(-72 * time.Hour)},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("16Gi"),
				},
			},
		},
	}

	pods := []corev1.Pod{
		// Pod on spot without toleration — at risk
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-unprotected", Namespace: "prod"},
			Spec:       corev1.PodSpec{NodeName: "spot-node-1"},
		},
		// Pod on spot with toleration — protected
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-protected", Namespace: "prod"},
			Spec: corev1.PodSpec{
				NodeName: "spot-node-1",
				Tolerations: []corev1.Toleration{
					{Key: "karpenter.sh/capacity-type", Value: "spot"},
				},
			},
		},
		// Pod on on-demand — not affected
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-ondemand", Namespace: "prod"},
			Spec:       corev1.PodSpec{NodeName: "on-demand-node-1"},
		},
		// Karpenter pod — interruption handler
		{
			ObjectMeta: metav1.ObjectMeta{Name: "karpenter-controller-abc", Namespace: "karpenter"},
			Spec:       corev1.PodSpec{NodeName: "on-demand-node-1"},
		},
	}

	result := spotReadinessAuditCore(nodes, pods, now)

	if result.Summary.TotalNodes != 2 {
		t.Errorf("expected totalNodes=2, got %d", result.Summary.TotalNodes)
	}
	if result.Summary.SpotNodes != 1 {
		t.Errorf("expected spotNodes=1, got %d", result.Summary.SpotNodes)
	}
	if result.Summary.OnDemandNodes != 1 {
		t.Errorf("expected onDemandNodes=1, got %d", result.Summary.OnDemandNodes)
	}
	if result.Summary.SpotPercentage < 49 || result.Summary.SpotPercentage > 51 {
		t.Errorf("expected spotPercentage ~50, got %.1f", result.Summary.SpotPercentage)
	}
	if !result.Summary.SpotInterruption {
		t.Error("expected spotInterruption=true (karpenter detected)")
	}
	if result.Summary.WorkloadsOnSpot != 2 {
		t.Errorf("expected workloadsOnSpot=2, got %d", result.Summary.WorkloadsOnSpot)
	}
	if result.Summary.CriticalOnSpot != 1 {
		t.Errorf("expected criticalOnSpot=1, got %d", result.Summary.CriticalOnSpot)
	}
	if result.Summary.HasSpotToleration != 1 {
		t.Errorf("expected hasSpotToleration=1, got %d", result.Summary.HasSpotToleration)
	}
	if result.Summary.SpotCapacity != 2 {
		t.Errorf("expected spotCapacity=2, got %d", result.Summary.SpotCapacity)
	}
	if len(result.AtRiskWorkloads) < 1 {
		t.Errorf("expected at least 1 at-risk workload, got %d", len(result.AtRiskWorkloads))
	}
	if len(result.SpotNodes) != 1 {
		t.Errorf("expected 1 spot node entry, got %d", len(result.SpotNodes))
	}
	if len(result.Recommendations) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(result.Recommendations))
	}
	if result.HealthScore > 100 {
		t.Errorf("health score should not exceed 100, got %d", result.HealthScore)
	}
}
