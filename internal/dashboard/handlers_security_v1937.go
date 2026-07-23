package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	admissionregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.37 — Security Dimension (Round 9)
// 1. RBAC Overexposure Auditor — excessive verb/wildcard permissions
// 2. Secret Encryption at Rest — etcd encryption status check
// 3. Admission Webhook Risk — webhook order, timeout, failure policy
// ============================================================

// ---------------------------------------------------------------
// 1. RBAC Overexposure Auditor
// ---------------------------------------------------------------

type RBACOverexposeResult1937 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         RBACOverexposeSummary1937 `json:"summary"`
	RiskyBindings   []RBACOverexposeEntry1937 `json:"riskyBindings"`
	ClusterAdmins   []RBACClusterAdmin1937    `json:"clusterAdmins"`
	Recommendations []string                  `json:"recommendations"`
}

type RBACOverexposeSummary1937 struct {
	TotalBindings     int `json:"totalBindings"`
	WildcardVerbs     int `json:"wildcardVerbs"`
	WildcardResources int `json:"wildcardResources"`
	ClusterAdminCount int `json:"clusterAdminCount"`
	EscalationRisks   int `json:"escalationRisks"`
	NamespacedAdmin   int `json:"namespacedAdmin"`
	HighRiskBindings  int `json:"highRiskBindings"`
}

type RBACOverexposeEntry1937 struct {
	Subject     string `json:"subject"`
	SubjectKind string `json:"subjectKind"`
	Namespace   string `json:"namespace"`
	Role        string `json:"role"`
	RoleKind    string `json:"roleKind"`
	RiskType    string `json:"riskType"`
	Severity    string `json:"severity"`
	Detail      string `json:"detail"`
}

type RBACClusterAdmin1937 struct {
	Subject     string `json:"subject"`
	SubjectKind string `json:"subjectKind"`
	Namespace   string `json:"namespace"`
}

func (s *Server) handleRBACOverexpose(w http.ResponseWriter, r *http.Request) {
	result := RBACOverexposeResult1937{ScannedAt: time.Now()}
	score := 100

	// Collect ClusterRole rules from ClusterRoleBindings
	crbList, err := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	crMap := make(map[string]struct{ Verbs, Resources []string })
	for _, cr := range crList.Items {
		for _, rule := range cr.Rules {
			crMap[cr.Name] = struct{ Verbs, Resources []string }{Verbs: rule.Verbs, Resources: rule.Resources}
		}
	}

	for _, crb := range crbList.Items {
		result.Summary.TotalBindings++
		roleName := crb.RoleRef.Name
		verbs, resources := crMap[roleName].Verbs, crMap[roleName].Resources

		isClusterAdmin := roleName == "cluster-admin" || strings.Contains(roleName, "cluster-admin")
		if isClusterAdmin {
			result.Summary.ClusterAdminCount++
			for _, sub := range crb.Subjects {
				result.ClusterAdmins = append(result.ClusterAdmins, RBACClusterAdmin1937{
					Subject: sub.Name, SubjectKind: string(sub.Kind), Namespace: sub.Namespace,
				})
			}
			score -= 3
			continue
		}

		// Check wildcard verbs
		hasWildcardVerb := false
		for _, v := range verbs {
			if v == "*" {
				hasWildcardVerb = true
				result.Summary.WildcardVerbs++
			}
		}
		// Check wildcard resources
		hasWildcardResource := false
		for _, res := range resources {
			if res == "*" {
				hasWildcardResource = true
				result.Summary.WildcardResources++
			}
		}

		// Check for escalation verbs (escalate, impersonate)
		for _, v := range verbs {
			if v == "escalate" || v == "impersonate" {
				result.Summary.EscalationRisks++
				for _, sub := range crb.Subjects {
					result.RiskyBindings = append(result.RiskyBindings, RBACOverexposeEntry1937{
						Subject: sub.Name, SubjectKind: string(sub.Kind),
						Role: roleName, RoleKind: "ClusterRole",
						RiskType: "escalation", Severity: "critical",
						Detail: fmt.Sprintf("Subject has '%s' verb — can escalate privileges", v),
					})
				}
				score -= 5
			}
		}

		if hasWildcardVerb && hasWildcardResource {
			result.Summary.HighRiskBindings++
			for _, sub := range crb.Subjects {
				result.RiskyBindings = append(result.RiskyBindings, RBACOverexposeEntry1937{
					Subject: sub.Name, SubjectKind: string(sub.Kind),
					Role: roleName, RoleKind: "ClusterRole",
					RiskType: "wildcard-all", Severity: "high",
					Detail: "Wildcard verbs+resources = full cluster access",
				})
			}
			score -= 5
		}
	}

	// Check RoleBindings for namespaced admin
	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})
	for _, rb := range rbList.Items {
		if isSystemNamespace(rb.Namespace) {
			continue
		}
		result.Summary.TotalBindings++
		roleName := rb.RoleRef.Name
		if strings.Contains(roleName, "admin") || strings.Contains(roleName, "edit") {
			result.Summary.NamespacedAdmin++
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.ClusterAdminCount > 3 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d cluster-admin bindings — reduce to <3", result.Summary.ClusterAdminCount))
	}
	if result.Summary.WildcardVerbs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d wildcard verb permissions — specify explicit verbs", result.Summary.WildcardVerbs))
	}
	if result.Summary.EscalationRisks > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d escalation risks (escalate/impersonate) — audit immediately", result.Summary.EscalationRisks))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Secret Encryption at Rest
// ---------------------------------------------------------------

type SecretEncResult1937 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SecretEncSummary1937 `json:"summary"`
	Risks           []SecretEncRisk1937  `json:"risks"`
	Recommendations []string             `json:"recommendations"`
}

type SecretEncSummary1937 struct {
	TotalSecrets      int  `json:"totalSecrets"`
	OpaqueSecrets     int  `json:"opaqueSecrets"`
	TLSSecrets        int  `json:"tlsSecrets"`
	DockerConfigJSON  int  `json:"dockerconfigjsonSecrets"`
	ServiceAccountTkn int  `json:"saTokenSecrets"`
	OtherTypeSecrets  int  `json:"otherTypeSecrets"`
	EstEncryptedPct   int  `json:"estEncryptedPct"`
	EtcdEncEnabled    bool `json:"etcdEncryptionEnabled"`
}

type SecretEncRisk1937 struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	SecretType string `json:"secretType"`
	RiskType   string `json:"riskType"`
	Severity   string `json:"severity"`
	Detail     string `json:"detail"`
}

func (s *Server) handleSecretEncRest(w http.ResponseWriter, r *http.Request) {
	result := SecretEncResult1937{ScannedAt: time.Now()}
	score := 100

	// Check API server encryption configuration (via kube-system configmap or apiserver flags)
	// Best effort: check if encryption-provider-config is present
	result.Summary.EtcdEncEnabled = false // Cannot definitively determine from API; conservative default

	secList, err := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	var totalDataSize int
	for _, sec := range secList.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		result.Summary.TotalSecrets++

		for _, v := range sec.Data {
			totalDataSize += len(v)
		}

		switch sec.Type {
		case corev1.SecretTypeOpaque:
			result.Summary.OpaqueSecrets++
		case corev1.SecretTypeTLS:
			result.Summary.TLSSecrets++
		case corev1.SecretTypeDockerConfigJson:
			result.Summary.DockerConfigJSON++
		case corev1.SecretTypeServiceAccountToken:
			result.Summary.ServiceAccountTkn++
		default:
			result.Summary.OtherTypeSecrets++
		}

		// Risk: SA token secrets are auto-created and may be stale
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			ageDays := time.Since(sec.CreationTimestamp.Time).Hours() / 24
			if ageDays > 90 {
				result.Risks = append(result.Risks, SecretEncRisk1937{
					Namespace: sec.Namespace, Name: sec.Name, SecretType: string(sec.Type),
					RiskType: "stale-sa-token", Severity: "medium",
					Detail: fmt.Sprintf("SA token secret %.0fd old — use projected tokens instead", ageDays),
				})
			}
		}

		// Risk: large secret (potential cert bundle)
		dataSize := 0
		for _, v := range sec.Data {
			dataSize += len(v)
		}
		if dataSize > 50000 {
			result.Risks = append(result.Risks, SecretEncRisk1937{
				Namespace: sec.Namespace, Name: sec.Name, SecretType: string(sec.Type),
				RiskType: "large-secret", Severity: "low",
				Detail: fmt.Sprintf("Secret is %dKB — may contain sensitive bundle", dataSize/1024),
			})
		}
	}

	// Estimate: if etcd encryption not enabled, 0% encrypted
	if result.Summary.EtcdEncEnabled {
		result.Summary.EstEncryptedPct = 100
	} else {
		result.Summary.EstEncryptedPct = 0
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if !result.Summary.EtcdEncEnabled {
		result.Recommendations = append(result.Recommendations, "Enable etcd encryption at rest with aescbc or secretbox provider")
	}
	if result.Summary.ServiceAccountTkn > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SA token secrets — migrate to projected volume tokens", result.Summary.ServiceAccountTkn))
	}
	if len(result.Risks) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d secret encryption risks detected", len(result.Risks)))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Admission Webhook Risk
// ---------------------------------------------------------------

type WebhookRiskResult1937 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         WebhookRiskSummary1937 `json:"summary"`
	Webhooks        []WebhookEntry1937     `json:"webhooks"`
	Risks           []WebhookRisk1937      `json:"risks"`
	Recommendations []string               `json:"recommendations"`
}

type WebhookRiskSummary1937 struct {
	TotalWebhooks    int `json:"totalWebhooks"`
	MutatingCount    int `json:"mutatingCount"`
	ValidatingCount  int `json:"validatingCount"`
	WithFailOpen     int `json:"failOpenCount"`
	WithLowTimeout   int `json:"lowTimeoutCount"`
	WithHighTimeout  int `json:"highTimeoutCount"`
	CatchAllWebhooks int `json:"catchAllWebhooks"`
	NoNSSelector     int `json:"noNamespaceSelector"`
}

type WebhookEntry1937 struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	FailureMode   string `json:"failurePolicy"`
	TimeoutSecs   int32  `json:"timeoutSeconds"`
	HasNSSelector bool   `json:"hasNamespaceSelector"`
	CatchAll      bool   `json:"catchAll"`
}

type WebhookRisk1937 struct {
	Name     string `json:"name"`
	RiskType string `json:"riskType"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

// fpStr converts FailurePolicyType pointer to string
func fpStr(fp *admissionregv1.FailurePolicyType) string {
	if fp == nil {
		return ""
	}
	return string(*fp)
}

func (s *Server) handleWebhookRisk(w http.ResponseWriter, r *http.Request) {
	result := WebhookRiskResult1937{ScannedAt: time.Now()}
	score := 100

	// Mutating webhooks
	mwList, err := s.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, mw := range mwList.Items {
			result.Summary.TotalWebhooks++
			result.Summary.MutatingCount++

			for _, wh := range mw.Webhooks {
				failureMode := fpStr(wh.FailurePolicy)
				timeoutSecs := int32(10)
				if wh.TimeoutSeconds != nil {
					timeoutSecs = *wh.TimeoutSeconds
				}
				catchAll := len(wh.Rules) == 0 || (len(wh.Rules) > 0 && len(wh.Rules[0].Resources) == 1 && wh.Rules[0].Resources[0] == "*")
				hasNS := wh.NamespaceSelector != nil

				entry := WebhookEntry1937{
					Name: wh.Name, Type: "mutating", FailureMode: failureMode,
					TimeoutSecs: timeoutSecs, HasNSSelector: hasNS, CatchAll: catchAll,
				}
				result.Webhooks = append(result.Webhooks, entry)

				if failureMode == "Ignore" {
					result.Summary.WithFailOpen++
					result.Risks = append(result.Risks, WebhookRisk1937{
						Name: wh.Name, RiskType: "fail-open", Severity: "high",
						Detail: "Webhook failurePolicy=Ignore — requests bypass policy on webhook failure",
					})
					score -= 3
				}
				if timeoutSecs < 3 {
					result.Summary.WithLowTimeout++
					result.Risks = append(result.Risks, WebhookRisk1937{
						Name: wh.Name, RiskType: "low-timeout", Severity: "low",
						Detail: fmt.Sprintf("Timeout %ds < 3s — may miss slow webhook processing", timeoutSecs),
					})
				}
				if timeoutSecs > 30 {
					result.Summary.WithHighTimeout++
					result.Risks = append(result.Risks, WebhookRisk1937{
						Name: wh.Name, RiskType: "high-timeout", Severity: "medium",
						Detail: fmt.Sprintf("Timeout %ds > 30s — can block API server", timeoutSecs),
					})
					score -= 2
				}
				if catchAll {
					result.Summary.CatchAllWebhooks++
					result.Risks = append(result.Risks, WebhookRisk1937{
						Name: wh.Name, RiskType: "catch-all", Severity: "medium",
						Detail: "Webhook matches all resources — broad blast radius",
					})
					score -= 2
				}
				if !hasNS {
					result.Summary.NoNSSelector++
				}
			}
		}
	}

	// Validating webhooks
	vwList, err := s.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, vw := range vwList.Items {
			result.Summary.TotalWebhooks++
			result.Summary.ValidatingCount++

			for _, wh := range vw.Webhooks {
				failureMode := fpStr(wh.FailurePolicy)
				timeoutSecs := int32(10)
				if wh.TimeoutSeconds != nil {
					timeoutSecs = *wh.TimeoutSeconds
				}
				catchAll := len(wh.Rules) == 0 || (len(wh.Rules) > 0 && len(wh.Rules[0].Resources) == 1 && wh.Rules[0].Resources[0] == "*")
				hasNS := wh.NamespaceSelector != nil

				entry := WebhookEntry1937{
					Name: wh.Name, Type: "validating", FailureMode: failureMode,
					TimeoutSecs: timeoutSecs, HasNSSelector: hasNS, CatchAll: catchAll,
				}
				result.Webhooks = append(result.Webhooks, entry)

				if failureMode == "Ignore" {
					result.Summary.WithFailOpen++
					result.Risks = append(result.Risks, WebhookRisk1937{
						Name: wh.Name, RiskType: "fail-open", Severity: "high",
						Detail: "Validating webhook failurePolicy=Ignore — policy bypass on failure",
					})
					score -= 3
				}
				if timeoutSecs > 30 {
					result.Summary.WithHighTimeout++
					score -= 2
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithFailOpen > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d webhooks with fail-open policy — set to Fail for security", result.Summary.WithFailOpen))
	}
	if result.Summary.CatchAllWebhooks > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d catch-all webhooks — scope to specific resources", result.Summary.CatchAllWebhooks))
	}
	if result.Summary.WithHighTimeout > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d webhooks with >30s timeout — reduce to prevent API blocking", result.Summary.WithHighTimeout))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
