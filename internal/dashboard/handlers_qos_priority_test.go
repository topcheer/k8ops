package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestQoSScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  QoSSummary
		minScore int
		maxScore int
	}{
		{
			name: "all guaranteed with priority",
			summary: QoSSummary{
				TotalPods:         10,
				GuaranteedPods:    10,
				WithPriorityClass: 10,
			},
			minScore: 90,
			maxScore: 100,
		},
		{
			name: "all besteffort no priority",
			summary: QoSSummary{
				TotalPods:       10,
				BestEffortPods:  10,
				NoPriorityClass: 10,
				NoRequestsPods:  10,
				NoLimitsPods:    10,
			},
			minScore: 0,
			maxScore: 25,
		},
		{
			name: "mixed cluster",
			summary: QoSSummary{
				TotalPods:         20,
				GuaranteedPods:    5,
				BurstablePods:     10,
				BestEffortPods:    5,
				WithPriorityClass: 15,
				NoRequestsPods:    3,
				NoLimitsPods:      5,
				MisconfigCount:    2,
			},
			minScore: 80,
			maxScore: 90,
		},
		{
			name:     "empty cluster",
			summary:  QoSSummary{},
			minScore: 95,
			maxScore: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := qosScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestComputeQoS(t *testing.T) {
	cpu := resource.MustParse("100m")
	mem := resource.MustParse("128Mi")

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "guaranteed - requests equal limits",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{"cpu": cpu, "memory": mem},
								Limits:   corev1.ResourceList{"cpu": cpu, "memory": mem},
							},
						},
					},
				},
			},
			want: string(corev1.PodQOSGuaranteed),
		},
		{
			name: "burstable - has requests but limits differ",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{"cpu": cpu, "memory": mem},
								Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m"), "memory": resource.MustParse("256Mi")},
							},
						},
					},
				},
			},
			want: string(corev1.PodQOSBurstable),
		},
		{
			name: "besteffort - no requests no limits",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Resources: corev1.ResourceRequirements{}},
					},
				},
			},
			want: string(corev1.PodQOSBestEffort),
		},
		{
			name: "burstable - has cpu request no limit",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{"cpu": cpu},
							},
						},
					},
				},
			},
			want: string(corev1.PodQOSBurstable),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeQoS(tt.pod)
			if got != tt.want {
				t.Errorf("computeQoS() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckPodResources(t *testing.T) {
	cpu := resource.MustParse("100m")
	mem := resource.MustParse("128Mi")

	tests := []struct {
		name    string
		pod     *corev1.Pod
		wantReq bool
		wantLim bool
	}{
		{
			name: "has both requests and limits",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{"cpu": cpu, "memory": mem},
							Limits:   corev1.ResourceList{"cpu": cpu, "memory": mem},
						}},
					},
				},
			},
			wantReq: true,
			wantLim: true,
		},
		{
			name: "no resources at all",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Resources: corev1.ResourceRequirements{}},
					},
				},
			},
			wantReq: false,
			wantLim: false,
		},
		{
			name: "requests only",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{"cpu": cpu},
						}},
					},
				},
			},
			wantReq: true,
			wantLim: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReq, gotLim := checkPodResources(tt.pod)
			if gotReq != tt.wantReq {
				t.Errorf("hasRequests = %v, want %v", gotReq, tt.wantReq)
			}
			if gotLim != tt.wantLim {
				t.Errorf("hasLimits = %v, want %v", gotLim, tt.wantLim)
			}
		})
	}
}

func TestCheckQoSMisconfigs(t *testing.T) {
	t.Run("besteffort in user namespace", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "app-pod", Namespace: "production"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{}}}},
		}
		issues := checkQoSMisconfigs(pod, "BestEffort", "", 0, false, "Deployment")
		if len(issues) == 0 {
			t.Error("expected at least one misconfig for BestEffort in user namespace")
		}
		foundBestEffort := false
		for _, m := range issues {
			if m.Severity == "high" {
				foundBestEffort = true
			}
		}
		if !foundBestEffort {
			t.Error("expected high severity misconfig")
		}
	})

	t.Run("guaranteed in system namespace - clean", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"},
		}
		issues := checkQoSMisconfigs(pod, "Guaranteed", "system-cluster-critical", 2000000000, true, "Deployment")
		if len(issues) != 0 {
			t.Errorf("expected 0 misconfigs for system Guaranteed pod, got %d", len(issues))
		}
	})
}

func TestQoSRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &QoSResult{
			Summary: QoSSummary{
				TotalPods:      10,
				GuaranteedPods: 10,
			},
			PriorityClasses: []PriorityClassInfo{
				{Name: "default", IsGlobalDefault: true},
			},
		}
		recs := qosRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})

	t.Run("with issues", func(t *testing.T) {
		r := &QoSResult{
			Summary: QoSSummary{
				BestEffortPods:  5,
				NoPriorityClass: 3,
				NoRequestsPods:  2,
			},
		}
		recs := qosRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
