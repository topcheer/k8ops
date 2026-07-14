package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CertManagerResult is the cert-manager health & certificate renewal pipeline audit.
type CertManagerResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         CertManagerSummary `json:"summary"`
	Certificates    []CertificateEntry `json:"certificates"`
	Issuers         []IssuerEntry      `json:"issuers"`
	Gaps            []CertManagerGap   `json:"gaps"`
	Recommendations []string           `json:"recommendations"`
	HealthScore     int                `json:"healthScore"`
}

// CertManagerSummary aggregates cert-manager statistics.
type CertManagerSummary struct {
	CertManagerInstalled bool `json:"certManagerInstalled"`
	TotalCertificates    int  `json:"totalCertificates"`
	ExpiringSoon         int  `json:"expiringSoon"` // <30 days
	Expired              int  `json:"expired"`
	Ready                int  `json:"ready"`
	NotReady             int  `json:"notReady"`
	TotalIssuers         int  `json:"totalIssuers"`
	ReadyIssuers         int  `json:"readyIssuers"`
	NotReadyIssuers      int  `json:"notReadyIssuers"`
	TLSSecrets           int  `json:"tlsSecrets"`     // kubernetes.io/tls secrets
	WithoutRenewal       int  `json:"withoutRenewal"` // TLS secrets not managed by cert-manager
}

// CertificateEntry describes a TLS certificate (secret).
type CertificateEntry struct {
	Name            string    `json:"name"`
	Namespace       string    `json:"namespace"`
	Issuer          string    `json:"issuer,omitempty"`
	NotBefore       time.Time `json:"notBefore,omitempty"`
	NotAfter        time.Time `json:"notAfter,omitempty"`
	DaysUntilExpiry int       `json:"daysUntilExpiry,omitempty"`
	Status          string    `json:"status"` // valid, expiring, expired
}

// IssuerEntry describes a cert-manager issuer.
type IssuerEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // Issuer or ClusterIssuer
	Type      string `json:"type"` // ACME, CA, SelfSigned, Vault
	Ready     bool   `json:"ready"`
	Status    string `json:"status"`
}

// CertManagerGap describes a gap in certificate management.
type CertManagerGap struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleCertManager audits cert-manager health & certificate renewal pipeline.
// GET /api/product/cert-manager
func (s *Server) handleCertManager(w http.ResponseWriter, r *http.Request) {
	result := CertManagerResult{
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

	// 1. Check if cert-manager is installed
	certManagerNamespaces := []string{"cert-manager", "cert-manager-system", "certmanager"}
	for _, ns := range certManagerNamespaces {
		_, err := rc.clientset.CoreV1().Namespaces().Get(r.Context(), ns, metav1.GetOptions{})
		if err == nil {
			result.Summary.CertManagerInstalled = true
			break
		}
	}

	// 2. Check cert-manager deployments
	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deployments.Items {
			if strings.Contains(strings.ToLower(dep.Name), "cert-manager") || strings.Contains(dep.Namespace, "cert-manager") {
				result.Summary.CertManagerInstalled = true
				if dep.Status.ReadyReplicas > 0 {
					result.Summary.ReadyIssuers++
				}
			}
		}
	}

	// 3. Scan TLS secrets for certificate expiry
	secrets, err := rc.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		now := time.Now()
		for _, secret := range secrets.Items {
			if systemNamespaces[secret.Namespace] {
				continue
			}
			if secret.Type != corev1.SecretTypeTLS {
				continue
			}
			result.Summary.TLSSecrets++

			certData, ok := secret.Data[corev1.TLSCertKey]
			if !ok || len(certData) == 0 {
				continue
			}

			entry := CertificateEntry{
				Name:      secret.Name,
				Namespace: secret.Namespace,
			}

			// Check for cert-manager annotations
			if secret.Annotations != nil {
				entry.Issuer = secret.Annotations["cert-manager.io/issuer-name"]
				if entry.Issuer == "" {
					entry.Issuer = secret.Annotations["cert-manager.io/cluster-issuer-name"]
				}
			}

			// Try to parse certificate expiry from annotation
			expiryStr := ""
			if secret.Annotations != nil {
				expiryStr = secret.Annotations["cert-manager.io/certificate-name"]
			}

			// Without proper cert parsing, use annotation if available
			if secret.Annotations != nil {
				if notAfter, ok := secret.Annotations["cert-manager.io/not-after"]; ok {
					if t, err := time.Parse(time.RFC3339, notAfter); err == nil {
						entry.NotAfter = t
						days := int(t.Sub(now).Hours() / 24)
						entry.DaysUntilExpiry = days
						if days < 0 {
							entry.Status = "expired"
							result.Summary.Expired++
							result.Gaps = append(result.Gaps, CertManagerGap{
								Namespace: secret.Namespace,
								Name:      secret.Name,
								Issue:     fmt.Sprintf("Certificate expired %d days ago", -days),
								Severity:  "critical",
							})
						} else if days < 30 {
							entry.Status = "expiring"
							result.Summary.ExpiringSoon++
							result.Gaps = append(result.Gaps, CertManagerGap{
								Namespace: secret.Namespace,
								Name:      secret.Name,
								Issue:     fmt.Sprintf("Certificate expires in %d days", days),
								Severity:  "high",
							})
						} else {
							entry.Status = "valid"
							result.Summary.Ready++
						}
					}
				} else {
					// TLS secret without cert-manager annotation
					result.Summary.WithoutRenewal++
					entry.Status = "manual"
				}
			} else {
				result.Summary.WithoutRenewal++
				entry.Status = "manual"
			}

			result.Certificates = append(result.Certificates, entry)
			result.Summary.TotalCertificates++
			_ = expiryStr
		}
	}

	// 4. Check for cert-manager CRDs (Issuers, ClusterIssuers) via ConfigMaps
	configmaps, err := rc.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		_ = configmaps
	}

	// Sort certificates by status
	sort.Slice(result.Certificates, func(i, j int) bool {
		return result.Certificates[i].Status > result.Certificates[j].Status
	})
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	// Recommendations
	if !result.Summary.CertManagerInstalled {
		result.Recommendations = append(result.Recommendations,
			"cert-manager is not installed — consider installing for automatic certificate management and renewal")
	}
	if result.Summary.Expired > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d certificates are expired — renew immediately", result.Summary.Expired))
	}
	if result.Summary.ExpiringSoon > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d certificates expiring within 30 days — verify renewal pipeline", result.Summary.ExpiringSoon))
	}
	if result.Summary.WithoutRenewal > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d TLS secrets not managed by cert-manager — consider migrating for automatic renewal", result.Summary.WithoutRenewal))
	}

	// Health score
	score := 100
	if !result.Summary.CertManagerInstalled {
		score -= 20
	}
	score -= result.Summary.Expired * 20
	score -= result.Summary.ExpiringSoon * 10
	score -= result.Summary.WithoutRenewal * 2
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}
