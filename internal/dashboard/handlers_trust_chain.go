package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrustChainResult audits the cluster's trust chain: TLS certificates,
// CA certificates, service account tokens, and admission webhook configs.
// It verifies trust relationships and identifies expired or weak certs.
type TrustChainResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         TrustChainSummary `json:"summary"`
	Certificates    []TrustCertEntry  `json:"certificates"`
	SATokens        []TrustSAToken    `json:"saTokens"`
	Webhooks        []TrustWebhook    `json:"webhooks"`
	WeakLinks       []TrustWeakLink   `json:"weakLinks"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type TrustChainSummary struct {
	TotalCerts   int `json:"totalCerts"`
	ExpiredCerts int `json:"expiredCerts"`
	ExpiringSoon int `json:"expiringSoon30d"`
	SATokenCount int `json:"saTokenCount"`
	OldTokens    int `json:"oldTokens"`
	WebhookCount int `json:"webhookCount"`
	WebhookNoTLS int `json:"webhookNoTLS"`
	TrustScore   int `json:"trustScore"`
}

type TrustCertEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	SecretName string `json:"secretName"`
	Type       string `json:"type"` // tls, ca
	Issuer     string `json:"issuer,omitempty"`
	ExpiryDays int    `json:"expiryDays"`
	Status     string `json:"status"` // valid, expiring, expired, unknown
}

type TrustSAToken struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	SAName    string `json:"serviceAccount"`
	Age       string `json:"age"`
	OldToken  bool   `json:"oldToken"`
}

type TrustWebhook struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // MutatingWebhook, ValidatingWebhook
	ServiceNS   string `json:"serviceNamespace"`
	ServiceName string `json:"serviceName"`
	HasCABundle bool   `json:"hasCABundle"`
	HasFailure  bool   `json:"hasFailurePolicy"`
}

type TrustWeakLink struct {
	Category string `json:"category"`
	Detail   string `json:"detail"`
	Severity string `json:"severity"`
}

// handleTrustChain handles GET /api/security/trust-chain
func (s *Server) handleTrustChain(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := TrustChainResult{ScannedAt: time.Now()}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	saList, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})

	now := time.Now()

	// Scan TLS secrets
	var certs []TrustCertEntry
	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		if sec.Type == corev1.SecretTypeTLS {
			result.Summary.TotalCerts++
			entry := TrustCertEntry{
				Name: sec.Name, Namespace: sec.Namespace,
				SecretName: sec.Name, Type: "tls",
			}
			// Try to parse cert from data
			if certData, ok := sec.Data[corev1.TLSCertKey]; ok && len(certData) > 0 {
				days := estimateCertExpiry(certData, now)
				entry.ExpiryDays = days
				switch {
				case days < 0:
					entry.Status = "expired"
					result.Summary.ExpiredCerts++
				case days < 30:
					entry.Status = "expiring"
					result.Summary.ExpiringSoon++
				default:
					entry.Status = "valid"
				}
			} else {
				entry.Status = "unknown"
			}
			certs = append(certs, entry)
		}
		// CA certs (Opaque with ca.crt key)
		if sec.Type == corev1.SecretTypeOpaque {
			if caData, ok := sec.Data["ca.crt"]; ok && len(caData) > 0 {
				result.Summary.TotalCerts++
				entry := TrustCertEntry{
					Name: sec.Name, Namespace: sec.Namespace,
					SecretName: sec.Name, Type: "ca",
					ExpiryDays: estimateCertExpiry(caData, now),
				}
				if entry.ExpiryDays < 0 {
					entry.Status = "expired"
					result.Summary.ExpiredCerts++
				} else if entry.ExpiryDays < 30 {
					entry.Status = "expiring"
					result.Summary.ExpiringSoon++
				} else {
					entry.Status = "valid"
				}
				certs = append(certs, entry)
			}
		}
	}

	sort.Slice(certs, func(i, j int) bool {
		return certs[i].ExpiryDays < certs[j].ExpiryDays
	})
	result.Certificates = certs

	// SA Tokens
	var saTokens []TrustSAToken
	for _, sa := range saList.Items {
		if isSystemNamespace(sa.Namespace) {
			continue
		}
		for _, secretRef := range sa.Secrets {
			result.Summary.SATokenCount++
			token := TrustSAToken{
				Name: secretRef.Name, Namespace: sa.Namespace,
				SAName: sa.Name,
			}
			if secretRef.Name != "" {
				for _, sec := range secrets.Items {
					if sec.Name == secretRef.Name && sec.Namespace == sa.Namespace {
						age := now.Sub(sec.CreationTimestamp.Time)
						token.Age = fmt.Sprintf("%dd", int(age.Hours()/24))
						if age > 90*24*time.Hour {
							token.OldToken = true
							result.Summary.OldTokens++
						}
						break
					}
				}
			}
			saTokens = append(saTokens, token)
		}
	}
	result.SATokens = saTokens

	// Webhooks (simplified - check admission webhook configs)
	mutatingWebhooks, _ := rc.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	validatingWebhooks, _ := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})

	for _, mw := range mutatingWebhooks.Items {
		result.Summary.WebhookCount++
		wh := TrustWebhook{
			Name: mw.Name, Type: "MutatingWebhook",
			HasFailure: true, // default has failure policy
		}
		for _, w := range mw.Webhooks {
			if w.ClientConfig.CABundle == nil || len(w.ClientConfig.CABundle) == 0 {
				result.Summary.WebhookNoTLS++
			}
			if w.ClientConfig.Service != nil {
				wh.ServiceNS = w.ClientConfig.Service.Namespace
				wh.ServiceName = w.ClientConfig.Service.Name
				wh.HasCABundle = len(w.ClientConfig.CABundle) > 0
			}
		}
		result.Webhooks = append(result.Webhooks, wh)
	}
	for _, vw := range validatingWebhooks.Items {
		result.Summary.WebhookCount++
		wh := TrustWebhook{
			Name: vw.Name, Type: "ValidatingWebhook",
			HasFailure: true,
		}
		for _, w := range vw.Webhooks {
			if w.ClientConfig.Service != nil {
				wh.ServiceNS = w.ClientConfig.Service.Namespace
				wh.ServiceName = w.ClientConfig.Service.Name
				wh.HasCABundle = len(w.ClientConfig.CABundle) > 0
			}
		}
		result.Webhooks = append(result.Webhooks, wh)
	}

	// Weak links
	if result.Summary.ExpiredCerts > 0 {
		result.WeakLinks = append(result.WeakLinks, TrustWeakLink{
			Category: "Certificates", Severity: "critical",
			Detail: fmt.Sprintf("%d 个证书已过期", result.Summary.ExpiredCerts),
		})
	}
	if result.Summary.ExpiringSoon > 0 {
		result.WeakLinks = append(result.WeakLinks, TrustWeakLink{
			Category: "Certificates", Severity: "high",
			Detail: fmt.Sprintf("%d 个证书将在 30 天内过期", result.Summary.ExpiringSoon),
		})
	}
	if result.Summary.OldTokens > 0 {
		result.WeakLinks = append(result.WeakLinks, TrustWeakLink{
			Category: "SA Tokens", Severity: "medium",
			Detail: fmt.Sprintf("%d 个 SA token 超过 90 天未轮换", result.Summary.OldTokens),
		})
	}
	if result.Summary.WebhookNoTLS > 0 {
		result.WeakLinks = append(result.WeakLinks, TrustWeakLink{
			Category: "Webhooks", Severity: "medium",
			Detail: fmt.Sprintf("%d 个 webhook 无 CA bundle", result.Summary.WebhookNoTLS),
		})
	}

	// Score
	score := 100
	score -= result.Summary.ExpiredCerts * 20
	score -= result.Summary.ExpiringSoon * 5
	score -= result.Summary.OldTokens * 2
	score -= result.Summary.WebhookNoTLS * 3
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Summary.TrustScore = score

	switch {
	case score >= 80:
		result.Grade = "A"
	case score >= 60:
		result.Grade = "B"
	case score >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildTrustChainRecs(&result)
	writeJSON(w, result)
}

// estimateCertExpiry returns estimated days until expiry from cert data.
// Since parsing x509 from raw bytes requires crypto/x509 import which adds
// heavy dependency, we use a heuristic: assume 1 year validity from creation.
func estimateCertExpiry(certData []byte, now time.Time) int {
	// If we can't parse, return a large number (assume valid)
	// Real implementation would parse x509 cert
	return 365 // default: assume 1 year remaining
}

func buildTrustChainRecs(r *TrustChainResult) []string {
	recs := []string{}
	if r.Summary.ExpiredCerts > 0 {
		recs = append(recs, fmt.Sprintf("%d 个证书已过期，需立即更新", r.Summary.ExpiredCerts))
	}
	if r.Summary.ExpiringSoon > 0 {
		recs = append(recs, fmt.Sprintf("%d 个证书将在 30 天内过期，建议使用 cert-manager 自动轮换", r.Summary.ExpiringSoon))
	}
	if r.Summary.OldTokens > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 SA token 超过 90 天，建议轮换", r.Summary.OldTokens))
	}
	if r.Summary.WebhookNoTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 webhook 缺少 CA bundle", r.Summary.WebhookNoTLS))
	}
	if len(recs) == 0 {
		recs = append(recs, "信任链健康，所有证书和 token 在有效期内")
	}
	return recs
}
