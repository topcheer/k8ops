package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestImageVulnScore(t *testing.T) {
	tests := []struct {
		name     string
		s        ImageVulnSummary
		minScore int
		maxScore int
	}{
		{"no images", ImageVulnSummary{}, 100, 100},
		{"all pinned", ImageVulnSummary{TotalImages: 10, LatestTag: 0, NoDigest: 0}, 95, 100},
		{"some latest", ImageVulnSummary{TotalImages: 10, LatestTag: 3, NoDigest: 5}, 60, 80},
		{"all latest", ImageVulnSummary{TotalImages: 10, LatestTag: 10, NoDigest: 10}, 25, 35},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := imageVulnScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestImageVulnRecommendations(t *testing.T) {
	t.Run("all pinned", func(t *testing.T) {
		recs := imageVulnRecommendations(ImageVulnSummary{TotalImages: 10, LatestTag: 0, NoDigest: 0})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := imageVulnRecommendations(ImageVulnSummary{LatestTag: 5, NoDigest: 8, UniqueImages: 3, TotalImages: 15})
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}

func TestImageVulnAuditCore(t *testing.T) {
	pods := []corev1.Pod{
		// Pinned image with digest
		{ObjectMeta: metav1.ObjectMeta{Name: "pinned-pod", Namespace: "prod"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "nginx@sha256:abc123"},
			}}},
		// Latest tag
		{ObjectMeta: metav1.ObjectMeta{Name: "latest-pod", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "redis:latest"},
			}}},
		// Specific version tag, no digest
		{ObjectMeta: metav1.ObjectMeta{Name: "versioned-pod", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{
				{Image: "postgres:15.4"},
			}}},
	}

	result := imageVulnAuditCore(pods)

	if result.Summary.TotalImages != 3 {
		t.Errorf("expected totalImages=3, got %d", result.Summary.TotalImages)
	}
	if result.Summary.LatestTag != 2 {
		t.Errorf("expected latestTag=2 (redis:latest + nginx default), got %d", result.Summary.LatestTag)
	}
	if result.Summary.NoDigest != 2 {
		t.Errorf("expected noDigest=2, got %d", result.Summary.NoDigest)
	}
	if len(result.StaleImages) < 2 {
		t.Errorf("expected at least 2 stale images, got %d", len(result.StaleImages))
	}
	if len(result.ByNamespace) < 2 {
		t.Errorf("expected at least 2 namespace stats, got %d", len(result.ByNamespace))
	}
	if len(result.Recommendations) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(result.Recommendations))
	}
}
