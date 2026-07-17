package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdmissionAuditResult analyzes admission controller posture: webhook configs,
// OPA/Gatekeeper policies, Kyverno rules, and enforcement coverage.
type AdmissionAuditResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         AdmissionSummary   `json:"summary"`
	Findings        []AdmissionFinding `json:"findings"`
	PostureScore    int                `json:"postureScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type AdmissionSummary struct {
	HasGatekeeper       bool `json:"hasGatekeeper"`
	HasKyverno          bool `json:"hasKyverno"`
	HasOPA              bool `json:"hasOPA"`
	ValidatingWebhooks  int  `json:"validatingWebhooks"`
	MutatingWebhooks    int  `json:"mutatingWebhooks"`
	EnforceMode         bool `json:"enforceMode"`
	PolicyCount         int  `json:"policyCount"`
	NoCABundle          int  `json:"noCABundle"`
	FailurePolicyIgnore int  `json:"failurePolicyIgnore"`
	NoNamespaceSelector int  `json:"noNamespaceSelector"`
	TotalValidating     int  `json:"totalValidating"`
	TotalMutating       int  `json:"totalMutating"`
	BroadScope          int  `json:"broadScope"`
	TimeoutShort        int  `json:"timeoutShort"`
	SecurityScore       int  `json:"securityScore"`
}

func assessAdmissionRisk(entry WebhookEntry) string {
	if !entry.HasCABundle {
		return "critical"
	}
	riskPoints := 0
	if entry.FailurePolicy == "Ignore" {
		riskPoints += 15
	}
	if !entry.HasNSSelector {
		riskPoints += 10
	}
	// Check for broad scope rules (containing wildcard)
	for _, rule := range entry.Rules {
		if strings.Contains(rule, "*") {
			riskPoints += 5
			break
		}
	}
	if riskPoints >= 25 {
		return "high"
	}
	if riskPoints > 0 {
		return "medium"
	}
	return "low"
}

func calculateAdmissionScore(s AdmissionSummary) int {
	score := 100
	score -= s.NoCABundle * 15
	score -= s.FailurePolicyIgnore * 5
	score -= s.NoNamespaceSelector * 3
	score -= s.BroadScope * 2
	if score < 0 {
		score = 0
	}
	return score
}

func generateAdmissionRecs(s AdmissionSummary) []string {
	var recs []string
	if s.NoCABundle > 0 {
		recs = append(recs, fmt.Sprintf("%d webhooks without CA bundle — insecure admission control", s.NoCABundle))
	}
	if s.FailurePolicyIgnore > 0 {
		recs = append(recs, fmt.Sprintf("%d webhooks with failurePolicy=Ignore — failures silently ignored", s.FailurePolicyIgnore))
	}
	if s.NoNamespaceSelector > 0 {
		recs = append(recs, fmt.Sprintf("%d webhooks without namespaceSelector — broad scope risk", s.NoNamespaceSelector))
	}
	if s.BroadScope > 0 {
		recs = append(recs, fmt.Sprintf("%d webhooks with broad scope rules — scope to specific resources", s.BroadScope))
	}
	if s.TimeoutShort > 0 {
		recs = append(recs, fmt.Sprintf("%d webhooks with short timeout — may cause request failures", s.TimeoutShort))
	}
	if s.SecurityScore < 50 {
		recs = append(recs, "Admission security score below 50 — review webhook configurations")
	}
	return recs
}

func admissionRiskRank(risk string) int {
	switch risk {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	}
	return 4
}

func admissionIssueRank(severity string) int {
	switch severity {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	}
	return 3
}

// strContains checks if a string contains a substring (case-insensitive).
func strContains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

type AdmissionFinding struct {
	Category string `json:"category"`
	Finding  string `json:"finding"`
	Severity string `json:"severity"`
	Impact   string `json:"impact"`
}

// handleAdmissionAudit analyzes admission controller posture.
// GET /api/security/admission-audit
func (s *Server) handleAdmissionAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AdmissionAuditResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Detect admission controllers
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			if strings.Contains(imgLower, "gatekeeper") || strings.Contains(imgLower, "opa") {
				result.Summary.HasGatekeeper = true
				result.Summary.HasOPA = true
			}
			if strings.Contains(imgLower, "kyverno") {
				result.Summary.HasKyverno = true
			}
		}
	}

	// Check webhook configurations via discovery
	_, vwhErr := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if vwhErr == nil {
		// If we can list, count them
		vwhList, err := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
		if err == nil {
			result.Summary.ValidatingWebhooks = len(vwhList.Items)
		}
	}

	mwhList, err := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.MutatingWebhooks = len(mwhList.Items)
	}

	// Findings
	if !result.Summary.HasGatekeeper && !result.Summary.HasKyverno {
		result.Findings = append(result.Findings, AdmissionFinding{
			Category: "policy-engine", Finding: "No OPA/Gatekeeper or Kyverno detected",
			Severity: "critical", Impact: "No policy enforcement — any resource can be created without validation",
		})
	}
	if result.Summary.ValidatingWebhooks == 0 {
		result.Findings = append(result.Findings, AdmissionFinding{
			Category: "validating-webhooks", Finding: "No validating webhooks configured",
			Severity: "high", Impact: "No admission validation beyond built-in K8s defaults",
		})
	}
	result.Summary.EnforceMode = result.Summary.HasGatekeeper || result.Summary.HasKyverno
	result.Summary.PolicyCount = result.Summary.ValidatingWebhooks + result.Summary.MutatingWebhooks

	// Score
	score := 20
	if result.Summary.HasGatekeeper || result.Summary.HasKyverno {
		score += 40
	}
	if result.Summary.ValidatingWebhooks > 0 {
		score += 20
	}
	if result.Summary.MutatingWebhooks > 0 {
		score += 10
	}
	if result.Summary.EnforceMode {
		score += 10
	}
	result.PostureScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.PostureScore)

	sort.Slice(result.Findings, func(i, j int) bool {
		return result.Findings[i].Severity > result.Findings[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Admission posture: %d/100 (grade %s) — Gatekeeper:%v Kyverno:%v Webhooks:%d", result.PostureScore, result.Grade, result.Summary.HasGatekeeper, result.Summary.HasKyverno, result.Summary.ValidatingWebhooks))
	if !result.Summary.HasGatekeeper && !result.Summary.HasKyverno {
		recs = append(recs, "Install OPA Gatekeeper or Kyverno for policy-as-code enforcement")
	}
	if result.Summary.ValidatingWebhooks == 0 {
		recs = append(recs, "Add validating webhooks for security-critical resources (secrets, RBAC, pods)")
	}
	if len(recs) == 1 {
		recs = append(recs, "Admission control posture is comprehensive")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}

// isSystemWebhook checks if a webhook config is a system/built-in one.
func isSystemWebhook(name string) bool {
	systemPrefixes := []string{"pod-security.", "kube-system-"}
	lower := strings.ToLower(name)
	for _, prefix := range systemPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// WebhookEntry represents a single webhook configuration entry.
type WebhookEntry struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Namespace      string   `json:"namespace"`
	IsSystem       bool     `json:"isSystem"`
	Severity       string   `json:"severity"`
	ServiceNS      string   `json:"serviceNamespace"`
	Enforced       bool     `json:"enforced"`
	HasCABundle    bool     `json:"hasCABundle"`
	FailurePolicy  string   `json:"failurePolicy"`
	TimeoutSeconds int32    `json:"timeoutSeconds"`
	HasNSSelector  bool     `json:"hasNSSelector"`
	Rules          []string `json:"rules"`
}

// AdmissionIssue represents a finding from the admission audit.
type AdmissionIssue struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func rankWebhookSeverity(name string) string {
	if isSystemWebhook(name) {
		return "low"
	}
	return "medium"
}
