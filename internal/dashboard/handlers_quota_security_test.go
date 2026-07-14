package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestQuotaSecScore(t *testing.T) {
	tests := []struct {
		name     string
		s        QuotaSecSummary
		minScore int
		maxScore int
	}{
		{"no namespaces", QuotaSecSummary{}, 100, 100},
		{"all protected", QuotaSecSummary{TotalNamespaces: 10, WithResourceQuota: 10, WithLimitRange: 10}, 95, 100},
		{"some unprotected", QuotaSecSummary{TotalNamespaces: 10, UnprotectedNamespaces: 3, NoLimitRange: 5}, 50, 70},
		{"all unprotected", QuotaSecSummary{TotalNamespaces: 10, UnprotectedNamespaces: 10, NoLimitRange: 10}, 0, 10},
		{"with pressure", QuotaSecSummary{TotalNamespaces: 10, WithResourceQuota: 10, WithLimitRange: 10, CPUPressure: 3, MemoryPressure: 2}, 70, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := quotaSecScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestQuotaSecRecommendations(t *testing.T) {
	t.Run("all protected", func(t *testing.T) {
		recs := quotaSecRecommendations(QuotaSecSummary{TotalNamespaces: 5, WithResourceQuota: 5, WithLimitRange: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := quotaSecRecommendations(QuotaSecSummary{
			UnprotectedNamespaces: 3,
			NoLimitRange:          5,
			CPUPressure:           2,
			MemoryPressure:        1,
			PodPressure:           1,
		})
		if len(recs) < 4 {
			t.Errorf("expected at least 4 recommendations, got %d", len(recs))
		}
	})
}

func TestCalcPercent(t *testing.T) {
	hard := resource.MustParse("10")
	used := resource.MustParse("8")
	pct := calcPercent(&used, &hard)
	if pct != 80 {
		t.Errorf("expected 80%%, got %d", pct)
	}

	// Zero hard limit
	zero := resource.MustParse("0")
	pct = calcPercent(&used, &zero)
	if pct != 0 {
		t.Errorf("expected 0%% for zero hard limit, got %d", pct)
	}

	// Nil used
	pct = calcPercent(nil, &hard)
	if pct != 0 {
		t.Errorf("expected 0%% for nil used, got %d", pct)
	}
}

func TestQuotaSecurityAuditCore(t *testing.T) {
	// Create test namespaces
	namespaces := []corev1.Namespace{
		{ObjectMeta: metav1.ObjectMeta{Name: "prod"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		{ObjectMeta: metav1.ObjectMeta{Name: "dev"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		{ObjectMeta: metav1.ObjectMeta{Name: "staging"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}},
		{ObjectMeta: metav1.ObjectMeta{Name: "terminating"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating}},
	}

	// Quotas: prod has quota, dev does not
	quotas := []corev1.ResourceQuota{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "quota-prod", Namespace: "prod"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
					corev1.ResourcePods:   resource.MustParse("50"),
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("9"),    // 90% — pressure
					corev1.ResourceMemory: resource.MustParse("10Gi"), // 50%
					corev1.ResourcePods:   resource.MustParse("45"),   // 90% — pressure
				},
			},
		},
	}

	// LimitRanges: prod and staging have them, dev does not
	limitRanges := []corev1.LimitRange{
		{ObjectMeta: metav1.ObjectMeta{Name: "lr-prod", Namespace: "prod"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "lr-staging", Namespace: "staging"}},
	}

	// Pods
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "prod"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "prod"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "dev"}},
	}

	result := quotaSecurityAuditCore(namespaces, quotas, limitRanges, pods)

	// Should skip terminating namespace
	if result.Summary.TotalNamespaces != 3 {
		t.Errorf("expected totalNamespaces=3 (skipped terminating), got %d", result.Summary.TotalNamespaces)
	}

	// prod has quota, staging and dev don't
	if result.Summary.WithResourceQuota != 1 {
		t.Errorf("expected withResourceQuota=1, got %d", result.Summary.WithResourceQuota)
	}

	// prod and staging have limit range, dev doesn't
	if result.Summary.WithLimitRange != 2 {
		t.Errorf("expected withLimitRange=2, got %d", result.Summary.WithLimitRange)
	}

	// dev and staging are unprotected (no quota)
	if result.Summary.UnprotectedNamespaces != 2 {
		t.Errorf("expected unprotectedNamespaces=2, got %d", result.Summary.UnprotectedNamespaces)
	}

	// dev has no limit range
	if result.Summary.NoLimitRange != 1 {
		t.Errorf("expected noLimitRange=1, got %d", result.Summary.NoLimitRange)
	}

	// prod has CPU pressure (90%)
	if result.Summary.CPUPressure != 1 {
		t.Errorf("expected cpuPressure=1, got %d", result.Summary.CPUPressure)
	}

	// prod has pod pressure (90%)
	if result.Summary.PodPressure != 1 {
		t.Errorf("expected podPressure=1, got %d", result.Summary.PodPressure)
	}

	// Should have unprotected namespaces listed
	if len(result.UnprotectedNS) < 2 {
		t.Errorf("expected at least 2 unprotected namespaces, got %d", len(result.UnprotectedNS))
	}

	// Should have limit range gaps listed
	if len(result.LimitRangeGaps) < 1 {
		t.Errorf("expected at least 1 limit range gap, got %d", len(result.LimitRangeGaps))
	}

	// Should have quota pressure entries
	if len(result.QuotaPressure) < 2 {
		t.Errorf("expected at least 2 quota pressure entries, got %d", len(result.QuotaPressure))
	}

	// Should have recommendations
	if len(result.Recommendations) < 3 {
		t.Errorf("expected at least 3 recommendations, got %d", len(result.Recommendations))
	}

	// Health score should be degraded
	if result.HealthScore > 80 {
		t.Errorf("expected health score <= 80 due to unprotected namespaces, got %d", result.HealthScore)
	}
}
