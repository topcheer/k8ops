package dashboard

import (
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHPAGapScore(t *testing.T) {
	tests := []struct {
		name     string
		s        HPAGapSummary
		minScore int
		maxScore int
	}{
		{"no HPAs", HPAGapSummary{}, 100, 100},
		{"all healthy", HPAGapSummary{TotalHPAs: 5, HPAsWithMetrics: 5}, 90, 100},
		{"no metrics", HPAGapSummary{TotalHPAs: 5, HPAsNoMetrics: 3}, 75, 85},
		{"min=max", HPAGapSummary{TotalHPAs: 5, MinEqualsMax: 2}, 85, 95},
		{"high gap", HPAGapSummary{TotalHPAs: 5, HighGapHPAs: 2}, 75, 85},
		{"all issues", HPAGapSummary{TotalHPAs: 10, NoScaleBehavior: 5, MinEqualsMax: 3, TargetTooHigh: 2, HighGapHPAs: 4, HPAsNoMetrics: 3}, 0, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := hpaGapScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestHPAGapRecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := hpaGapRecommendations(HPAGapSummary{TotalHPAs: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := hpaGapRecommendations(HPAGapSummary{
			HPAsNoMetrics: 2, TargetTooHigh: 1, MinEqualsMax: 1, NoScaleBehavior: 3,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestHPAGapAuditCore(t *testing.T) {
	target95 := int32(95)
	target50 := int32(50)
	target20 := int32(20)
	current60 := int32(60)
	current90 := int32(90)
	min1 := int32(1)

	hpas := []autoscalingv2.HorizontalPodAutoscaler{
		// Good HPA with proper config
		{
			ObjectMeta: metav1.ObjectMeta{Name: "good-hpa", Namespace: "default"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &min1,
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								AverageUtilization: &target50,
							},
						},
					},
				},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &autoscalingv2.HPAScalingRules{
						StabilizationWindowSeconds: intPtrDefault(300),
					},
				},
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 3,
				DesiredReplicas: 3,
				CurrentMetrics: []autoscalingv2.MetricStatus{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricStatus{
							Name: corev1.ResourceCPU,
							Current: autoscalingv2.MetricValueStatus{
								AverageUtilization: &current60,
							},
						},
					},
				},
			},
		},
		// Bad HPA: target too high, no behavior, min=max
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bad-hpa", Namespace: "production"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &min1,
				MaxReplicas: 1,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								AverageUtilization: &target95,
							},
						},
					},
				},
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 1,
				DesiredReplicas: 1,
			},
		},
		// No metrics HPA
		{
			ObjectMeta: metav1.ObjectMeta{Name: "no-metrics-hpa", Namespace: "default"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &min1,
				MaxReplicas: 5,
			},
		},
		// High gap HPA
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gap-hpa", Namespace: "production"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				MinReplicas: &min1,
				MaxReplicas: 10,
				Metrics: []autoscalingv2.MetricSpec{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								AverageUtilization: &target20,
							},
						},
					},
				},
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 2,
				DesiredReplicas: 2,
				CurrentMetrics: []autoscalingv2.MetricStatus{
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricStatus{
							Name: corev1.ResourceCPU,
							Current: autoscalingv2.MetricValueStatus{
								AverageUtilization: &current90,
							},
						},
					},
				},
			},
		},
	}

	result := hpaGapAuditCore(hpas)

	if result.Summary.TotalHPAs != 4 {
		t.Errorf("expected totalHPAs=4, got %d", result.Summary.TotalHPAs)
	}
	if result.Summary.HPAsWithMetrics != 3 {
		t.Errorf("expected hpasWithMetrics=3, got %d", result.Summary.HPAsWithMetrics)
	}
	if result.Summary.HPAsNoMetrics != 1 {
		t.Errorf("expected hpasNoMetrics=1, got %d", result.Summary.HPAsNoMetrics)
	}
	if result.Summary.TargetTooHigh != 1 {
		t.Errorf("expected targetTooHigh=1, got %d", result.Summary.TargetTooHigh)
	}
	if result.Summary.TargetTooLow != 1 {
		t.Errorf("expected targetTooLow=1, got %d", result.Summary.TargetTooLow)
	}
	if result.Summary.MinEqualsMax != 1 {
		t.Errorf("expected minEqualsMax=1, got %d", result.Summary.MinEqualsMax)
	}
	if result.Summary.NoScaleBehavior < 3 {
		t.Errorf("expected noScaleBehavior>=3, got %d", result.Summary.NoScaleBehavior)
	}
	if len(result.Issues) < 5 {
		t.Errorf("expected at least 5 issues, got %d", len(result.Issues))
	}
	if len(result.Recommendations) < 4 {
		t.Errorf("expected at least 4 recommendations, got %d", len(result.Recommendations))
	}
}

// intPtrDefault returns a pointer to the given int32.
func intPtrDefault(i int32) *int32 { return &i }
