package dashboard

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestOPAComplianceScore(t *testing.T) {
	tests := []struct {
		name     string
		s        OPAComplianceSummary
		minScore int
		maxScore int
	}{
		{"no policy engine", OPAComplianceSummary{}, 48, 52},
		{"gatekeeper no violations", OPAComplianceSummary{HasGatekeeper: true, EnforcedCount: 5}, 95, 100},
		{"gatekeeper with violations", OPAComplianceSummary{HasGatekeeper: true, ViolationCount: 5, NamespacesWithViolations: 3}, 60, 80},
		{"kyverno enforced", OPAComplianceSummary{HasKyverno: true, EnforcedCount: 3, ViolationCount: 0}, 95, 100},
		{"all audit mode", OPAComplianceSummary{HasGatekeeper: true, AuditMode: 5, EnforcedCount: 0}, 95, 100},
		{"many violations", OPAComplianceSummary{HasGatekeeper: true, ViolationCount: 20, NamespacesWithViolations: 10}, 0, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := opaComplianceScore(tt.s)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestOPAComplianceRecommendations(t *testing.T) {
	t.Run("no engine", func(t *testing.T) {
		recs := opaComplianceRecommendations(OPAComplianceSummary{})
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with violations", func(t *testing.T) {
		recs := opaComplianceRecommendations(OPAComplianceSummary{
			HasGatekeeper: true, ViolationCount: 5, NamespacesWithViolations: 3,
		})
		if len(recs) == 0 {
			t.Error("expected recommendations for violations")
		}
	})
	t.Run("all audit", func(t *testing.T) {
		recs := opaComplianceRecommendations(OPAComplianceSummary{
			HasGatekeeper: true, AuditMode: 3, EnforcedCount: 0,
		})
		found := false
		for _, r := range recs {
			if r != "" {
				found = true
			}
		}
		if !found {
			t.Error("expected audit mode recommendation")
		}
	})
}

func TestOPAComplianceAuditCore(t *testing.T) {
	gatekeeperPods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "gatekeeper-audit-0", Namespace: "gatekeeper-system"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Image: "openpolicyagent/gatekeeper:v3.17"}}},
		},
	}

	// Create constraint unstructured objects
	constraint1 := &unstructured.Unstructured{}
	constraint1.SetKind("K8sRequiredLabels")
	constraint1.SetName("require-labels")
	constraint1.SetNamespace("")
	unstructured.SetNestedField(constraint1.Object, "enforce", "spec", "enforcementAction")
	unstructured.SetNestedSlice(constraint1.Object, []interface{}{
		map[string]interface{}{"name": "app-pod", "namespace": "default", "message": "missing required label app.kubernetes.io/name"},
		map[string]interface{}{"name": "api-pod", "namespace": "api", "message": "missing required label app.kubernetes.io/name"},
	}, "status", "violations")

	constraint2 := &unstructured.Unstructured{}
	constraint2.SetKind("K8sContainerLimits")
	constraint2.SetName("require-limits")
	constraint2.SetNamespace("")
	unstructured.SetNestedField(constraint2.Object, "audit", "spec", "enforcementAction")

	constraints := []unstructured.Unstructured{*constraint1, *constraint2}

	result := opaComplianceAuditCore(gatekeeperPods, nil, constraints, nil)

	if !result.Summary.HasGatekeeper {
		t.Error("expected hasGatekeeper=true")
	}
	if result.Summary.TotalConstraints != 2 {
		t.Errorf("expected totalConstraints=2, got %d", result.Summary.TotalConstraints)
	}
	if result.Summary.EnforcedCount != 1 {
		t.Errorf("expected enforcedCount=1, got %d", result.Summary.EnforcedCount)
	}
	if result.Summary.AuditMode != 1 {
		t.Errorf("expected auditMode=1, got %d", result.Summary.AuditMode)
	}
	if result.Summary.ViolationCount != 2 {
		t.Errorf("expected violationCount=2, got %d", result.Summary.ViolationCount)
	}
	if len(result.Violations) != 2 {
		t.Errorf("expected 2 violations, got %d", len(result.Violations))
	}
	if result.Summary.NamespacesWithViolations != 2 {
		t.Errorf("expected 2 namespaces with violations, got %d", result.Summary.NamespacesWithViolations)
	}
	if len(result.Constraints) != 2 {
		t.Errorf("expected 2 constraints, got %d", len(result.Constraints))
	}
	if len(result.Recommendations) == 0 {
		t.Error("expected recommendations")
	}
}
