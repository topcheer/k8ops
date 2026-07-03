package safety

import (
	"testing"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
)

func TestCheckAction_Allowed(t *testing.T) {
	checker := NewChecker(nil, &aiv1alpha1.AutoRemediationSpec{
		Enabled:      true,
		MaxRiskLevel: "medium",
	})

	action := aiv1alpha1.RemediationAction{
		Type:        "patchResource",
		Description: "Test patch",
		Risk:        "low",
	}

	result := checker.CheckAction(action)
	if !result.Allowed {
		t.Errorf("expected action to be allowed, but was denied: %s", result.Reason)
	}
}

func TestCheckAction_RiskTooHigh(t *testing.T) {
	checker := NewChecker(nil, &aiv1alpha1.AutoRemediationSpec{
		Enabled:      true,
		MaxRiskLevel: "low",
	})

	action := aiv1alpha1.RemediationAction{
		Type: "patchResource",
		Risk: "high",
	}

	result := checker.CheckAction(action)
	if result.Allowed {
		t.Error("expected high-risk action to be denied when maxRisk is low")
	}
}

func TestCheckAction_DeniedOperation(t *testing.T) {
	checker := NewChecker(&aiv1alpha1.SafetySpec{
		DeniedOperations: []string{"deleteResource"},
	}, nil)

	action := aiv1alpha1.RemediationAction{
		Type: "deleteResource",
		Risk: "low",
	}

	result := checker.CheckAction(action)
	if result.Allowed {
		t.Error("expected denied operation to be blocked")
	}
}

func TestCheckAction_OperationNotInAllowList(t *testing.T) {
	checker := NewChecker(&aiv1alpha1.SafetySpec{
		AllowedOperations: []string{"patchResource", "scaleResource"},
	}, nil)

	action := aiv1alpha1.RemediationAction{
		Type: "deleteResource",
		Risk: "low",
	}

	result := checker.CheckAction(action)
	if result.Allowed {
		t.Error("expected operation not in allow list to be blocked")
	}
}

func TestCheckAction_NamespaceDenyList(t *testing.T) {
	checker := NewChecker(nil, &aiv1alpha1.AutoRemediationSpec{
		Enabled:           true,
		MaxRiskLevel:      "high",
		NamespaceDenyList: []string{"kube-system"},
	})

	action := aiv1alpha1.RemediationAction{
		Type: "patchResource",
		Risk: "low",
		Target: &aiv1alpha1.ResourceRef{
			Kind:      "Deployment",
			Namespace: "kube-system",
			Name:      "coredns",
		},
	}

	result := checker.CheckAction(action)
	if result.Allowed {
		t.Error("expected action in deny-listed namespace to be blocked")
	}
}

func TestCheckAction_ProtectedResource(t *testing.T) {
	checker := NewChecker(&aiv1alpha1.SafetySpec{
		ProtectedResources: []aiv1alpha1.ResourceRef{
			{Kind: "Deployment", Namespace: "production", Name: "critical-app"},
		},
	}, &aiv1alpha1.AutoRemediationSpec{
		Enabled:      true,
		MaxRiskLevel: "high",
	})

	action := aiv1alpha1.RemediationAction{
		Type: "patchResource",
		Risk: "low",
		Target: &aiv1alpha1.ResourceRef{
			Kind:      "Deployment",
			Namespace: "production",
			Name:      "critical-app",
		},
	}

	result := checker.CheckAction(action)
	if result.Allowed {
		t.Error("expected action on protected resource to be blocked")
	}
}

func TestCheckAction_NamespaceAllowList(t *testing.T) {
	checker := NewChecker(nil, &aiv1alpha1.AutoRemediationSpec{
		Enabled:            true,
		MaxRiskLevel:       "high",
		NamespaceAllowList: []string{"default", "dev"},
	})

	// Action in allowed namespace - should pass
	action1 := aiv1alpha1.RemediationAction{
		Type:   "patchResource",
		Risk:   "low",
		Target: &aiv1alpha1.ResourceRef{Namespace: "default"},
	}
	if !checker.CheckAction(action1).Allowed {
		t.Error("expected action in allowed namespace to pass")
	}

	// Action in non-allowed namespace - should be blocked
	action2 := aiv1alpha1.RemediationAction{
		Type:   "patchResource",
		Risk:   "low",
		Target: &aiv1alpha1.ResourceRef{Namespace: "staging"},
	}
	if checker.CheckAction(action2).Allowed {
		t.Error("expected action in non-allowed namespace to be blocked")
	}
}

func TestRiskCompare(t *testing.T) {
	if !RiskCompare("high", "medium") {
		t.Error("high >= medium should be true")
	}
	if RiskCompare("low", "high") {
		t.Error("low >= high should be false")
	}
	if !RiskCompare("critical", "critical") {
		t.Error("critical >= critical should be true")
	}
}

func TestCheckAction_DefaultRiskLevel(t *testing.T) {
	checker := NewChecker(nil, &aiv1alpha1.AutoRemediationSpec{
		Enabled:      true,
		MaxRiskLevel: "medium",
	})

	// Empty risk should default to medium and pass
	action := aiv1alpha1.RemediationAction{
		Type: "patchResource",
		Risk: "",
	}

	result := checker.CheckAction(action)
	if !result.Allowed {
		t.Error("expected action with empty risk (defaults to medium) to be allowed when maxRisk=medium")
	}
	if result.RiskLevel != "medium" {
		t.Errorf("expected risk level 'medium', got '%s'", result.RiskLevel)
	}
}
