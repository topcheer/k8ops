package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makePodWithAnnotations(annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Annotations: annotations,
		},
	}
}

func makePodWithOwnerRefs(owners []string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}
	for _, owner := range owners {
		parts := splitOwnerRef(owner)
		if parts == nil {
			continue
		}
		pod.OwnerReferences = append(pod.OwnerReferences, metav1.OwnerReference{
			Kind: parts[0],
			Name: parts[1],
		})
	}
	return pod
}

func splitOwnerRef(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return nil
}

func TestCfgSyncScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  CfgSyncSummary
		minScore int
		maxScore int
	}{
		{
			name: "no config refs",
			summary: CfgSyncSummary{
				PodsWithConfigRef: 0,
			},
			minScore: 95,
			maxScore: 100,
		},
		{
			name: "all volume mounts, no staleness",
			summary: CfgSyncSummary{
				PodsWithConfigRef: 10,
				EnvVarRefs:        0,
				VolumeRefs:        10,
				StalePodCount:     0,
				ImmutableCMs:      5,
			},
			minScore: 80,
			maxScore: 100,
		},
		{
			name: "all env vars, many stale",
			summary: CfgSyncSummary{
				PodsWithConfigRef: 10,
				EnvVarRefs:        15,
				VolumeRefs:        0,
				StalePodCount:     8,
				SubPathRefs:       3,
				NeedsReloader:     4,
			},
			minScore: 0,
			maxScore: 30,
		},
		{
			name: "mixed, some stale",
			summary: CfgSyncSummary{
				PodsWithConfigRef: 20,
				EnvVarRefs:        8,
				VolumeRefs:        12,
				StalePodCount:     5,
				NeedsReloader:     2,
			},
			minScore: 30,
			maxScore: 70,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := cfgSyncScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestHasReloaderAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{"stakater reloader", map[string]string{"configmap.reloader.stakater.com/reload": "true"}, true},
		{"secret reloader", map[string]string{"secret.reloader.stakater.com/reload": "true"}, true},
		{"no annotations", nil, false},
		{"unrelated annotations", map[string]string{"kubectl.kubernetes.io/last-applied": "{}"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := makePodWithAnnotations(tt.annotations)
			got := hasReloaderAnnotation(pod)
			if got != tt.want {
				t.Errorf("hasReloaderAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetWorkloadName(t *testing.T) {
	tests := []struct {
		name    string
		podName string
		owners  []string // Kind:Name pairs
		want    string
	}{
		{"deployment", "app-xxx-yyy", []string{"ReplicaSet:app-xxx"}, "app"},
		{"statefulset", "web-0", []string{"StatefulSet:web"}, "web"},
		{"daemonset", "fluentd-node1", []string{"DaemonSet:fluentd"}, "fluentd"},
		{"standalone", "test-pod", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := makePodWithOwnerRefs(tt.owners)
			got := getWorkloadName(pod)
			if got != tt.want {
				t.Errorf("getWorkloadName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCfgSyncRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &CfgSyncResult{
			Summary: CfgSyncSummary{
				PodsWithConfigRef: 10,
				EnvVarRefs:        0,
				VolumeRefs:        10,
				StalePodCount:     0,
				ImmutableCMs:      5,
				ImmutableSecrets:  3,
			},
		}
		recs := cfgSyncRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})

	t.Run("with issues", func(t *testing.T) {
		r := &CfgSyncResult{
			Summary: CfgSyncSummary{
				PodsWithConfigRef: 10,
				EnvVarRefs:        8,
				VolumeRefs:        2,
				StalePodCount:     5,
				SubPathRefs:       3,
				NeedsReloader:     4,
				ImmutableCMs:      0,
			},
		}
		recs := cfgSyncRecommendations(r)
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}
