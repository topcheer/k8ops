package dashboard

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CertExpiryLevel categorizes certificate urgency.
type CertExpiryLevel string

const (
	CertLevelCritical CertExpiryLevel = "critical" // <7 days
	CertLevelWarning  CertExpiryLevel = "warning"  // 7-30 days
	CertLevelOK       CertExpiryLevel = "ok"       // >30 days
	CertLevelExpired  CertExpiryLevel = "expired"  // already expired
	CertLevelError    CertExpiryLevel = "error"    // parse failure
)

// CertInfo holds certificate expiration details.
type CertInfo struct {
	Name           string          `json:"name"`
	Namespace      string          `json:"namespace"`
	Kind           string          `json:"kind"` // "Secret" or "Ingress"
	SecretType     string          `json:"secretType,omitempty"`
	Issuer         string          `json:"issuer,omitempty"`
	Subject        string          `json:"subject,omitempty"`
	DNSNames       []string        `json:"dnsNames,omitempty"`
	NotBefore      time.Time       `json:"notBefore"`
	NotAfter       time.Time       `json:"notAfter"`
	DaysUntilExpiry int            `json:"daysUntilExpiry"`
	Level          CertExpiryLevel `json:"level"`
	UsedByIngress  []string        `json:"usedByIngress,omitempty"` // ingress names referencing this cert
	Error          string          `json:"error,omitempty"`
}

// CertExpirySummary aggregates cert scan results.
type CertExpirySummary struct {
	Total     int                       `json:"total"`
	Expired   int                       `json:"expired"`
	Critical  int                       `json:"critical"`
	Warning   int                       `json:"warning"`
	OK        int                       `json:"ok"`
	Errors    int                       `json:"errors"`
	Certificates []CertInfo             `json:"certificates"`
	ScannedAt  time.Time                `json:"scannedAt"`
}

// certExpiryThresholds
const (
	certCriticalDays = 7
	certWarningDays  = 30
)

// handleCertExpiryScan scans all TLS secrets and ingress certificates.
// GET /api/certificates/expiry?namespace=xxx
func (s *Server) handleCertExpiryScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)

	ns := r.URL.Query().Get("namespace")

	// Collect ingress → secret references
	ingressSecrets := make(map[string][]string) // "ns/secretname" → []ingress names
	ingressList, err := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ing := range ingressList.Items {
			for _, tls := range ing.Spec.TLS {
				if tls.SecretName != "" {
					key := fmt.Sprintf("%s/%s", ing.Namespace, tls.SecretName)
					ingressSecrets[key] = append(ingressSecrets[key], ing.Name)
				}
			}
		}
	}

	// Scan all secrets and filter in-code (field selectors are not reliably
	// supported by all clientset implementations, e.g. fake clientset in tests)
	allSecrets, err := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var certs []CertInfo

	for _, sec := range allSecrets.Items {
		// Only scan TLS and Opaque secrets that have tls.crt data
		if sec.Type != corev1.SecretTypeTLS && sec.Type != corev1.SecretTypeOpaque {
			continue
		}
		certData, ok := sec.Data["tls.crt"]
		if !ok || len(certData) == 0 {
			continue
		}
		info := parseCertFromPEM(certData)
		info.Name = sec.Name
		info.Namespace = sec.Namespace
		info.Kind = "Secret"
		info.SecretType = string(sec.Type)

		key := fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)
		if refs, ok := ingressSecrets[key]; ok {
			info.UsedByIngress = refs
		}
		certs = append(certs, info)
	}

	// Sort: expired first, then critical, warning, ok, error
	sort.Slice(certs, func(i, j int) bool {
		return certLevelPriority(certs[i].Level) < certLevelPriority(certs[j].Level)
	})

	// Build summary
	summary := CertExpirySummary{
		Total:        len(certs),
		Certificates: certs,
		ScannedAt:    time.Now(),
	}
	for _, c := range certs {
		switch c.Level {
		case CertLevelExpired:
			summary.Expired++
		case CertLevelCritical:
			summary.Critical++
		case CertLevelWarning:
			summary.Warning++
		case CertLevelOK:
			summary.OK++
		case CertLevelError:
			summary.Errors++
		}
	}

	writeJSON(w, summary)
}

// parseCertFromPEM decodes PEM certificate data and extracts key info.
func parseCertFromPEM(data []byte) CertInfo {
	info := CertInfo{}

	// Try to decode as PEM
	block, _ := pem.Decode(data)
	if block == nil {
		// Try raw DER
		cert, err := x509.ParseCertificate(data)
		if err != nil {
			info.Level = CertLevelError
			info.Error = fmt.Sprintf("failed to parse certificate: %v", err)
			return info
		}
		fillCertInfo(&info, cert)
		return info
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		info.Level = CertLevelError
		info.Error = fmt.Sprintf("failed to parse certificate: %v", err)
		return info
	}

	fillCertInfo(&info, cert)
	return info
}

// fillCertInfo populates CertInfo from a parsed x509 certificate.
func fillCertInfo(info *CertInfo, cert *x509.Certificate) {
	info.NotBefore = cert.NotBefore
	info.NotAfter = cert.NotAfter
	info.Issuer = cert.Issuer.CommonName
	if info.Issuer == "" {
		info.Issuer = strings.Join(cert.Issuer.Organization, ", ")
	}
	info.Subject = cert.Subject.CommonName
	if info.Subject == "" {
		info.Subject = strings.Join(cert.Subject.Organization, ", ")
	}
	info.DNSNames = cert.DNSNames

	// Calculate days until expiry
	now := time.Now()
	days := int(cert.NotAfter.Sub(now).Hours() / 24)
	info.DaysUntilExpiry = days

	switch {
	case days < 0:
		info.Level = CertLevelExpired
	case days < certCriticalDays:
		info.Level = CertLevelCritical
	case days < certWarningDays:
		info.Level = CertLevelWarning
	default:
		info.Level = CertLevelOK
	}
}

// certLevelPriority returns sort priority (lower = more urgent).
func certLevelPriority(level CertExpiryLevel) int {
	switch level {
	case CertLevelExpired:
		return 0
	case CertLevelCritical:
		return 1
	case CertLevelWarning:
		return 2
	case CertLevelOK:
		return 3
	case CertLevelError:
		return 4
	default:
		return 5
	}
}
