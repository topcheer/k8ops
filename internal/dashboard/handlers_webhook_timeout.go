package dashboard

import (
	"fmt"
	"net/http"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type WebhookAuditResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         WebhookTimeoutSummary `json:"summary"`
	Webhooks        []WebhookTimeoutEntry `json:"webhooks"`
	RiskyWebhooks   []WebhookTimeoutEntry `json:"riskyWebhooks"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type WebhookTimeoutSummary struct {
	TotalWebhooks  int `json:"totalWebhooks"`
	Validating     int `json:"validatingWebhooks"`
	Mutating       int `json:"mutatingWebhooks"`
	Timeout30s     int `json:"timeout30sOrMore"`
	FailOpen       int `json:"failOpenWebhooks"`
	NoNamespaceSel int `json:"noNamespaceSelector"`
	CatchAll       int `json:"catchAllWebhooks"`
}

type WebhookTimeoutEntry struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Service        string   `json:"service"`
	TimeoutSeconds int32    `json:"timeoutSeconds"`
	FailOpen       bool     `json:"failurePolicyFailOpen"`
	HasNSSelector  bool     `json:"hasNamespaceSelector"`
	IsCatchAll     bool     `json:"isCatchAll"`
	RiskLevel      string   `json:"riskLevel"`
	Issues         []string `json:"issues"`
}

func (s *Server) handleWebhookTimeoutAudit(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	ctx := r.Context()
	result := WebhookAuditResult{ScannedAt: time.Now()}

	vwhs, _ := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	for _, vwh := range vwhs.Items {
		if isSystemWebhook1876(vwh.Name) {
			continue
		}
		for _, wh := range vwh.Webhooks {
			result.Summary.TotalWebhooks++
			result.Summary.Validating++
			entry := whEntryFromVWH(vwh.Name, wh)
			result.Webhooks = append(result.Webhooks, entry)
			if entry.RiskLevel != "low" {
				result.RiskyWebhooks = append(result.RiskyWebhooks, entry)
			}
			whAgg(&result.Summary, entry)
		}
	}

	mwhs, _ := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	for _, mwh := range mwhs.Items {
		if isSystemWebhook1876(mwh.Name) {
			continue
		}
		for _, wh := range mwh.Webhooks {
			result.Summary.TotalWebhooks++
			result.Summary.Mutating++
			entry := whEntryFromMWH(mwh.Name, wh)
			result.Webhooks = append(result.Webhooks, entry)
			if entry.RiskLevel != "low" {
				result.RiskyWebhooks = append(result.RiskyWebhooks, entry)
			}
			whAgg(&result.Summary, entry)
		}
	}

	if result.Summary.TotalWebhooks > 0 {
		result.HealthScore = (result.Summary.TotalWebhooks - result.Summary.FailOpen - result.Summary.Timeout30s) * 100 / result.Summary.TotalWebhooks
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Webhook 超时审计: %d webhook (%d 校验, %d 变更), %d 超时, %d fail-open, %d 全捕获",
			result.Summary.TotalWebhooks, result.Summary.Validating, result.Summary.Mutating,
			result.Summary.Timeout30s, result.Summary.FailOpen, result.Summary.CatchAll),
	}
	if result.Summary.FailOpen > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 webhook fail-open", result.Summary.FailOpen))
	}
	writeJSON(w, result)
}

func whEntryFromVWH(name string, wh admissionregistrationv1.ValidatingWebhook) WebhookTimeoutEntry {
	e := WebhookTimeoutEntry{Name: name, Type: "Validating"}
	if wh.TimeoutSeconds != nil {
		e.TimeoutSeconds = *wh.TimeoutSeconds
	}
	e.FailOpen = wh.FailurePolicy != nil && *wh.FailurePolicy == admissionregistrationv1.Ignore
	e.HasNSSelector = wh.NamespaceSelector != nil
	e.IsCatchAll = len(wh.Rules) == 0
	if wh.ClientConfig.Service != nil {
		e.Service = wh.ClientConfig.Service.Name + "." + wh.ClientConfig.Service.Namespace
	}
	var issues []string
	if e.TimeoutSeconds >= 30 {
		issues = append(issues, fmt.Sprintf("timeout %ds", e.TimeoutSeconds))
	}
	if e.FailOpen {
		issues = append(issues, "fail-open")
	}
	if e.IsCatchAll {
		issues = append(issues, "catch-all")
	}
	e.Issues = issues
	switch {
	case len(issues) >= 2:
		e.RiskLevel = "high"
	case len(issues) >= 1:
		e.RiskLevel = "medium"
	default:
		e.RiskLevel = "low"
	}
	return e
}

func whEntryFromMWH(name string, wh admissionregistrationv1.MutatingWebhook) WebhookTimeoutEntry {
	e := WebhookTimeoutEntry{Name: name, Type: "Mutating"}
	if wh.TimeoutSeconds != nil {
		e.TimeoutSeconds = *wh.TimeoutSeconds
	}
	e.FailOpen = wh.FailurePolicy != nil && *wh.FailurePolicy == admissionregistrationv1.Ignore
	e.HasNSSelector = wh.NamespaceSelector != nil
	e.IsCatchAll = len(wh.Rules) == 0
	if wh.ClientConfig.Service != nil {
		e.Service = wh.ClientConfig.Service.Name + "." + wh.ClientConfig.Service.Namespace
	}
	var issues []string
	if e.TimeoutSeconds >= 30 {
		issues = append(issues, fmt.Sprintf("timeout %ds", e.TimeoutSeconds))
	}
	if e.FailOpen {
		issues = append(issues, "fail-open")
	}
	if e.IsCatchAll {
		issues = append(issues, "catch-all")
	}
	e.Issues = issues
	switch {
	case len(issues) >= 2:
		e.RiskLevel = "high"
	case len(issues) >= 1:
		e.RiskLevel = "medium"
	default:
		e.RiskLevel = "low"
	}
	return e
}

func whAgg(s *WebhookTimeoutSummary, e WebhookTimeoutEntry) {
	if e.TimeoutSeconds >= 30 {
		s.Timeout30s++
	}
	if e.FailOpen {
		s.FailOpen++
	}
	if !e.HasNSSelector {
		s.NoNamespaceSel++
	}
	if e.IsCatchAll {
		s.CatchAll++
	}
}

func isSystemWebhook1876(name string) bool {
	for _, p := range []string{"cert-manager", "kube-system", "k8s.io"} {
		if containsStr1876(name, p) {
			return true
		}
	}
	return false
}
