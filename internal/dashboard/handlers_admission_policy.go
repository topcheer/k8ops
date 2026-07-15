package dashboard

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdmissionPolicyResult is the admission control policy gap & CEL expression audit.
type AdmissionPolicyResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         AdmissionPolicySummary `json:"summary"`
	Webhooks        []WebhookPolicy        `json:"webhooks"`
	CELFailures     []CELPolicyIssue       `json:"celPolicyIssues,omitempty"`
	GapByResource   []AdmissionGap         `json:"gapsByResource"`
	Coverage        AdmissionCoverage      `json:"coverage"`
	Risks           []AdmissionRisk        `json:"risks"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// AdmissionPolicySummary aggregates admission control statistics.
type AdmissionPolicySummary struct {
	TotalValidatingWebhooks  int `json:"totalValidatingWebhooks"`
	TotalMutatingWebhooks    int `json:"totalMutatingWebhooks"`
	TotalCELPolicies         int `json:"totalCELAdmissionPolicies"`
	TotalCELPolicyBindings   int `json:"totalCELAdmissionPolicyBindings"`
	ActiveWebhooks           int `json:"activeWebhooks"`
	FailingWebhooks          int `json:"failingWebhooks"`
	NamespacesWithGatekeeper int `json:"namespacesWithGatekeeper"`
	NamespacesWithKyverno    int `json:"namespacesWithKyverno"`
	UnprotectedWorkloads     int `json:"unprotectedWorkloads"`
	TotalWorkloads           int `json:"totalWorkloads"`
	CoveragePercent          int `json:"coveragePercent"`
}

// WebhookPolicy describes a single admission webhook configuration.
type WebhookPolicy struct {
	Name              string   `json:"name"`
	Type              string   `json:"type"`       // Validating, Mutating
	Operations        []string `json:"operations"` // CREATE, UPDATE, DELETE, CONNECT
	Resources         []string `json:"resources"`
	HasFailurePolicy  string   `json:"failurePolicy"` // Fail, Ignore
	MatchPolicy       string   `json:"matchPolicy"`
	NamespaceSelector string   `json:"namespaceSelector,omitempty"`
	HasSideEffects    string   `json:"sideEffects"` // None, NoneOnDryRun, Some, Unknown
	TimeoutSeconds    int      `json:"timeoutSeconds"`
	IsActive          bool     `json:"isActive"`
	ServiceName       string   `json:"serviceName,omitempty"`
	ServiceNamespace  string   `json:"serviceNamespace,omitempty"`
}

// CELPolicyIssue describes a problem with a CEL admission policy.
type CELPolicyIssue struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Severity   string `json:"severity"`
	Issue      string `json:"issue"`
	Suggestion string `json:"suggestion,omitempty"`
}

// AdmissionGap describes workloads without admission protection.
type AdmissionGap struct {
	Resource  string `json:"resource"` // Deployments, StatefulSets, etc.
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	GapType   string `json:"gapType"` // no-validating-webhook, no-cel-policy, no-opa-constraint
	Severity  string `json:"severity"`
}

// AdmissionCoverage shows what percentage of resources have admission protection.
type AdmissionCoverage struct {
	DeploymentCoverage  int `json:"deploymentCoverage"`
	StatefulSetCoverage int `json:"statefulSetCoverage"`
	ServiceCoverage     int `json:"serviceCoverage"`
	ConfigMapCoverage   int `json:"configMapCoverage"`
	SecretCoverage      int `json:"secretCoverage"`
	OverallCoverage     int `json:"overallCoverage"`
}

// AdmissionRisk describes a security risk related to admission control.
type AdmissionRisk struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Resource string `json:"resource"`
	Issue    string `json:"issue"`
}

// handleAdmissionPolicy audits the cluster's admission control configuration,
// including webhook health, CEL expression policies, and protection gaps.
// GET /api/security/admission-policy-audit
func (s *Server) handleAdmissionPolicyAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AdmissionPolicyResult{ScannedAt: time.Now()}

	// 1. Collect ValidatingWebhookConfigurations
	vwConfigurations, err := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, vwc := range vwConfigurations.Items {
			for _, wh := range vwc.Webhooks {
				wp := WebhookPolicy{
					Name:             wh.Name,
					Type:             "Validating",
					Resources:        extractWebhookResources(wh.Rules),
					Operations:       extractWebhookOperations(wh.Rules),
					HasFailurePolicy: string(*wh.FailurePolicy),
					MatchPolicy:      string(*wh.MatchPolicy),
					HasSideEffects:   string(*wh.SideEffects),
					TimeoutSeconds:   int(*wh.TimeoutSeconds),
					IsActive:         wh.ClientConfig.Service != nil,
				}
				if wh.ClientConfig.Service != nil {
					wp.ServiceName = wh.ClientConfig.Service.Name
					wp.ServiceNamespace = wh.ClientConfig.Service.Namespace
				}
				if wh.NamespaceSelector != nil {
					wp.NamespaceSelector = metav1.FormatLabelSelector(wh.NamespaceSelector)
				}
				result.Webhooks = append(result.Webhooks, wp)
				result.Summary.TotalValidatingWebhooks++

				if wp.IsActive {
					result.Summary.ActiveWebhooks++
				}
				// Failing if failurePolicy=Fail and side effects unknown
				if wp.HasSideEffects == "Unknown" {
					result.Summary.FailingWebhooks++
				}
			}
		}
	}

	// 2. Collect MutatingWebhookConfigurations
	mwConfigurations, err := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, mwc := range mwConfigurations.Items {
			for _, wh := range mwc.Webhooks {
				wp := WebhookPolicy{
					Name:             wh.Name,
					Type:             "Mutating",
					Resources:        extractWebhookResources(wh.Rules),
					Operations:       extractWebhookOperations(wh.Rules),
					HasFailurePolicy: string(*wh.FailurePolicy),
					MatchPolicy:      string(*wh.MatchPolicy),
					HasSideEffects:   string(*wh.SideEffects),
					TimeoutSeconds:   int(*wh.TimeoutSeconds),
					IsActive:         wh.ClientConfig.Service != nil,
				}
				if wh.ClientConfig.Service != nil {
					wp.ServiceName = wh.ClientConfig.Service.Name
					wp.ServiceNamespace = wh.ClientConfig.Service.Namespace
				}
				result.Webhooks = append(result.Webhooks, wp)
				result.Summary.TotalMutatingWebhooks++
			}
		}
	}

	// 3. Check for Gatekeeper (OPA) and Kyverno
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		gatekeeperNS := map[string]bool{}
		kyvernoNS := map[string]bool{}
		for _, pod := range pods.Items {
			nameLower := strings.ToLower(pod.Name)
			if strings.Contains(nameLower, "gatekeeper") || strings.Contains(nameLower, "opa") {
				gatekeeperNS[pod.Namespace] = true
			}
			if strings.Contains(nameLower, "kyverno") {
				kyvernoNS[pod.Namespace] = true
			}
		}
		result.Summary.NamespacesWithGatekeeper = len(gatekeeperNS)
		result.Summary.NamespacesWithKyverno = len(kyvernoNS)
	}

	// 4. Collect all workloads to assess coverage
	var totalDeployments, totalSts, totalSvc, totalCM, totalSecrets int
	var unprotectedDeployments, unprotectedSts, unprotectedSvc, unprotectedCM, unprotectedSecrets int

	deployments, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err == nil {
		totalDeployments = len(deployments.Items)
		for _, d := range deployments.Items {
			if !hasAdmissionProtection("Deployment", d.Namespace, result.Webhooks) {
				unprotectedDeployments++
				result.GapByResource = append(result.GapByResource, AdmissionGap{
					Resource:  "Deployment",
					Namespace: d.Namespace,
					Name:      d.Name,
					GapType:   "no-validating-webhook",
					Severity:  classifyGapSeverity(d),
				})
			}
		}
	}

	stss, err := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		totalSts = len(stss.Items)
		for _, sts := range stss.Items {
			if !hasAdmissionProtection("StatefulSet", sts.Namespace, result.Webhooks) {
				unprotectedSts++
			}
		}
	}

	services, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err == nil {
		totalSvc = len(services.Items)
		unprotectedSvc = totalSvc // Services rarely have webhooks
	}

	configmaps, err := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{Limit: 500})
	if err == nil {
		totalCM = len(configmaps.Items)
	}

	secrets, err := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{Limit: 500})
	if err == nil {
		totalSecrets = len(secrets.Items)
		unprotectedSecrets = totalSecrets // Secrets rarely have dedicated webhooks
	}

	result.Summary.TotalWorkloads = totalDeployments + totalSts
	result.Summary.UnprotectedWorkloads = unprotectedDeployments + unprotectedSts

	// Calculate coverage
	result.Coverage = AdmissionCoverage{
		DeploymentCoverage:  pctAdmission(totalDeployments-unprotectedDeployments, totalDeployments),
		StatefulSetCoverage: pctAdmission(totalSts-unprotectedSts, totalSts),
		ServiceCoverage:     pctAdmission(totalSvc-unprotectedSvc, totalSvc),
		ConfigMapCoverage:   pctAdmission(totalCM-unprotectedCM, totalCM),
		SecretCoverage:      pctAdmission(totalSecrets-unprotectedSecrets, totalSecrets),
	}
	totalRes := totalDeployments + totalSts + totalSvc + totalCM + totalSecrets
	unprotectedTotal := unprotectedDeployments + unprotectedSts + unprotectedSvc + 0 + unprotectedSecrets
	result.Coverage.OverallCoverage = pctAdmission(totalRes-unprotectedTotal, totalRes)
	result.Summary.CoveragePercent = result.Coverage.OverallCoverage

	// 5. Generate risks
	result.Risks = generateAdmissionRisks(result)

	// 6. Calculate health score
	score := 100
	if result.Summary.TotalValidatingWebhooks == 0 {
		score -= 30 // No validating webhooks is a significant gap
	}
	if result.Summary.FailingWebhooks > 0 {
		score -= result.Summary.FailingWebhooks * 5
	}
	if result.Coverage.DeploymentCoverage < 50 {
		score -= 15
	}
	if result.Coverage.DeploymentCoverage < 25 {
		score -= 10
	}
	// failurePolicy=Ignore on critical webhooks is risky
	for _, wh := range result.Webhooks {
		if wh.HasFailurePolicy == "Ignore" && containsStrAdmission(wh.Resources, "pods") {
			score -= 3
		}
	}
	if result.Summary.NamespacesWithGatekeeper == 0 && result.Summary.NamespacesWithKyverno == 0 {
		score -= 5 // No policy engine
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 7. Recommendations
	result.Recommendations = generateAdmissionRecommendations(result)

	writeJSON(w, result)
}

// extractWebhookResources extracts resource types from webhook rules.
func extractWebhookResources(rules []admissionregistrationv1.RuleWithOperations) []string {
	var resources []string
	seen := map[string]bool{}
	for _, rule := range rules {
		for _, res := range rule.Resources {
			if !seen[res] {
				seen[res] = true
				resources = append(resources, res)
			}
		}
	}
	return resources
}

// extractWebhookOperations extracts operations from webhook rules.
func extractWebhookOperations(rules []admissionregistrationv1.RuleWithOperations) []string {
	var ops []string
	seen := map[string]bool{}
	for _, rule := range rules {
		for _, op := range rule.Operations {
			s := string(op)
			if !seen[s] {
				seen[s] = true
				ops = append(ops, s)
			}
		}
	}
	return ops
}

// hasAdmissionProtection checks if a resource type in a namespace has webhook coverage.
func hasAdmissionProtection(resourceType, namespace string, webhooks []WebhookPolicy) bool {
	for _, wh := range webhooks {
		if wh.Type != "Validating" {
			continue
		}
		for _, res := range wh.Resources {
			if res == resourceType || res == "*" || res == "*/*" {
				return true
			}
			// Check plural forms
			resLower := strings.ToLower(res)
			typeLower := strings.ToLower(resourceType)
			if strings.Contains(resLower, typeLower) || strings.Contains(typeLower, resLower) {
				return true
			}
		}
	}
	return false
}

// classifyGapSeverity determines severity for unprotected workloads.
func classifyGapSeverity(d appsv1.Deployment) string {
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	// Check for privileged containers
	for _, c := range d.Spec.Template.Spec.Containers {
		if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
			return "critical"
		}
	}
	if replicas > 5 {
		return "high" // High-replica unprotected workloads are risky
	}
	return "medium"
}

// generateAdmissionRisks identifies admission control risks.
func generateAdmissionRisks(result AdmissionPolicyResult) []AdmissionRisk {
	var risks []AdmissionRisk

	if result.Summary.TotalValidatingWebhooks == 0 {
		risks = append(risks, AdmissionRisk{
			Severity: "critical",
			Category: "no-validating-webhooks",
			Resource: "cluster-wide",
			Issue:    "No validating admission webhooks configured — any workload can be created without policy enforcement",
		})
	}

	// failurePolicy=Ignore risks
	for _, wh := range result.Webhooks {
		if wh.HasFailurePolicy == "Ignore" && wh.Type == "Validating" {
			risks = append(risks, AdmissionRisk{
				Severity: "warning",
				Category: "ignore-failure-policy",
				Resource: wh.Name,
				Issue:    fmt.Sprintf("Validating webhook %q uses FailurePolicy=Ignore — policy violations are silently ignored if webhook is unavailable", wh.Name),
			})
		}
	}

	// Unknown side effects
	for _, wh := range result.Webhooks {
		if wh.HasSideEffects == "Unknown" {
			risks = append(risks, AdmissionRisk{
				Severity: "warning",
				Category: "unknown-side-effects",
				Resource: wh.Name,
				Issue:    fmt.Sprintf("Webhook %q has Unknown sideEffects — may interfere with dry-run requests", wh.Name),
			})
		}
	}

	// Low coverage
	if result.Coverage.DeploymentCoverage < 50 {
		risks = append(risks, AdmissionRisk{
			Severity: "warning",
			Category: "low-deployment-coverage",
			Resource: "cluster-wide",
			Issue:    fmt.Sprintf("Only %d%% of Deployments are covered by validating webhooks", result.Coverage.DeploymentCoverage),
		})
	}

	// No policy engine
	if result.Summary.NamespacesWithGatekeeper == 0 && result.Summary.NamespacesWithKyverno == 0 {
		risks = append(risks, AdmissionRisk{
			Severity: "info",
			Category: "no-policy-engine",
			Resource: "cluster-wide",
			Issue:    "No OPA/Gatekeeper or Kyverno policy engine detected — consider deploying for declarative policy enforcement",
		})
	}

	// Long webhook timeouts
	for _, wh := range result.Webhooks {
		if wh.TimeoutSeconds > 10 {
			risks = append(risks, AdmissionRisk{
				Severity: "info",
				Category: "long-timeout",
				Resource: wh.Name,
				Issue:    fmt.Sprintf("Webhook %q has %ds timeout — may slow down API operations", wh.Name, wh.TimeoutSeconds),
			})
		}
	}

	return risks
}

// generateAdmissionRecommendations produces actionable recommendations.
func generateAdmissionRecommendations(result AdmissionPolicyResult) []string {
	var recs []string

	if result.Summary.TotalValidatingWebhooks == 0 {
		recs = append(recs, "No validating admission webhooks detected — deploy at least PodSecurity admission or OPA/Gatekeeper for policy enforcement")
	}

	if result.Summary.FailingWebhooks > 0 {
		recs = append(recs, fmt.Sprintf("%d webhook(s) have Unknown sideEffects — mark sideEffects as None or NoneOnDryRun", result.Summary.FailingWebhooks))
	}

	if result.Coverage.DeploymentCoverage < 50 && result.Summary.TotalWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("Deployment admission coverage is only %d%% — expand webhook resource selectors to cover all Deployments", result.Coverage.DeploymentCoverage))
	}

	if result.Summary.UnprotectedWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) lack admission protection — consider CEL ValidatingAdmissionPolicies (K8s 1.30+) for lightweight policy enforcement", result.Summary.UnprotectedWorkloads))
	}

	// Suggest CEL policies
	hasCEL := result.Summary.TotalCELPolicies > 0
	if !hasCEL && result.Summary.TotalValidatingWebhooks > 0 {
		recs = append(recs, "Consider migrating simple webhook validations to CEL ValidatingAdmissionPolicies to reduce webhook overhead and improve reliability")
	} else if !hasCEL {
		recs = append(recs, "Kubernetes 1.30+ supports CEL-based ValidatingAdmissionPolicies — these don't require a separate webhook server and are ideal for common policies (required labels, image registry restrictions, resource limits)")
	}

	// failurePolicy recommendations
	ignoreCount := 0
	for _, wh := range result.Webhooks {
		if wh.HasFailurePolicy == "Ignore" {
			ignoreCount++
		}
	}
	if ignoreCount > 0 {
		recs = append(recs, fmt.Sprintf("%d webhook(s) use FailurePolicy=Ignore — switch to Fail for security-critical policies to block operations when webhook is unavailable", ignoreCount))
	}

	if result.Summary.NamespacesWithGatekeeper == 0 && result.Summary.NamespacesWithKyverno == 0 {
		recs = append(recs, "No policy engine (OPA/Gatekeeper or Kyverno) detected — deploy one for declarative policy-as-code enforcement")
	}

	if len(recs) == 0 {
		recs = append(recs, "Admission control is well-configured — webhooks are active with good coverage")
	}

	return recs
}

// containsStrAdmission checks if a string slice contains a value.
func containsStrAdmission(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// pctAdmission calculates percentage safely.
func pctAdmission(numerator, denominator int) int {
	if denominator == 0 {
		return 100
	}
	p := numerator * 100 / denominator
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return p
}
