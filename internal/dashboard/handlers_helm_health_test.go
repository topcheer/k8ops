package dashboard

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestHelmHealthScore(t *testing.T) {
	tests := []struct {
		name     string
		s        HelmHealthSummary
		minScore int
		maxScore int
	}{
		{"no helm", HelmHealthSummary{}, 100, 100},
		{"all healthy", HelmHealthSummary{HasHelm: true, TotalReleases: 5, HealthyReleases: 5}, 95, 100},
		{"some failed", HelmHealthSummary{HasHelm: true, TotalReleases: 5, FailedReleases: 2, HealthyReleases: 3}, 60, 75},
		{"pending", HelmHealthSummary{HasHelm: true, TotalReleases: 5, PendingReleases: 1, HealthyReleases: 4}, 85, 95},
		{"all failed", HelmHealthSummary{HasHelm: true, TotalReleases: 3, FailedReleases: 3}, 40, 60},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := helmHealthScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestHelmHealthRecommendations(t *testing.T) {
	t.Run("no helm", func(t *testing.T) {
		recs := helmHealthRecommendations(HelmHealthSummary{})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := helmHealthRecommendations(HelmHealthSummary{
			HasHelm: true, FailedReleases: 2, PendingReleases: 1, StaleReleases: 3,
		})
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
	t.Run("all healthy", func(t *testing.T) {
		recs := helmHealthRecommendations(HelmHealthSummary{HasHelm: true, TotalReleases: 5, HealthyReleases: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
}

func TestHelmHealthAuditCore(t *testing.T) {
	now := time.Now()

	secrets := []corev1.Secret{
		// Healthy deployed release
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.myapp.v1",
				Labels: map[string]string{
					"owner":   "helm",
					"name":    "myapp",
					"status":  "deployed",
					"version": "1",
					"chart":   "myapp-0.1.0",
				},
				CreationTimestamp: metav1.Time{Time: now.Add(-24 * time.Hour)},
			},
		},
		// Failed release
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.broken.v2",
				Labels: map[string]string{
					"owner":   "helm",
					"name":    "broken",
					"status":  "failed",
					"version": "2",
					"chart":   "broken-1.0.0",
				},
				CreationTimestamp: metav1.Time{Time: now.Add(-2 * time.Hour)},
			},
		},
		// Pending release
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.stuck.v1",
				Labels: map[string]string{
					"owner":   "helm",
					"name":    "stuck",
					"status":  "pending-upgrade",
					"version": "1",
					"chart":   "stuck-0.5.0",
				},
				CreationTimestamp: metav1.Time{Time: now.Add(-30 * time.Minute)},
			},
		},
		// Newer version of myapp (should override v1)
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "sh.helm.release.v1.myapp.v3",
				Labels: map[string]string{
					"owner":   "helm",
					"name":    "myapp",
					"status":  "deployed",
					"version": "3",
					"chart":   "myapp-0.3.0",
				},
				CreationTimestamp: metav1.Time{Time: now.Add(-1 * time.Hour)},
			},
		},
		// Non-helm secret (should be ignored)
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default-token-abc",
				Labels: map[string]string{
					"owner": "kubernetes",
				},
			},
		},
	}

	result := helmHealthAuditCore(secrets, now)

	if !result.Summary.HasHelm {
		t.Error("expected hasHelm=true")
	}
	// myapp (deployed), broken (failed), stuck (pending) = 3 unique releases
	if result.Summary.TotalReleases != 3 {
		t.Errorf("expected totalReleases=3, got %d", result.Summary.TotalReleases)
	}
	if result.Summary.HealthyReleases != 1 {
		t.Errorf("expected healthyReleases=1, got %d", result.Summary.HealthyReleases)
	}
	if result.Summary.FailedReleases != 1 {
		t.Errorf("expected failedReleases=1, got %d", result.Summary.FailedReleases)
	}
	if result.Summary.PendingReleases != 1 {
		t.Errorf("expected pendingReleases=1, got %d", result.Summary.PendingReleases)
	}
	if len(result.DriftedReleases) != 2 {
		t.Errorf("expected 2 drifted/issue releases, got %d", len(result.DriftedReleases))
	}
	if len(result.Recommendations) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(result.Recommendations))
	}

	// Verify myapp shows version 3 (latest)
	for _, rel := range result.Releases {
		if rel.Name == "myapp" && rel.Version != "3" {
			t.Errorf("expected myapp version=3, got %s", rel.Version)
		}
	}
}
