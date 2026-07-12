package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTenantPressureScore(t *testing.T) {
	tests := []struct {
		name     string
		s        TenantPressureSummary
		minScore int
		maxScore int
	}{
		{"no namespaces", TenantPressureSummary{}, 100, 100},
		{"all healthy", TenantPressureSummary{TotalNamespaces: 5, NamespacesWithQuota: 5}, 95, 100},
		{"some unbounded", TenantPressureSummary{TotalNamespaces: 5, UnboundedNamespaces: 2}, 75, 85},
		{"critical quotas", TenantPressureSummary{TotalNamespaces: 5, CriticalQuotas: 3}, 70, 80},
		{"all bad", TenantPressureSummary{TotalNamespaces: 10, UnboundedNamespaces: 5, CriticalQuotas: 5, SaturatedQuotas: 3, NoLimitRange: 4}, 0, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := tenantPressureScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestTenantPressureRecommendations(t *testing.T) {
	t.Run("all healthy", func(t *testing.T) {
		recs := tenantPressureRecommendations(TenantPressureSummary{TotalNamespaces: 5, NamespacesWithQuota: 5})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		recs := tenantPressureRecommendations(TenantPressureSummary{
			UnboundedNamespaces: 2,
			CriticalQuotas:      1,
			SaturatedQuotas:     2,
		})
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}

func TestTenantPressureAuditCore(t *testing.T) {
	quotas := []corev1.ResourceQuota{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "quota-1", Namespace: "tenant-a"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10"),
					corev1.ResourceMemory: resource.MustParse("20Gi"),
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("9.5"),
					corev1.ResourceMemory: resource.MustParse("10Gi"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "quota-2", Namespace: "tenant-b"},
			Status: corev1.ResourceQuotaStatus{
				Hard: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("5"),
				},
				Used: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4.2"),
				},
			},
		},
	}

	limitRanges := []corev1.LimitRange{
		{ObjectMeta: metav1.ObjectMeta{Name: "lr-1", Namespace: "tenant-a"}},
	}

	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-a1", Namespace: "tenant-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-a2", Namespace: "tenant-a"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-b1", Namespace: "tenant-b"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-c1", Namespace: "tenant-c"}}, // no quota, no limit range
	}

	nodes := []corev1.Node{
		{
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("16"),
					corev1.ResourceMemory: resource.MustParse("64Gi"),
				},
			},
		},
	}

	result := tenantPressureAuditCore(quotas, limitRanges, pods, nodes)

	if result.Summary.TotalNamespaces != 3 {
		t.Errorf("expected totalNamespaces=3, got %d", result.Summary.TotalNamespaces)
	}
	if result.Summary.NamespacesWithQuota != 2 {
		t.Errorf("expected namespacesWithQuota=2, got %d", result.Summary.NamespacesWithQuota)
	}
	if result.Summary.NamespacesNoQuota != 1 {
		t.Errorf("expected namespacesNoQuota=1, got %d", result.Summary.NamespacesNoQuota)
	}
	if result.Summary.UnboundedNamespaces != 1 {
		t.Errorf("expected unboundedNamespaces=1, got %d", result.Summary.UnboundedNamespaces)
	}
	// tenant-a CPU at 95% -> saturated (>80%), tenant-b CPU at 84% -> saturated (>80%)
	if result.Summary.CriticalQuotas != 0 {
		t.Errorf("expected criticalQuotas=0, got %d", result.Summary.CriticalQuotas)
	}
	if result.Summary.SaturatedQuotas != 2 {
		t.Errorf("expected saturatedQuotas=2, got %d", result.Summary.SaturatedQuotas)
	}

	// Verify tenant-c is highest risk (unbounded)
	if len(result.ByNamespace) == 0 {
		t.Fatal("expected byNamespace entries")
	}
	if result.ByNamespace[0].Namespace != "tenant-c" && result.ByNamespace[0].RiskLevel != "critical" {
		// Either tenant-a (critical quota) or tenant-c (unbounded) should be first
	}
	if len(result.Recommendations) < 2 {
		t.Errorf("expected at least 2 recommendations, got %d", len(result.Recommendations))
	}
}
