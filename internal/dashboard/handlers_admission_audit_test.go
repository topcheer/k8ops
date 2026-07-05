package dashboard

import (
	"testing"
)

func TestIsSystemWebhook(t *testing.T) {
	tests := []struct {
		name   string
		expect bool
	}{
		{"pod-security.admission.config.k8s.io", true},
		{"kube-system-webhook", true},
		{"my-validating-webhook", false},
		{"cert-manager-webhook", false},
		{"istio-validator", false},
	}

	for _, tt := range tests {
		got := isSystemWebhook(tt.name)
		if got != tt.expect {
			t.Errorf("isSystemWebhook(%q) = %v, want %v", tt.name, got, tt.expect)
		}
	}
}

func TestAssessAdmissionRisk(t *testing.T) {
	// Critical — no CA bundle
	entry := WebhookEntry{
		HasCABundle: false, // +30
	}
	if level := assessAdmissionRisk(entry); level != "critical" {
		t.Errorf("Expected critical for no CA bundle, got %s", level)
	}

	// High — Ignore + no selector
	entry = WebhookEntry{
		HasCABundle:    true,
		FailurePolicy:  "Ignore", // +15
		HasNSSelector:  false,    // +10
		TimeoutSeconds: 10,
	}
	// 15 + 10 = 25 → high
	if level := assessAdmissionRisk(entry); level != "high" {
		t.Errorf("Expected high for Ignore+no-selector, got %s", level)
	}

	// Medium — broad scope only
	entry = WebhookEntry{
		HasCABundle:   true,
		FailurePolicy: "Fail",
		HasNSSelector: true,
		Rules:         []string{"core/v1/*:CREATE,UPDATE"}, // +5
	}
	if level := assessAdmissionRisk(entry); level != "medium" {
		t.Errorf("Expected medium for broad scope, got %s", level)
	}

	// Low — clean
	entry = WebhookEntry{
		HasCABundle:    true,
		FailurePolicy:  "Fail",
		HasNSSelector:  true,
		TimeoutSeconds: 10,
		Rules:          []string{"apps/v1/Deployment:CREATE"},
	}
	if level := assessAdmissionRisk(entry); level != "low" {
		t.Errorf("Expected low for clean webhook, got %s", level)
	}
}

func TestCalculateAdmissionScore(t *testing.T) {
	// No webhooks
	if score := calculateAdmissionScore(AdmissionSummary{}); score != 100 {
		t.Errorf("Expected 100 for empty, got %d", score)
	}

	// With issues
	s := AdmissionSummary{
		TotalValidating:     3,
		TotalMutating:       2,
		NoCABundle:          1, // -15
		FailurePolicyIgnore: 2, // -10
		NoNamespaceSelector: 3, // -9
		BroadScope:          1, // -2
	}
	// 100 - 15 - 10 - 9 - 2 = 64
	if score := calculateAdmissionScore(s); score != 64 {
		t.Errorf("Expected 64, got %d", score)
	}

	// Floor at 0
	terrible := AdmissionSummary{
		TotalValidating: 5,
		NoCABundle:      10, // -150
	}
	if score := calculateAdmissionScore(terrible); score != 0 {
		t.Errorf("Expected 0, got %d", score)
	}
}

func TestGenerateAdmissionRecs(t *testing.T) {
	s := AdmissionSummary{
		NoCABundle:          2,
		FailurePolicyIgnore: 1,
		NoNamespaceSelector: 3,
		BroadScope:          1,
		TimeoutShort:        2,
		SecurityScore:       40,
	}

	recs := generateAdmissionRecs(s)

	if len(recs) < 5 {
		t.Errorf("Expected at least 5 recommendations, got %d", len(recs))
	}

	foundCABundle := false
	foundIgnore := false
	foundSelector := false
	for _, r := range recs {
		if containsSubstr(r, "CA bundle") {
			foundCABundle = true
		}
		if containsSubstr(r, "Ignore") {
			foundIgnore = true
		}
		if containsSubstr(r, "namespaceSelector") {
			foundSelector = true
		}
	}
	if !foundCABundle {
		t.Error("Expected recommendation about CA bundle")
	}
	if !foundIgnore {
		t.Error("Expected recommendation about failurePolicy=Ignore")
	}
	if !foundSelector {
		t.Error("Expected recommendation about namespaceSelector")
	}
}

func TestGenerateAdmissionRecsClean(t *testing.T) {
	s := AdmissionSummary{
		TotalValidating: 2,
		TotalMutating:   1,
		SecurityScore:   100,
	}
	recs := generateAdmissionRecs(s)
	if len(recs) != 0 {
		t.Errorf("Expected 0 recommendations for clean, got %d", len(recs))
	}
}

func TestAdmissionRiskRank(t *testing.T) {
	if admissionRiskRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if admissionRiskRank("high") != 1 {
		t.Error("Expected 1 for high")
	}
	if admissionRiskRank("medium") != 2 {
		t.Error("Expected 2 for medium")
	}
	if admissionRiskRank("low") != 3 {
		t.Error("Expected 3 for low")
	}
}

func TestAdmissionIssueRank(t *testing.T) {
	if admissionIssueRank("critical") != 0 {
		t.Error("Expected 0 for critical")
	}
	if admissionIssueRank("warning") != 1 {
		t.Error("Expected 1 for warning")
	}
	if admissionIssueRank("info") != 2 {
		t.Error("Expected 2 for info")
	}
}
