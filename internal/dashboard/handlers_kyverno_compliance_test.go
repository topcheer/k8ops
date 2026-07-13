package dashboard

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseKyvernoPolicy(t *testing.T) {
	// Policy with validate rules, Enforce mode
	item := unstructured.Unstructured{}
	item.Object = map[string]interface{}{
		"apiVersion": "kyverno.io/v1",
		"kind":       "ClusterPolicy",
		"metadata": map[string]interface{}{
			"name": "disallow-latest-tag",
		},
		"spec": map[string]interface{}{
			"failurePolicy": "Enforce",
			"background":    true,
			"rules": []interface{}{
				map[string]interface{}{
					"name":     "require-image-tag",
					"validate": map[string]interface{}{},
				},
				map[string]interface{}{
					"name":   "mutate-label",
					"mutate": map[string]interface{}{},
				},
			},
		},
		"status": map[string]interface{}{
			"violations": []interface{}{
				map[string]interface{}{},
				map[string]interface{}{},
			},
		},
	}

	policy := parseKyvernoPolicy(item)
	if policy.Name != "disallow-latest-tag" {
		t.Errorf("name = %s, want disallow-latest-tag", policy.Name)
	}
	if policy.Enforcement != "Enforce" {
		t.Errorf("enforcement = %s, want Enforce", policy.Enforcement)
	}
	if policy.ValidationRules != 1 {
		t.Errorf("validationRules = %d, want 1", policy.ValidationRules)
	}
	if policy.MutationRules != 1 {
		t.Errorf("mutationRules = %d, want 1", policy.MutationRules)
	}
	if policy.Violations != 2 {
		t.Errorf("violations = %d, want 2", policy.Violations)
	}
	if policy.BackgroundScan != "enabled" {
		t.Errorf("backgroundScan = %s, want enabled", policy.BackgroundScan)
	}
}

func TestParseKyvernoPolicyDefaults(t *testing.T) {
	// Minimal policy with no spec fields
	item := unstructured.Unstructured{}
	item.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "minimal-policy",
		},
	}

	policy := parseKyvernoPolicy(item)
	if policy.Enforcement != "Audit" {
		t.Errorf("default enforcement = %s, want Audit", policy.Enforcement)
	}
	if policy.BackgroundScan != "default" {
		t.Errorf("default backgroundScan = %s, want default", policy.BackgroundScan)
	}
	if policy.RiskLevel != "info" {
		t.Errorf("minimal policy risk = %s, want info", policy.RiskLevel)
	}
}

func TestExtractKyvernoVersion(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"kyverno/kyverno:v1.9.0", "v1.9.0"},
		{"kyverno/kyverno:v1.12.5-scratch", "v1.12.5"},
		{"kyverno/kyverno", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractKyvernoVersion(tt.image); got != tt.expected {
			t.Errorf("extractKyvernoVersion(%q) = %q, want %q", tt.image, got, tt.expected)
		}
	}
}

func TestAssessKyvernoPolicyRisk(t *testing.T) {
	// Healthy policy: Enforce, no violations, background enabled
	policy := KyvernoPolicyEntry{
		Enforcement:     "Enforce",
		Violations:      0,
		BackgroundScan:  "enabled",
		ValidationRules: 1,
	}
	if got := assessKyvernoPolicyRisk(policy); got != "healthy" {
		t.Errorf("healthy policy risk = %s, want healthy", got)
	}

	// Critical: violations + audit mode + background disabled
	policy = KyvernoPolicyEntry{
		Enforcement:     "Audit",
		Violations:      5,
		BackgroundScan:  "disabled",
		ValidationRules: 2,
	}
	if got := assessKyvernoPolicyRisk(policy); got != "critical" {
		t.Errorf("critical policy risk = %s, want critical", got)
	}
}

func TestComputeKyvernoHealthScore(t *testing.T) {
	// No Kyverno → 50
	score := computeKyvernoHealthScore(KyvernoSummary{}, 0)
	if score != 50 {
		t.Fatalf("no-kyverno score = %d, want 50", score)
	}

	// All healthy
	score = computeKyvernoHealthScore(KyvernoSummary{
		KyvernoDetected: true,
		PodCount:        3,
		ReadyPods:       3,
		TotalPolicies:   10,
		EnforcePolicies: 10,
	}, 0)
	if score != 100 {
		t.Fatalf("all-healthy score = %d, want 100", score)
	}

	// Not ready pods + violations
	score = computeKyvernoHealthScore(KyvernoSummary{
		KyvernoDetected: true,
		PodCount:        3,
		ReadyPods:       1,
		ViolationCount:  4,
		TotalPolicies:   5,
	}, 3)
	if score > 55 || score < 0 {
		t.Fatalf("problem-heavy score = %d, expected 0-55", score)
	}

	// No policies penalty
	score = computeKyvernoHealthScore(KyvernoSummary{
		KyvernoDetected: true,
		PodCount:        1,
		ReadyPods:       1,
		TotalPolicies:   0,
	}, 0)
	if score > 95 {
		t.Fatalf("no-policies score = %d, expected <= 95", score)
	}
}
