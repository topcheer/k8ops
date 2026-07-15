package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func priPolicyPtr(p corev1.PreemptionPolicy) *corev1.PreemptionPolicy { return &p }

func TestAnalyzePriorityPreemption_Basic(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec: corev1.PodSpec{
				PriorityClassName: "normal",
				Priority:          int32Ptr(1000),
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default"},
			Spec: corev1.PodSpec{
				PriorityClassName: "low",
				Priority:          int32Ptr(-10),
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	pcs := []schedulingv1.PriorityClass{
		{
			ObjectMeta:       metav1.ObjectMeta{Name: "normal"},
			Value:            1000,
			GlobalDefault:    true,
			Description:      "Normal priority",
			PreemptionPolicy: priPolicyPtr(corev1.PreemptLowerPriority),
		},
		{
			ObjectMeta:       metav1.ObjectMeta{Name: "low"},
			Value:            -10,
			GlobalDefault:    false,
			Description:      "Low priority",
			PreemptionPolicy: priPolicyPtr(corev1.PreemptNever),
		},
	}

	result := analyzePriorityPreemption(pods, pcs)

	if result.Summary.TotalPriorityClasses != 2 {
		t.Errorf("expected 2 priority classes, got %d", result.Summary.TotalPriorityClasses)
	}
	if result.Summary.PodsWithPriority != 2 {
		t.Errorf("expected 2 pods with priority, got %d", result.Summary.PodsWithPriority)
	}
	if result.Summary.PodsWithoutPriority != 0 {
		t.Errorf("expected 0 pods without priority, got %d", result.Summary.PodsWithoutPriority)
	}
	if result.Summary.HighPriorityPods != 0 {
		t.Errorf("expected 0 high priority pods, got %d", result.Summary.HighPriorityPods)
	}
	if result.Summary.LowPriorityPods != 1 {
		t.Errorf("expected 1 low priority pod, got %d", result.Summary.LowPriorityPods)
	}
	if result.Summary.PreemptionRiskCount != 1 {
		t.Errorf("expected 1 preemption risk, got %d", result.Summary.PreemptionRiskCount)
	}
	if result.Score < 95 {
		t.Errorf("expected score >= 95, got %d", result.Score)
	}
}

func TestAnalyzePriorityPreemption_PendingAndStarvation(t *testing.T) {
	oldTime := metav1.NewTime(metav1.Now().Add(-15 * time.Minute))

	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pending1", Namespace: "app"},
			Spec: corev1.PodSpec{
				PriorityClassName: "low",
				Priority:          int32Ptr(100),
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:               corev1.PodScheduled,
						Status:             corev1.ConditionFalse,
						Reason:             "Unschedulable",
						Message:            "0/3 nodes are available",
						LastTransitionTime: oldTime,
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "running1", Namespace: "app"},
			Spec: corev1.PodSpec{
				PriorityClassName: "high",
				Priority:          int32Ptr(1000000),
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	pcs := []schedulingv1.PriorityClass{
		{ObjectMeta: metav1.ObjectMeta{Name: "high"}, Value: 1000000,
			PreemptionPolicy: priPolicyPtr(corev1.PreemptLowerPriority)},
		{ObjectMeta: metav1.ObjectMeta{Name: "low"}, Value: 100,
			PreemptionPolicy: priPolicyPtr(corev1.PreemptLowerPriority)},
	}

	result := analyzePriorityPreemption(pods, pcs)

	if result.Summary.PendingPods != 1 {
		t.Errorf("expected 1 pending pod, got %d", result.Summary.PendingPods)
	}
	if result.Summary.StarvationRiskCount != 1 {
		t.Errorf("expected 1 starvation risk, got %d", result.Summary.StarvationRiskCount)
	}
	if len(result.PendingPods) != 1 {
		t.Errorf("expected 1 pending pod entry, got %d", len(result.PendingPods))
	}
	// 1 starvation risk => score = 100 - 5 = 95
	if result.Score != 95 {
		t.Errorf("expected score 95 due to 1 starvation risk, got %d", result.Score)
	}
}

func TestAnalyzePriorityPreemption_Heatmap(t *testing.T) {
	pods := []corev1.Pod{
		{Spec: corev1.PodSpec{Priority: int32Ptr(-1)}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{Spec: corev1.PodSpec{Priority: int32Ptr(500)}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{Spec: corev1.PodSpec{Priority: int32Ptr(5000)}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
		{Spec: corev1.PodSpec{Priority: int32Ptr(2000000)}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
	}

	result := analyzePriorityPreemption(pods, nil)

	if len(result.PriorityHeatmap) != 5 {
		t.Fatalf("expected 5 heatmap buckets, got %d", len(result.PriorityHeatmap))
	}

	found := map[string]int{}
	for _, e := range result.PriorityHeatmap {
		found[e.Range] = e.PodCount
	}
	if found["<0 (Preemptible)"] != 1 {
		t.Errorf("expected 1 preemptible pod, got %d", found["<0 (Preemptible)"])
	}
	if found["0-999 (Low)"] != 1 {
		t.Errorf("expected 1 low priority pod, got %d", found["0-999 (Low)"])
	}
	if found["1K-99K (Normal)"] != 1 {
		t.Errorf("expected 1 normal priority pod, got %d", found["1K-99K (Normal)"])
	}
	if found[">1M (System)"] != 1 {
		t.Errorf("expected 1 system priority pod, got %d", found[">1M (System)"])
	}
}

func TestAnalyzePriorityPreemption_NoPriorityClass(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "nopri", Namespace: "default"},
			Spec:       corev1.PodSpec{},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	}

	result := analyzePriorityPreemption(pods, nil)

	if result.Summary.PodsWithoutPriority != 1 {
		t.Errorf("expected 1 pod without priority, got %d", result.Summary.PodsWithoutPriority)
	}
	if result.Score < 90 {
		t.Errorf("expected score >= 90 for single pod, got %d", result.Score)
	}
	foundRec := false
	for _, r := range result.Recommendations {
		if r != "" {
			foundRec = true
			break
		}
	}
	if !foundRec {
		t.Error("expected at least one recommendation")
	}
}
