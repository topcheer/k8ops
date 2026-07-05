package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdmissionAuditResult is the admission webhook configuration audit.
type AdmissionAuditResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         AdmissionSummary `json:"summary"`
	ValidatingHooks []WebhookEntry   `json:"validatingWebhooks"`
	MutatingHooks   []WebhookEntry   `json:"mutatingWebhooks"`
	Issues          []AdmissionIssue `json:"issues"`
	Recommendations []string         `json:"recommendations"`
}

// AdmissionSummary aggregates webhook configuration health.
type AdmissionSummary struct {
	TotalValidating     int `json:"totalValidating"`
	TotalMutating       int `json:"totalMutating"`
	HealthyHooks        int `json:"healthyHooks"`
	WithIssues          int `json:"withIssues"`
	FailurePolicyIgnore int `json:"failurePolicyIgnore"` // failurePolicy: Ignore (silent fail)
	FailurePolicyFail   int `json:"failurePolicyFail"`   // failurePolicy: Fail (block)
	NoNamespaceSelector int `json:"noNamespaceSelector"` // catches all namespaces
	NoCABundle          int `json:"noCABundle"`          // missing CA bundle
	BroadScope          int `json:"broadScope"`          // matches */* resources
	MatchedOnAllOps     int `json:"matchedOnAllOps"`     // CREATE+UPDATE+DELETE without filtering
	TimeoutShort        int `json:"timeoutShort"`        // timeout < 3s
	SecurityScore       int `json:"securityScore"`       // 0-100
}

// WebhookEntry describes one webhook configuration.
type WebhookEntry struct {
	Name             string   `json:"name"`
	Type             string   `json:"type"` // validating / mutating
	ServiceNamespace string   `json:"serviceNamespace"`
	ServiceName      string   `json:"serviceName"`
	ServicePath      string   `json:"servicePath"`
	FailurePolicy    string   `json:"failurePolicy"`
	MatchPolicy      string   `json:"matchPolicy"`
	TimeoutSeconds   int32    `json:"timeoutSeconds"`
	HasCABundle      bool     `json:"hasCABundle"`
	HasNSSelector    bool     `json:"hasNamespaceSelector"`
	HasObjSelector   bool     `json:"hasObjectSelector"`
	Rules            []string `json:"rules"` // e.g. ["apps/v1/Deployment:CREATE,UPDATE"]
	SideEffects      string   `json:"sideEffects"`
	RiskLevel        string   `json:"riskLevel"`
	Issues           []string `json:"issues"`
}

// AdmissionIssue is a detected webhook problem.
type AdmissionIssue struct {
	Webhook  string `json:"webhook"`
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Message  string `json:"message"`
}

// handleAdmissionAudit audits all admission webhook configurations.
// GET /api/security/admission-audit
func (s *Server) handleAdmissionAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	vwbs, err := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	mwbs, err := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := AdmissionAuditResult{ScannedAt: time.Now()}

	// Analyze validating webhooks
	for _, vwc := range vwbs.Items {
		if isSystemWebhook(vwc.Name) {
			continue
		}
		for _, wh := range vwc.Webhooks {
			entry := analyzeValidatingWebhook(wh, vwc.Name)
			result.ValidatingHooks = append(result.ValidatingHooks, entry)
			result.Summary.TotalValidating++

			if len(entry.Issues) > 0 {
				result.Summary.WithIssues++
			} else {
				result.Summary.HealthyHooks++
			}

			aggregateWebhookIssues(entry, &result.Summary, &result.Issues, vwc.Name)
		}
	}

	// Analyze mutating webhooks
	for _, mwc := range mwbs.Items {
		if isSystemWebhook(mwc.Name) {
			continue
		}
		for _, wh := range mwc.Webhooks {
			entry := analyzeMutatingWebhook(wh, mwc.Name)
			result.MutatingHooks = append(result.MutatingHooks, entry)
			result.Summary.TotalMutating++

			if len(entry.Issues) > 0 {
				result.Summary.WithIssues++
			} else {
				result.Summary.HealthyHooks++
			}

			aggregateWebhookIssues(entry, &result.Summary, &result.Issues, mwc.Name)
		}
	}

	// Sort by risk
	sort.Slice(result.ValidatingHooks, func(i, j int) bool {
		return admissionRiskRank(result.ValidatingHooks[i].RiskLevel) < admissionRiskRank(result.ValidatingHooks[j].RiskLevel)
	})
	sort.Slice(result.MutatingHooks, func(i, j int) bool {
		return admissionRiskRank(result.MutatingHooks[i].RiskLevel) < admissionRiskRank(result.MutatingHooks[j].RiskLevel)
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return admissionIssueRank(result.Issues[i].Severity) < admissionIssueRank(result.Issues[j].Severity)
	})

	result.Summary.SecurityScore = calculateAdmissionScore(result.Summary)
	result.Recommendations = generateAdmissionRecs(result.Summary)

	writeJSON(w, result)
}

// analyzeValidatingWebhook evaluates a single validating webhook entry.
func analyzeValidatingWebhook(wh admissionv1.ValidatingWebhook, configName string) WebhookEntry {
	mp, se := "Exact", "Unknown"
	if wh.MatchPolicy != nil {
		mp = string(*wh.MatchPolicy)
	}
	if wh.SideEffects != nil {
		se = string(*wh.SideEffects)
	}
	return analyzeWebhookCommon(wh.Name, mp, se,
		wh.FailurePolicy, wh.TimeoutSeconds, wh.ClientConfig,
		wh.NamespaceSelector, wh.ObjectSelector, wh.Rules)
}

// analyzeMutatingWebhook evaluates a single mutating webhook entry.
func analyzeMutatingWebhook(wh admissionv1.MutatingWebhook, configName string) WebhookEntry {
	mp, se := "Exact", "Unknown"
	if wh.MatchPolicy != nil {
		mp = string(*wh.MatchPolicy)
	}
	if wh.SideEffects != nil {
		se = string(*wh.SideEffects)
	}
	return analyzeWebhookCommon(wh.Name, mp, se,
		wh.FailurePolicy, wh.TimeoutSeconds, wh.ClientConfig,
		wh.NamespaceSelector, wh.ObjectSelector, wh.Rules)
}

// analyzeWebhookCommon contains the shared analysis logic.
func analyzeWebhookCommon(
	name, matchPolicy, sideEffects string,
	failurePolicy *admissionv1.FailurePolicyType, timeoutSeconds *int32,
	clientConfig admissionv1.WebhookClientConfig,
	nsSelector, objSelector *metav1.LabelSelector,
	rules []admissionv1.RuleWithOperations,
) WebhookEntry {
	entry := WebhookEntry{
		Name:           name,
		FailurePolicy:  "Ignore",
		TimeoutSeconds: 10,
		MatchPolicy:    matchPolicy,
		SideEffects:    sideEffects,
	}

	if failurePolicy != nil {
		entry.FailurePolicy = string(*failurePolicy)
	}
	if timeoutSeconds != nil {
		entry.TimeoutSeconds = *timeoutSeconds
	}

	// Service info
	if clientConfig.Service != nil {
		entry.ServiceNamespace = clientConfig.Service.Namespace
		entry.ServiceName = clientConfig.Service.Name
		entry.ServicePath = *clientConfig.Service.Path
	}

	// CA bundle
	entry.HasCABundle = len(clientConfig.CABundle) > 0

	// Namespace selector
	if nsSelector != nil && (len(nsSelector.MatchExpressions) > 0 || len(nsSelector.MatchLabels) > 0) {
		entry.HasNSSelector = true
	}

	// Object selector
	if objSelector != nil && (len(objSelector.MatchExpressions) > 0 || len(objSelector.MatchLabels) > 0) {
		entry.HasObjSelector = true
	}

	// Rules summary
	for _, rule := range rules {
		for _, res := range rule.Resources {
			ops := ""
			for i, op := range rule.Operations {
				if i > 0 {
					ops += ","
				}
				ops += string(op)
			}
			for _, group := range rule.APIGroups {
				if len(group) == 0 {
					group = "core"
				}
				for _, version := range rule.APIVersions {
					entry.Rules = append(entry.Rules, fmt.Sprintf("%s/%s/%s:%s", group, version, res, ops))
				}
			}
		}
	}

	// Analyze issues
	var issues []string

	if !entry.HasCABundle {
		issues = append(issues, "Missing CA bundle — webhook may fail TLS verification")
	}
	if entry.FailurePolicy == "Ignore" {
		issues = append(issues, "failurePolicy=Ignore — webhook failures are silently ignored")
	}
	if !entry.HasNSSelector {
		issues = append(issues, "No namespaceSelector — webhook matches all namespaces including system namespaces")
	}
	for _, rule := range rules {
		for _, res := range rule.Resources {
			if res == "*" {
				issues = append(issues, "Matches all resources (*) — may impact cluster-wide performance")
				break
			}
		}
	}
	if len(rules) > 0 {
		for _, rule := range rules {
			if len(rule.Operations) >= 3 {
				issues = append(issues, "Matches all operations (CREATE+UPDATE+DELETE) — consider narrowing")
				break
			}
		}
	}
	if entry.TimeoutSeconds < 3 {
		issues = append(issues, fmt.Sprintf("Timeout %ds < 3s — may cause false timeouts under load", entry.TimeoutSeconds))
	}

	entry.Issues = issues
	entry.RiskLevel = assessAdmissionRisk(entry)
	return entry
}

// aggregateWebhookIssues collects summary stats and issues.
func aggregateWebhookIssues(entry WebhookEntry, summary *AdmissionSummary, issues *[]AdmissionIssue, configName string) {
	if entry.FailurePolicy == "Ignore" {
		summary.FailurePolicyIgnore++
		*issues = append(*issues, AdmissionIssue{
			Webhook: configName + "/" + entry.Name, Severity: "warning", Type: "failure-policy-ignore",
			Message: "Webhook uses failurePolicy=Ignore — failures are silently dropped",
		})
	} else {
		summary.FailurePolicyFail++
	}

	if !entry.HasNSSelector {
		summary.NoNamespaceSelector++
		*issues = append(*issues, AdmissionIssue{
			Webhook: configName + "/" + entry.Name, Severity: "warning", Type: "no-namespace-selector",
			Message: "Webhook has no namespaceSelector — affects all namespaces including system",
		})
	}

	if !entry.HasCABundle {
		summary.NoCABundle++
		*issues = append(*issues, AdmissionIssue{
			Webhook: configName + "/" + entry.Name, Severity: "critical", Type: "no-ca-bundle",
			Message: "Webhook has no CA bundle — TLS verification will fail",
		})
	}

	for _, r := range entry.Rules {
		if strContains(r, "/*:") {
			summary.BroadScope++
			*issues = append(*issues, AdmissionIssue{
				Webhook: configName + "/" + entry.Name, Severity: "info", Type: "broad-scope",
				Message: "Webhook matches wildcard resources — may impact performance",
			})
			break
		}
	}

	if entry.TimeoutSeconds < 3 {
		summary.TimeoutShort++
		*issues = append(*issues, AdmissionIssue{
			Webhook: configName + "/" + entry.Name, Severity: "info", Type: "short-timeout",
			Message: fmt.Sprintf("Webhook timeout %ds is very short", entry.TimeoutSeconds),
		})
	}
}

// assessAdmissionRisk determines risk level.
func assessAdmissionRisk(entry WebhookEntry) string {
	risk := 0
	if !entry.HasCABundle {
		risk += 30
	}
	if entry.FailurePolicy == "Ignore" {
		risk += 15
	}
	if !entry.HasNSSelector {
		risk += 10
	}
	for _, r := range entry.Rules {
		if strContains(r, "/*:") {
			risk += 5
			break
		}
	}
	if entry.TimeoutSeconds < 3 {
		risk += 5
	}

	switch {
	case risk >= 30:
		return "critical"
	case risk >= 15:
		return "high"
	case risk >= 5:
		return "medium"
	default:
		return "low"
	}
}

// calculateAdmissionScore computes 0-100.
func calculateAdmissionScore(s AdmissionSummary) int {
	total := s.TotalValidating + s.TotalMutating
	if total == 0 {
		return 100
	}
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

// generateAdmissionRecs produces actionable advice.
func generateAdmissionRecs(s AdmissionSummary) []string {
	var recs []string

	if s.NoCABundle > 0 {
		recs = append(recs, fmt.Sprintf("%d webhook(s) have no CA bundle — TLS verification will fail, add CA bundles immediately", s.NoCABundle))
	}
	if s.FailurePolicyIgnore > 0 {
		recs = append(recs, fmt.Sprintf("%d webhook(s) use failurePolicy=Ignore — failures are silently ignored, consider using Fail with namespace exclusions", s.FailurePolicyIgnore))
	}
	if s.NoNamespaceSelector > 0 {
		recs = append(recs, fmt.Sprintf("%d webhook(s) have no namespaceSelector — add selectors to exclude kube-system and other system namespaces", s.NoNamespaceSelector))
	}
	if s.BroadScope > 0 {
		recs = append(recs, fmt.Sprintf("%d webhook(s) match all resources (*) — narrow to specific resources for better performance", s.BroadScope))
	}
	if s.TimeoutShort > 0 {
		recs = append(recs, fmt.Sprintf("%d webhook(s) have timeout < 3s — increase to at least 10s for production reliability", s.TimeoutShort))
	}
	if s.SecurityScore < 60 {
		recs = append(recs, fmt.Sprintf("Admission webhook security score is %d/100 — review webhook configurations", s.SecurityScore))
	}

	return recs
}

// isSystemWebhook checks if a webhook is from Kubernetes core.
func isSystemWebhook(name string) bool {
	systemPrefixes := []string{
		"cert-manager.io",
		"cilium",
		"istio",
		"linkerd",
	}
	for _, prefix := range systemPrefixes {
		if strContains(name, prefix) {
			return false // These are user-installed, still audit them
		}
	}
	// K8s built-in webhooks
	if strContains(name, "kube-system") || strContains(name, "pod-security.admission.config.k8s.io") {
		return true
	}
	return false
}

func admissionRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func admissionIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

// strContains checks if a string contains a substring.
func strContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

// findSubstring is a simple substring search.
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
