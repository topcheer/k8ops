package dashboard

import (
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestBuildPodUsageMap(t *testing.T) {
	pods := &corev1.PodList{
		Items: []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "p1"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	m := buildPodUsageMap(pods)

	usage, ok := m["p1"]
	if !ok {
		t.Fatal("Expected p1 in map")
	}
	if usage.cpuM != 500 {
		t.Errorf("Expected 500m CPU, got %d", usage.cpuM)
	}
	if usage.memMB != 256 {
		t.Errorf("Expected 256MB mem, got %d", usage.memMB)
	}
}

func TestBuildPodUsageMapNil(t *testing.T) {
	m := buildPodUsageMap(nil)
	if len(m) != 0 {
		t.Errorf("Expected empty map for nil, got %d", len(m))
	}
}

func TestAnalyzeHPAEfficiencyAtMax(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa1", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Name: "dep1"},
			MinReplicas:    ptr.To(int32(2)),
			MaxReplicas:    10,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 10,
		},
	}

	entry := analyzeHPAEfficiency(hpa)

	if entry.Status != "at-max" {
		t.Errorf("Expected at-max, got %s", entry.Status)
	}
	if entry.UtilizationPct != 100 {
		t.Errorf("Expected 100%% utilization, got %d", entry.UtilizationPct)
	}
}

func TestAnalyzeHPAEfficiencyAtMin(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa2", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Name: "dep2"},
			MinReplicas:    ptr.To(int32(2)),
			MaxReplicas:    10,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 2,
		},
	}

	entry := analyzeHPAEfficiency(hpa)

	if entry.Status != "at-min" {
		t.Errorf("Expected at-min, got %s", entry.Status)
	}
}

func TestAnalyzeHPAEfficiencyOptimal(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "hpa3", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Name: "dep3"},
			MinReplicas:    ptr.To(int32(1)),
			MaxReplicas:    10,
		},
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			CurrentReplicas: 5,
		},
	}

	entry := analyzeHPAEfficiency(hpa)

	if entry.Status != "optimal" {
		t.Errorf("Expected optimal, got %s", entry.Status)
	}
	if entry.UtilizationPct != 50 {
		t.Errorf("Expected 50%% utilization, got %d", entry.UtilizationPct)
	}
}

func TestCalculateAutoscaleScore(t *testing.T) {
	// Perfect
	perfect := AutoscaleSummary{
		TotalWorkloads: 10,
		WithHPA:        10,
	}
	if score := calculateAutoscaleScore(perfect); score != 100 {
		t.Errorf("Expected 100 for perfect, got %d", score)
	}

	// Low HPA coverage
	lowHPA := AutoscaleSummary{
		TotalWorkloads: 10,
		WithHPA:        1, // 10% coverage → -20
		WithoutHPA:     9, // -18 (capped at 30)
		HPAAtMax:       1, // -5
	}
	// 100 - 20 - 18 - 5 = 57
	score := calculateAutoscaleScore(lowHPA)
	if score != 57 {
		t.Errorf("Expected 57, got %d", score)
	}

	// Floor at 0
	terrible := AutoscaleSummary{
		TotalWorkloads:  10,
		WithHPA:         0,
		WithoutHPA:      10,
		HPAAtMax:        10,
		OverProvisioned: 10,
	}
	if score := calculateAutoscaleScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestAbsFloat64(t *testing.T) {
	if absFloat64(-5.5) != 5.5 {
		t.Error("Expected 5.5")
	}
	if absFloat64(3.2) != 3.2 {
		t.Error("Expected 3.2")
	}
	if absFloat64(0) != 0 {
		t.Error("Expected 0")
	}
}

func TestAnalyzeWorkloadAutoscaleOverProvisioned(t *testing.T) {
	containers := []corev1.Container{
		{
			Name: "app",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2000m"), // 2 cores — over-provisioned
					corev1.ResourceMemory: resource.MustParse("4Gi"),   // >2GB — over-provisioned
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4000m"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
	}

	replicas := int32(3)
	rec, unscaled := analyzeWorkloadAutoscale("Deployment", "app", "default",
		&replicas, containers, &metav1.LabelSelector{}, make(map[string]*autoscalingv2.HorizontalPodAutoscaler), nil)

	if rec == nil {
		t.Fatal("Expected recommendation for over-provisioned workload")
	}
	if rec.RecommendedCPUm >= rec.CurrentCPUm {
		t.Errorf("Expected recommended < current CPU, got rec=%d cur=%d", rec.RecommendedCPUm, rec.CurrentCPUm)
	}
	if rec.Confidence != "high" {
		t.Errorf("Expected high confidence for 2-core request, got %s", rec.Confidence)
	}
	if unscaled == nil {
		t.Error("Expected unscaled workload (3 replicas, no HPA)")
	}
}

func TestAnalyzeWorkloadAutoscaleNoLimits(t *testing.T) {
	containers := []corev1.Container{
		{
			Name: "app",
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				// No limits
			},
		},
	}

	replicas := int32(1)
	rec, unscaled := analyzeWorkloadAutoscale("Deployment", "app", "default",
		&replicas, containers, &metav1.LabelSelector{}, make(map[string]*autoscalingv2.HorizontalPodAutoscaler), nil)

	// With 100m and 128Mi, it's small enough that only no-limits triggers
	if rec != nil {
		foundNoLimit := false
		for _, r := range []string{rec.Reason} {
			if containsSubstr(r, "no resource limits") {
				foundNoLimit = true
			}
		}
		if !foundNoLimit {
			t.Error("Expected recommendation about missing limits")
		}
	}
	if unscaled != nil {
		t.Error("Should not be unscaled with 1 replica")
	}
}

func TestAnalyzeWorkloadAutoscaleWithHPA(t *testing.T) {
	containers := []corev1.Container{
		{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("128Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("200m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
		},
	}

	replicas := int32(5)
	hpaMap := map[string]*autoscalingv2.HorizontalPodAutoscaler{
		"default/app": {},
	}

	rec, unscaled := analyzeWorkloadAutoscale("Deployment", "app", "default",
		&replicas, containers, &metav1.LabelSelector{}, hpaMap, nil)

	// Has HPA so should not be unscaled
	if unscaled != nil {
		t.Error("Should not be unscaled when HPA exists")
	}
	// Small requests with limits, may or may not have a rec
	_ = rec
}
