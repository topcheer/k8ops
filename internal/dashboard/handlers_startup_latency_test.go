package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestComputeStartupPercentiles(t *testing.T) {
	tests := []struct {
		name   string
		sorted []int64
		avg    int64
		p50    int64
	}{
		{"empty", []int64{}, 0, 0},
		{"single", []int64{100}, 100, 100},
		{"uniform", []int64{1000, 1000, 1000, 1000}, 1000, 1000},
		{"varied", []int64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}, 550, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			avg, p50, _, _ := computeStartupPercentiles(tt.sorted)
			if avg != tt.avg {
				t.Errorf("avg = %d, want %d", avg, tt.avg)
			}
			if p50 != tt.p50 {
				t.Errorf("p50 = %d, want %d", p50, tt.p50)
			}
		})
	}
}

func TestStartupPercentile(t *testing.T) {
	sorted := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got := startupPercentile(sorted, 50); got != 60 {
		t.Errorf("p50 = %d, want 60", got)
	}
	if got := startupPercentile(sorted, 90); got != 100 {
		t.Errorf("p90 = %d, want 100", got)
	}
	if got := startupPercentile(sorted, 0); got != 10 {
		t.Errorf("p0 = %d, want 10", got)
	}
}

func TestComputeStartupHealthScore(t *testing.T) {
	// No pods → perfect
	score := computeStartupHealthScore(StartupLatencySummary{}, 0)
	if score != 100 {
		t.Fatalf("empty score = %d, want 100", score)
	}

	// CrashLoopBackOff pods have highest impact
	score = computeStartupHealthScore(StartupLatencySummary{
		TotalPods:     10,
		CrashLoopBack: 2,
		SlowPods:      3,
		NoReadiness:   2,
	}, 5)
	if score > 50 || score < 0 {
		t.Fatalf("crash-heavy score = %d, expected 0-50", score)
	}

	// All healthy
	score = computeStartupHealthScore(StartupLatencySummary{
		TotalPods:    10,
		FastPods:     10,
		P99StartupMs: 5000,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// High p99 penalty
	score = computeStartupHealthScore(StartupLatencySummary{
		TotalPods:    10,
		P99StartupMs: 150_000,
	}, 0)
	if score > 95 {
		t.Fatalf("high-p99 score = %d, expected <= 95", score)
	}
}

func TestFormatStartupMs(t *testing.T) {
	if got := formatStartupMs(500); got != "500ms" {
		t.Errorf("formatStartupMs(500) = %s, want 500ms", got)
	}
	if got := formatStartupMs(1500); got != "1.5s" {
		t.Errorf("formatStartupMs(1500) = %s, want 1.5s", got)
	}
	if got := formatStartupMs(60000); got != "60.0s" {
		t.Errorf("formatStartupMs(60000) = %s, want 60.0s", got)
	}
}

func TestStartupWorkloadType(t *testing.T) {
	// Pod with no owner references → "Pod"
	pod := corev1.Pod{}
	if got := startupWorkloadType(pod); got != "Pod" {
		t.Errorf("no owner: got %s, want Pod", got)
	}

	// Pod with ReplicaSet owner
	pod = corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "my-rs"},
			},
		},
	}
	if got := startupWorkloadType(pod); got != "ReplicaSet" {
		t.Errorf("ReplicaSet owner: got %s, want ReplicaSet", got)
	}
}
