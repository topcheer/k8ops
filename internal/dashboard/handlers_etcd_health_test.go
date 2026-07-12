package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestEtcdPressureScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  EtcdHealthSummary
		minScore int
		maxScore int
	}{
		{"no large objects", EtcdHealthSummary{}, 95, 100},
		{"some large objects", EtcdHealthSummary{LargeObjects: 5}, 70, 80},
		{"many large objects", EtcdHealthSummary{LargeObjects: 20}, 65, 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := etcdPressureScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestEtcdHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  EtcdHealthSummary
		minScore int
		maxScore int
	}{
		{"healthy 3 nodes", EtcdHealthSummary{EtcdFound: 3, EtcdReady: 3}, 90, 100},
		{"one not ready", EtcdHealthSummary{EtcdFound: 3, EtcdReady: 2, EtcdNotReady: 1}, 65, 85},
		{"single etcd", EtcdHealthSummary{EtcdFound: 1, EtcdReady: 1}, 75, 85},
		{"no etcd", EtcdHealthSummary{EtcdFound: 0}, 78, 82},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := etcdHealthScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestConfigMapSizeKB(t *testing.T) {
	cm := &corev1.ConfigMap{
		Data: map[string]string{
			"key1": string(make([]byte, 1024)),
			"key2": string(make([]byte, 2048)),
		},
	}
	size := configMapSizeKB(cm)
	if size < 2.5 || size > 3.5 {
		t.Errorf("configMapSizeKB = %.2f, want ~3.0", size)
	}
}

func TestSecretSizeKB(t *testing.T) {
	sec := &corev1.Secret{
		Data: map[string][]byte{
			"key1": make([]byte, 512),
			"key2": make([]byte, 512),
		},
	}
	size := secretSizeKB(sec)
	if size < 0.5 || size > 1.5 {
		t.Errorf("secretSizeKB = %.2f, want ~1.0", size)
	}
}

func TestIsEtcdPod(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			"etcd pod",
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "etcd-master"},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Image: "registry.k8s.io/etcd:3.5.10"},
				}},
			},
			true,
		},
		{
			"normal pod",
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "nginx-pod"},
				Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Image: "nginx:latest"},
				}},
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEtcdPod(tt.pod); got != tt.want {
				t.Errorf("isEtcdPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEtcdHealthRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &EtcdHealthResult{Summary: EtcdHealthSummary{EtcdFound: 3, EtcdReady: 3}}
		recs := etcdHealthRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &EtcdHealthResult{
			Summary:    EtcdHealthSummary{EtcdFound: 1, EtcdNotReady: 1},
			ConfigMaps: []LargeObjectEntry{{Name: "big-cm", SizeKB: 500}},
		}
		recs := etcdHealthRecommendations(r)
		if len(recs) < 2 {
			t.Errorf("expected at least 2 recommendations, got %d", len(recs))
		}
	})
}
