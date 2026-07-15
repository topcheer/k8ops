package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WhHealthResult is the admission webhook configuration health & performance risk audit.
type WhHealthResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         WhHealthSummary `json:"summary"`
	Webhooks        []WhEntry       `json:"webhooks"`
	Risks           []WhRisk        `json:"risks"`
	Recommendations []string        `json:"recommendations"`
	HealthScore     int             `json:"healthScore"`
}

// WhHealthSummary aggregates webhook health metrics.
type WhHealthSummary struct {
	TotalWebhooks      int `json:"totalWebhooks"`
	MutatingWebhooks   int `json:"mutatingWebhooks"`
	ValidatingWebhooks int `json:"validatingWebhooks"`
	FailOpenCount      int `json:"failOpenCount"`
	FailClosedCount    int `json:"failClosedCount"`
	WithTimeout        int `json:"withTimeout"`
	NoTimeout          int `json:"noTimeout"`
	ShortTimeout       int `json:"shortTimeout"`
	LongTimeout        int `json:"longTimeout"`
	NoNamespaceSel     int `json:"noNamespaceSel"`
	MatchAllResources  int `json:"matchAllResources"`
	WithServiceRef     int `json:"withServiceRef"`
	WithURLRef         int `json:"withURLRef"`
}

// WebhookEntry describes a single webhook configuration.
type WhEntry struct {
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	WebhookName     string   `json:"webhookName"`
	FailurePolicy   string   `json:"failurePolicy"`
	TimeoutSeconds  int32    `json:"timeoutSeconds"`
	HasNamespaceSel bool     `json:"hasNamespaceSelector"`
	MatchAllRes     bool     `json:"matchAllResources"`
	ServiceNS       string   `json:"serviceNamespace,omitempty"`
	ServiceName     string   `json:"serviceName,omitempty"`
	URL             string   `json:"url,omitempty"`
	Operations      []string `json:"operations"`
	Resources       []string `json:"resources"`
	RiskLevel       string   `json:"riskLevel"`
	Issues          []string `json:"issues,omitempty"`
}

// WhRisk describes a webhook-related risk.
type WhRisk struct {
	Webhook  string `json:"webhook,omitempty"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

// handleWebhookHealth audits admission webhook configuration health & performance risk.
// GET /api/operations/webhook-health
func (s *Server) handleWebhookHealth(w http.ResponseWriter, r *http.Request) {
	result := WhHealthResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Get MutatingWebhookConfigurations
	mutating, err := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	if err == nil && mutating != nil {
		for _, mwc := range mutating.Items {
			for _, wh := range mwc.Webhooks {
				entry := buildMutatingWebhookEntry(mwc.Name, &wh)
				result.Webhooks = append(result.Webhooks, entry)
				result.Summary.MutatingWebhooks++
				result.Summary.TotalWebhooks++
				analyzeWebhook(&entry, &result)
			}
		}
	}

	// 2. Get ValidatingWebhookConfigurations
	validating, err := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	if err == nil && validating != nil {
		for _, vwc := range validating.Items {
			for _, wh := range vwc.Webhooks {
				entry := buildValidatingWebhookEntry(vwc.Name, &wh)
				result.Webhooks = append(result.Webhooks, entry)
				result.Summary.ValidatingWebhooks++
				result.Summary.TotalWebhooks++
				analyzeWebhook(&entry, &result)
			}
		}
	}

	sort.Slice(result.Webhooks, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return riskOrder[result.Webhooks[i].RiskLevel] < riskOrder[result.Webhooks[j].RiskLevel]
	})

	// Health score
	score := 100
	if result.Summary.FailOpenCount > 0 {
		score -= min(15, result.Summary.FailOpenCount*3)
	}
	if result.Summary.LongTimeout > 0 {
		score -= min(15, result.Summary.LongTimeout*5)
	}
	if result.Summary.MatchAllResources > 0 {
		score -= min(15, result.Summary.MatchAllResources*3)
	}
	if result.Summary.NoNamespaceSel > 0 {
		score -= min(10, result.Summary.NoNamespaceSel*2)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.FailOpenCount > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d webhook(s) use fail-open (Ignore) — admission control bypassed on failure", result.Summary.FailOpenCount))
	}
	if result.Summary.LongTimeout > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d webhook(s) have long timeout (>30s) — can block API server", result.Summary.LongTimeout))
	}
	if result.Summary.MatchAllResources > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d webhook(s) match all resources (*/*) — scope to specific resources", result.Summary.MatchAllResources))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Admission webhook configuration is healthy — appropriate timeouts and scope")
	}

	writeJSON(w, result)
}

func buildMutatingWebhookEntry(parentName string, wh *admissionv1.MutatingWebhook) WhEntry {
	entry := WhEntry{
		Name:        parentName,
		Kind:        "MutatingWebhookConfiguration",
		WebhookName: wh.Name,
		RiskLevel:   "low",
	}

	fp := "Fail"
	if wh.FailurePolicy != nil {
		fp = string(*wh.FailurePolicy)
	}
	entry.FailurePolicy = fp

	if wh.TimeoutSeconds != nil {
		entry.TimeoutSeconds = *wh.TimeoutSeconds
	}

	entry.HasNamespaceSel = wh.NamespaceSelector != nil && len(wh.NamespaceSelector.MatchLabels) > 0

	if wh.ClientConfig.Service != nil {
		entry.ServiceNS = wh.ClientConfig.Service.Namespace
		entry.ServiceName = wh.ClientConfig.Service.Name
	}
	if wh.ClientConfig.URL != nil {
		entry.URL = *wh.ClientConfig.URL
	}

	for _, rule := range wh.Rules {
		for _, op := range rule.Operations {
			entry.Operations = append(entry.Operations, string(op))
		}
		entry.Resources = append(entry.Resources, rule.Resources...)
		if isMatchAll(rule.APIGroups, rule.Resources) {
			entry.MatchAllRes = true
		}
	}

	return entry
}

func buildValidatingWebhookEntry(parentName string, wh *admissionv1.ValidatingWebhook) WhEntry {
	entry := WhEntry{
		Name:        parentName,
		Kind:        "ValidatingWebhookConfiguration",
		WebhookName: wh.Name,
		RiskLevel:   "low",
	}

	fp := "Fail"
	if wh.FailurePolicy != nil {
		fp = string(*wh.FailurePolicy)
	}
	entry.FailurePolicy = fp

	if wh.TimeoutSeconds != nil {
		entry.TimeoutSeconds = *wh.TimeoutSeconds
	}

	entry.HasNamespaceSel = wh.NamespaceSelector != nil && len(wh.NamespaceSelector.MatchLabels) > 0

	if wh.ClientConfig.Service != nil {
		entry.ServiceNS = wh.ClientConfig.Service.Namespace
		entry.ServiceName = wh.ClientConfig.Service.Name
	}
	if wh.ClientConfig.URL != nil {
		entry.URL = *wh.ClientConfig.URL
	}

	for _, rule := range wh.Rules {
		for _, op := range rule.Operations {
			entry.Operations = append(entry.Operations, string(op))
		}
		entry.Resources = append(entry.Resources, rule.Resources...)
		if isMatchAll(rule.APIGroups, rule.Resources) {
			entry.MatchAllRes = true
		}
	}

	return entry
}

func analyzeWebhook(entry *WhEntry, result *WhHealthResult) {
	// Failure policy
	if entry.FailurePolicy == "Ignore" {
		result.Summary.FailOpenCount++
		entry.Issues = append(entry.Issues, "fail-open (Ignore) — admission bypassed on failure")
		entry.RiskLevel = "medium"
	} else {
		result.Summary.FailClosedCount++
	}

	// Timeout
	if entry.TimeoutSeconds == 0 {
		result.Summary.NoTimeout++
		entry.Issues = append(entry.Issues, "no timeout configured (defaults to 10s)")
	} else if entry.TimeoutSeconds > 30 {
		result.Summary.LongTimeout++
		entry.Issues = append(entry.Issues, fmt.Sprintf("long timeout (%ds) — can block API server", entry.TimeoutSeconds))
		entry.RiskLevel = "high"
	} else if entry.TimeoutSeconds < 3 {
		result.Summary.ShortTimeout++
	} else {
		result.Summary.WithTimeout++
	}

	// Namespace selector
	if !entry.HasNamespaceSel {
		result.Summary.NoNamespaceSel++
		entry.Issues = append(entry.Issues, "no namespace selector — matches all namespaces")
		if entry.RiskLevel == "low" {
			entry.RiskLevel = "medium"
		}
	}

	// Match all resources
	if entry.MatchAllRes {
		result.Summary.MatchAllResources++
		entry.Issues = append(entry.Issues, "matches all resources (*/*) — broad scope")
		if entry.RiskLevel == "low" {
			entry.RiskLevel = "medium"
		}
	}

	// Service vs URL
	if entry.ServiceName != "" {
		result.Summary.WithServiceRef++
	}
	if entry.URL != "" {
		result.Summary.WithURLRef++
		result.Risks = append(result.Risks, WhRisk{
			Webhook:  entry.WebhookName,
			Issue:    fmt.Sprintf("Webhook %s uses URL instead of in-cluster service — no mTLS, no HA", entry.WebhookName),
			Severity: "warning",
		})
	}

	// Add risks for high/critical
	if entry.RiskLevel == "high" {
		result.Risks = append(result.Risks, WhRisk{
			Webhook:  entry.WebhookName,
			Issue:    fmt.Sprintf("Webhook %s has high risk: %s", entry.WebhookName, strings.Join(entry.Issues, "; ")),
			Severity: "high",
		})
	}
}

func isMatchAll(apiGroups []string, resources []string) bool {
	for _, g := range apiGroups {
		if g == "*" {
			for _, r := range resources {
				if r == "*" {
					return true
				}
			}
		}
	}
	return false
}
