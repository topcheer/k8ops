package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodStartupScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  PodStartupSummary
		minScore int
		maxScore int
	}{
		{
			name:     "healthy cluster",
			summary:  PodStartupSummary{},
			minScore: 95,
			maxScore: 100,
		},
		{
			name: "few stuck pods",
			summary: PodStartupSummary{
				StuckCount:        2,
				SlowStartupCount:  1,
				AvgStartupSeconds: 25,
			},
			minScore: 75,
			maxScore: 95,
		},
		{
			name: "many stuck pods",
			summary: PodStartupSummary{
				StuckCount:        10,
				SlowStartupCount:  8,
				AvgStartupSeconds: 90,
				FailedPods:        3,
				MaxStartupSeconds: 400,
			},
			minScore: 0,
			maxScore: 40,
		},
		{
			name: "no pods at all",
			summary: PodStartupSummary{
				TotalPods: 0,
			},
			minScore: 95,
			maxScore: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := podStartupScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestCategorizeWaitingReason(t *testing.T) {
	tests := []struct {
		reason string
		want   string
	}{
		{"ImagePullBackOff", "image_pull"},
		{"ErrImagePull", "image_pull"},
		{"ImageApplyFailed", "image_pull"},
		{"ContainerCreating", "volume"},
		{"CreateContainerConfigError", "volume"},
		{"PodInitializing", "unknown"},
		{"", "unknown"},
		{"CrashLoopBackOff", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			got := categorizeWaitingReason(tt.reason)
			if got != tt.want {
				t.Errorf("categorizeWaitingReason(%q) = %q, want %q", tt.reason, got, tt.want)
			}
		})
	}
}

func TestInferWorkloadTypeFromPod(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "deployment pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "app-xxx"},
					},
				},
			},
			want: "ReplicaSet",
		},
		{
			name: "statefulset pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "StatefulSet", Name: "app"},
					},
				},
			},
			want: "StatefulSet",
		},
		{
			name: "daemonset pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "DaemonSet", Name: "agent"},
					},
				},
			},
			want: "DaemonSet",
		},
		{
			name: "standalone pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			want: "Pod",
		},
		{
			name: "job pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "Job", Name: "batch"},
					},
				},
			},
			want: "Job",
		},
		{
			name: "static pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-apiserver-node1",
					Annotations: map[string]string{
						"kubernetes.io/config.hash": "abc123",
					},
				},
			},
			want: "StaticPod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferWorkloadTypeFromPod(tt.pod)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildStartupBottlenecks(t *testing.T) {
	bnCount := map[string]int{
		"image_pull": 3,
		"scheduling": 1,
	}
	bns := buildStartupBottlenecks(bnCount)
	if len(bns) != 2 {
		t.Fatalf("expected 2 bottlenecks, got %d", len(bns))
	}
	if bns[0].Category != "image_pull" {
		t.Errorf("expected image_pull first (highest count), got %s", bns[0].Category)
	}
	if bns[0].Count != 3 {
		t.Errorf("expected count 3, got %d", bns[0].Count)
	}
}

func TestPodStartupRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &PodStartupResult{
			Summary:     PodStartupSummary{},
			Bottlenecks: []StartupBottleneck{},
		}
		recs := podStartupRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})

	t.Run("with bottlenecks", func(t *testing.T) {
		r := &PodStartupResult{
			Summary: PodStartupSummary{
				StuckCount:        3,
				AvgStartupSeconds: 75,
			},
			Bottlenecks: []StartupBottleneck{
				{Category: "image_pull", Count: 5},
				{Category: "scheduling", Count: 2},
			},
		}
		recs := podStartupRecommendations(r)
		if len(recs) < 2 {
			t.Errorf("expected at least 2 recommendations, got %d", len(recs))
		}
	})
}

func TestAnalyzeRunningPodStartup(t *testing.T) {
	now := time.Now()
	created := now.Add(-10 * time.Minute)
	startedTime := metav1.NewTime(created.Add(30 * time.Second))
	containerStart := metav1.NewTime(created.Add(2*time.Minute + 30*time.Second))
	readyTime := metav1.NewTime(created.Add(3 * time.Minute))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "slow-app-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(created),
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "slow-app"},
			},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &startedTime,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: containerStart,
						},
					},
				},
			},
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: readyTime,
				},
			},
		},
	}

	result := &PodStartupResult{}
	wlStats := map[string]*StartupWorkloadStat{}
	bnCount := map[string]int{}

	totalSec := analyzeRunningPodStartup(pod, wlStats, result, bnCount)

	// Total = 3 minutes = 180 seconds
	if totalSec < 170 || totalSec > 190 {
		t.Errorf("total startup = %.1fs, expected ~180s", totalSec)
	}

	// Should be flagged as slow (>120s)
	if len(result.SlowPods) != 1 {
		t.Errorf("expected 1 slow pod, got %d", len(result.SlowPods))
	}

	// Should have bottlenecks
	if bnCount["image_pull"] == 0 {
		t.Error("expected image_pull bottleneck (container start > 60s after scheduling)")
	}
}

func TestAnalyzeStuckPod(t *testing.T) {
	now := time.Now()
	created := now.Add(-12 * time.Minute) // stuck for 12 minutes

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "stuck-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(created),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{
							Reason:  "ContainerCreating",
							Message: "unable to attach volume",
						},
					},
				},
			},
		},
	}

	result := &PodStartupResult{}
	bnCount := map[string]int{}

	analyzeStuckPod(pod, now, result, bnCount)

	if len(result.StuckPods) != 1 {
		t.Fatalf("expected 1 stuck pod, got %d", len(result.StuckPods))
	}

	sp := result.StuckPods[0]
	if sp.PodName != "stuck-pod" {
		t.Errorf("pod name = %s", sp.PodName)
	}
	if sp.PendingMin < 11 || sp.PendingMin > 13 {
		t.Errorf("pending min = %.1f, expected ~12", sp.PendingMin)
	}
	if sp.RiskLevel != "high" {
		t.Errorf("risk = %s, expected high (>10min)", sp.RiskLevel)
	}

	if bnCount["volume"] == 0 {
		t.Error("expected volume bottleneck for ContainerCreating")
	}
}
