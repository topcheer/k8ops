// Package safety implements guardrails for AI-initiated actions.
package safety

import (
	"fmt"
	"strings"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
)

// Level maps risk level strings to numeric values for comparison.
var riskLevels = map[string]int{
	"low":      1,
	"medium":   2,
	"high":     3,
	"critical": 4,
}

// Checker validates actions against safety policies.
type Checker struct {
	config *aiv1alpha1.SafetySpec
	auto   *aiv1alpha1.AutoRemediationSpec
}

// NewChecker creates a safety checker from config.
func NewChecker(safety *aiv1alpha1.SafetySpec, auto *aiv1alpha1.AutoRemediationSpec) *Checker {
	return &Checker{config: safety, auto: auto}
}

// CheckAction validates whether an action is allowed.
type CheckResult struct {
	Allowed  bool   `json:"allowed"`
	Reason   string `json:"reason,omitempty"`
	RiskLevel string `json:"riskLevel"`
}

// CheckAction validates whether a remediation action is allowed.
func (c *Checker) CheckAction(action aiv1alpha1.RemediationAction) CheckResult {
	risk := strings.ToLower(action.Risk)
	if risk == "" {
		risk = "medium"
	}

	// Check denied operations
	if c.config != nil {
		for _, denied := range c.config.DeniedOperations {
			if strings.EqualFold(denied, action.Type) {
				return CheckResult{Allowed: false, Reason: fmt.Sprintf("operation %s is in the denied list", action.Type), RiskLevel: risk}
			}
		}
	}

	// Check allowed operations (if non-empty, it's a whitelist)
	if c.config != nil && len(c.config.AllowedOperations) > 0 {
		found := false
		for _, allowed := range c.config.AllowedOperations {
			if strings.EqualFold(allowed, action.Type) {
				found = true
				break
			}
		}
		if !found {
			return CheckResult{Allowed: false, Reason: fmt.Sprintf("operation %s is not in the allowed list", action.Type), RiskLevel: risk}
		}
	}

	// Check namespace restrictions
	if c.auto != nil && action.Target != nil && action.Target.Namespace != "" {
		// Check deny list
		for _, ns := range c.auto.NamespaceDenyList {
			if ns == action.Target.Namespace {
				return CheckResult{Allowed: false, Reason: fmt.Sprintf("namespace %s is in deny list", action.Target.Namespace), RiskLevel: risk}
			}
		}
		// Check allow list (if non-empty)
		if len(c.auto.NamespaceAllowList) > 0 {
			found := false
			for _, ns := range c.auto.NamespaceAllowList {
				if ns == action.Target.Namespace {
					found = true
					break
				}
			}
			if !found {
				return CheckResult{Allowed: false, Reason: fmt.Sprintf("namespace %s not in allow list", action.Target.Namespace), RiskLevel: risk}
			}
		}
	}

	// Check risk level
	maxRisk := "medium"
	if c.auto != nil && c.auto.MaxRiskLevel != "" {
		maxRisk = strings.ToLower(c.auto.MaxRiskLevel)
	}
	if riskLevels[risk] > riskLevels[maxRisk] {
		return CheckResult{Allowed: false, Reason: fmt.Sprintf("risk level %s exceeds max allowed %s", risk, maxRisk), RiskLevel: risk}
	}

	// Check protected resources
	if c.config != nil && action.Target != nil {
		for _, protected := range c.config.ProtectedResources {
			if (protected.Name == "" || protected.Name == action.Target.Name) &&
				(protected.Kind == "" || strings.EqualFold(protected.Kind, action.Target.Kind)) &&
				(protected.Namespace == "" || protected.Namespace == action.Target.Namespace) {
				return CheckResult{Allowed: false, Reason: fmt.Sprintf("resource %s/%s is protected", action.Target.Kind, action.Target.Name), RiskLevel: risk}
			}
		}
	}

	return CheckResult{Allowed: true, RiskLevel: risk}
}

// RiskCompare returns true if level1 >= level2.
func RiskCompare(level1, level2 string) bool {
	return riskLevels[strings.ToLower(level1)] >= riskLevels[strings.ToLower(level2)]
}
