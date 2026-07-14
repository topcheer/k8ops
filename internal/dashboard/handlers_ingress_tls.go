package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressTLSResult is the ingress TLS certificate & HTTPS enforcement audit.
type IngressTLSResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         IngressTLSSummary `json:"summary"`
	Ingresses       []IngressTLSEntry `json:"ingresses"`
	Gaps            []IngressTLSGap   `json:"gaps"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
}

// IngressTLSSummary aggregates ingress TLS statistics.
type IngressTLSSummary struct {
	TotalIngresses     int `json:"totalIngresses"`
	WithTLS            int `json:"withTLS"`
	WithoutTLS         int `json:"withoutTLS"`      // HTTP only
	HTTPRedirect       int `json:"httpRedirect"`    // has HTTP->HTTPS redirect
	NoRedirect         int `json:"noRedirect"`      // TLS but no redirect
	WithCertManager    int `json:"withCertManager"` // has cert-manager annotation
	WithoutCertManager int `json:"withoutCertManager"`
	TLSSecrets         int `json:"tlsSecrets"`
}

// IngressTLSEntry describes an ingress's TLS configuration.
type IngressTLSEntry struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	HasTLS            bool     `json:"hasTLS"`
	TLSHosts          []string `json:"tlsHosts,omitempty"`
	TLSSecret         string   `json:"tlsSecret,omitempty"`
	HasRedirect       bool     `json:"hasRedirect"`
	HasCertManager    bool     `json:"hasCertManager"`
	CertManagerIssuer string   `json:"certManagerIssuer,omitempty"`
	Rules             int      `json:"rules"`
	Status            string   `json:"status"`
}

// IngressTLSGap describes a TLS gap.
type IngressTLSGap struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleIngressTLS audits ingress TLS certificate & HTTPS enforcement.
// GET /api/product/ingress-tls
func (s *Server) handleIngressTLS(w http.ResponseWriter, r *http.Request) {
	result := IngressTLSResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	ingresses, err := rc.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ing := range ingresses.Items {
			if systemNamespaces[ing.Namespace] {
				continue
			}
			result.Summary.TotalIngresses++

			entry := IngressTLSEntry{
				Name:      ing.Name,
				Namespace: ing.Namespace,
				Rules:     len(ing.Spec.Rules),
				Status:    "healthy",
			}

			// Check TLS configuration
			hasTLS := len(ing.Spec.TLS) > 0
			entry.HasTLS = hasTLS
			if hasTLS {
				result.Summary.WithTLS++
				for _, tls := range ing.Spec.TLS {
					entry.TLSHosts = append(entry.TLSHosts, tls.Hosts...)
					entry.TLSSecret = tls.SecretName
					if tls.SecretName != "" {
						result.Summary.TLSSecrets++
					}
				}
			} else {
				result.Summary.WithoutTLS++
				entry.Status = "no-tls"
				result.Gaps = append(result.Gaps, IngressTLSGap{
					Namespace: ing.Namespace,
					Name:      ing.Name,
					Issue:     "Ingress has no TLS — traffic served over HTTP only",
					Severity:  "high",
				})
			}

			// Check for cert-manager annotations
			if ing.Annotations != nil {
				issuer := ""
				if v, ok := ing.Annotations["cert-manager.io/cluster-issuer"]; ok {
					issuer = v
					entry.HasCertManager = true
				} else if v, ok := ing.Annotations["cert-manager.io/issuer"]; ok {
					issuer = v
					entry.HasCertManager = true
				}
				entry.CertManagerIssuer = issuer

				if entry.HasCertManager {
					result.Summary.WithCertManager++
				} else {
					result.Summary.WithoutCertManager++
				}

				// Check for HTTP->HTTPS redirect
				redirect := ing.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"]
				if redirect == "true" || redirect == "" {
					entry.HasRedirect = true
					result.Summary.HTTPRedirect++
				} else {
					entry.HasRedirect = false
					result.Summary.NoRedirect++
				}
			} else {
				result.Summary.WithoutCertManager++
				result.Summary.NoRedirect++
			}

			// Check if TLS hosts match rule hosts
			if hasTLS {
				ruleHosts := make(map[string]bool)
				for _, rule := range ing.Spec.Rules {
					if rule.Host != "" {
						ruleHosts[rule.Host] = true
					}
				}
				tlsHosts := make(map[string]bool)
				for _, tls := range ing.Spec.TLS {
					for _, h := range tls.Hosts {
						tlsHosts[h] = true
					}
				}
				for host := range ruleHosts {
					if !tlsHosts[host] {
						entry.Status = "tls-mismatch"
						result.Gaps = append(result.Gaps, IngressTLSGap{
							Namespace: ing.Namespace,
							Name:      ing.Name,
							Issue:     fmt.Sprintf("Rule host %s not covered by TLS certificate", host),
							Severity:  "medium",
						})
					}
				}
			}

			result.Ingresses = append(result.Ingresses, entry)
		}
	}

	sort.Slice(result.Ingresses, func(i, j int) bool {
		return result.Ingresses[i].Status > result.Ingresses[j].Status
	})
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	// Recommendations
	if result.Summary.WithoutTLS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ingresses have no TLS — add TLS certificates for HTTPS", result.Summary.WithoutTLS))
	}
	if result.Summary.WithoutCertManager > 0 && result.Summary.WithCertManager == 0 {
		result.Recommendations = append(result.Recommendations,
			"No cert-manager annotations found — use cert-manager for automatic certificate management")
	}
	if result.Summary.NoRedirect > 0 && result.Summary.WithTLS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d TLS ingresses without HTTP->HTTPS redirect — enable ssl-redirect", result.Summary.NoRedirect))
	}

	// Health score
	score := 100
	score -= result.Summary.WithoutTLS * 10
	score -= result.Summary.NoRedirect * 3
	if result.Summary.WithoutCertManager > 0 && result.Summary.WithCertManager == 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}

var _ = networkingv1.IngressSpec{}
var _ = strings.Contains
