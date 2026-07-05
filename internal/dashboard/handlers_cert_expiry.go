package dashboard

import (
	"crypto/tls"
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

// CertExpResult is the certificate & TLS expiry analysis.
type CertExpResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         CertExpSummary  `json:"summary"`
	ExpiringSoon    []CertExpEntry  `json:"expiringSoon"`
	Expired         []CertExpEntry  `json:"expired"`
	AllCerts        []CertExpEntry  `json:"allCerts"`
	ByNamespace     []CertExpNSStat `json:"byNamespace"`
	Issues          []CertExpIssue  `json:"issues"`
	Recommendations []string        `json:"recommendations"`
}

// CertExpSummary aggregates certificate statistics.
type CertExpSummary struct {
	TotalCerts  int `json:"totalCerts"`
	TLSSecrets  int `json:"tlsSecrets"`
	Expired     int `json:"expired"`
	Expiring30d int `json:"expiring30d"`
	Expiring60d int `json:"expiring60d"`
	Expiring90d int `json:"expiring90d"`
	Healthy     int `json:"healthy"`
	FromCA      int `json:"fromCA"`
	SelfSigned  int `json:"selfSigned"`
	HealthScore int `json:"healthScore"`
}

// CertExpEntry describes one TLS certificate.
type CertExpEntry struct {
	Name          string    `json:"name"`
	Namespace     string    `json:"namespace"`
	CN            string    `json:"cn"`
	SANs          []string  `json:"sans"`
	Issuer        string    `json:"issuer"`
	NotBefore     time.Time `json:"notBefore"`
	NotAfter      time.Time `json:"notAfter"`
	DaysRemaining int       `json:"daysRemaining"`
	IsExpired     bool      `json:"isExpired"`
	IsSelfSigned  bool      `json:"isSelfSigned"`
	RiskLevel     string    `json:"riskLevel"`
	KeySize       int       `json:"keySize"`
	IsReferenced  bool      `json:"isReferenced"`
}

// CertExpNSStat per-namespace cert stats.
type CertExpNSStat struct {
	Namespace string `json:"namespace"`
	CertCount int    `json:"certCount"`
	Expiring  int    `json:"expiring"`
	Expired   int    `json:"expired"`
}

// CertExpIssue is a detected certificate problem.
type CertExpIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleCertExpiry monitors TLS certificate expiry across the cluster.
// GET /api/security/cert-expiry
func (s *Server) handleCertExpiry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	secrets, err := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build secret reference map
	secretRefs := make(map[string]bool)
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				secretRefs[fmt.Sprintf("%s/%s", pod.Namespace, vol.Secret.SecretName)] = true
			}
		}
	}

	result := CertExpResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*CertExpNSStat)
	now := time.Now()

	for _, sec := range secrets.Items {
		if sec.Type != corev1.SecretTypeTLS {
			continue
		}

		certData, ok := sec.Data[corev1.TLSCertKey]
		if !ok || len(certData) == 0 {
			continue
		}

		entry, perr := certExpParse(string(certData), sec.Name, sec.Namespace)
		if perr != nil {
			result.Issues = append(result.Issues, CertExpIssue{
				Severity: "warning", Type: "parse-error",
				Resource: fmt.Sprintf("%s/%s", sec.Namespace, sec.Name),
				Message:  fmt.Sprintf("Failed to parse TLS certificate: %v", perr),
			})
			continue
		}

		entry.IsReferenced = secretRefs[fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)]
		entry.DaysRemaining = int(entry.NotAfter.Sub(now).Hours() / 24)
		entry.IsExpired = now.After(entry.NotAfter)
		entry.RiskLevel = certExpRisk(entry.DaysRemaining)

		result.Summary.TotalCerts++
		result.Summary.TLSSecrets++

		if entry.IsSelfSigned {
			result.Summary.SelfSigned++
		} else {
			result.Summary.FromCA++
		}

		nsStat := certExpGetOrCreateNS(nsMap, sec.Namespace)
		nsStat.CertCount++

		if entry.IsExpired {
			result.Summary.Expired++
			nsStat.Expired++
			result.Expired = append(result.Expired, entry)
			result.Issues = append(result.Issues, CertExpIssue{
				Severity: "critical", Type: "expired",
				Resource: fmt.Sprintf("%s/%s", sec.Namespace, sec.Name),
				Message:  fmt.Sprintf("Certificate %s/%s EXPIRED %d days ago (CN: %s)", sec.Namespace, sec.Name, -entry.DaysRemaining, entry.CN),
			})
		} else if entry.DaysRemaining <= 30 {
			result.Summary.Expiring30d++
			nsStat.Expiring++
			result.ExpiringSoon = append(result.ExpiringSoon, entry)
			result.Issues = append(result.Issues, CertExpIssue{
				Severity: "critical", Type: "expiring-soon",
				Resource: fmt.Sprintf("%s/%s", sec.Namespace, sec.Name),
				Message:  fmt.Sprintf("Certificate %s/%s expires in %d days (CN: %s)", sec.Namespace, sec.Name, entry.DaysRemaining, entry.CN),
			})
		} else if entry.DaysRemaining <= 60 {
			result.Summary.Expiring60d++
			nsStat.Expiring++
			result.ExpiringSoon = append(result.ExpiringSoon, entry)
			result.Issues = append(result.Issues, CertExpIssue{
				Severity: "warning", Type: "expiring-soon",
				Resource: fmt.Sprintf("%s/%s", sec.Namespace, sec.Name),
				Message:  fmt.Sprintf("Certificate %s/%s expires in %d days (CN: %s)", sec.Namespace, sec.Name, entry.DaysRemaining, entry.CN),
			})
		} else if entry.DaysRemaining <= 90 {
			result.Summary.Expiring90d++
			result.ExpiringSoon = append(result.ExpiringSoon, entry)
			result.Issues = append(result.Issues, CertExpIssue{
				Severity: "info", Type: "expiring-soon",
				Resource: fmt.Sprintf("%s/%s", sec.Namespace, sec.Name),
				Message:  fmt.Sprintf("Certificate %s/%s expires in %d days (CN: %s)", sec.Namespace, sec.Name, entry.DaysRemaining, entry.CN),
			})
		} else {
			result.Summary.Healthy++
		}

		result.AllCerts = append(result.AllCerts, entry)
	}

	sort.Slice(result.Expired, func(i, j int) bool {
		return result.Expired[i].NotAfter.Before(result.Expired[j].NotAfter)
	})
	sort.Slice(result.ExpiringSoon, func(i, j int) bool {
		return result.ExpiringSoon[i].DaysRemaining < result.ExpiringSoon[j].DaysRemaining
	})
	sort.Slice(result.AllCerts, func(i, j int) bool {
		return certExpRank(result.AllCerts[i].RiskLevel) < certExpRank(result.AllCerts[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return certExpIssueRank(result.Issues[i].Severity) < certExpIssueRank(result.Issues[j].Severity)
	})

	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		if result.ByNamespace[i].Expired != result.ByNamespace[j].Expired {
			return result.ByNamespace[i].Expired > result.ByNamespace[j].Expired
		}
		return result.ByNamespace[i].Expiring > result.ByNamespace[j].Expiring
	})

	result.Summary.HealthScore = certExpScore(result.Summary)
	result.Recommendations = certExpRecs(result.Summary, result.Expired, result.ExpiringSoon)

	writeJSON(w, result)
}

// certExpParse parses a PEM-encoded certificate.
func certExpParse(pemData, name, namespace string) (CertExpEntry, error) {
	entry := CertExpEntry{Name: name, Namespace: namespace}

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return entry, fmt.Errorf("no PEM block found")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return entry, fmt.Errorf("failed to parse certificate: %w", err)
	}

	entry.CN = cert.Subject.CommonName
	entry.Issuer = cert.Issuer.CommonName
	entry.NotBefore = cert.NotBefore
	entry.NotAfter = cert.NotAfter
	entry.IsSelfSigned = cert.Subject.String() == cert.Issuer.String()

	for _, dns := range cert.DNSNames {
		entry.SANs = append(entry.SANs, dns)
	}
	for _, ip := range cert.IPAddresses {
		entry.SANs = append(entry.SANs, ip.String())
	}

	switch key := cert.PublicKey.(type) {
	case interface{ Size() int }:
		entry.KeySize = key.Size() * 8
	}

	return entry, nil
}

func certExpRisk(daysRemaining int) string {
	if daysRemaining < 0 {
		return "critical"
	}
	if daysRemaining <= 30 {
		return "critical"
	}
	if daysRemaining <= 60 {
		return "high"
	}
	if daysRemaining <= 90 {
		return "medium"
	}
	return "low"
}

func certExpScore(s CertExpSummary) int {
	if s.TotalCerts == 0 {
		return 100
	}
	score := 100
	score -= s.Expired * 30
	score -= s.Expiring30d * 15
	score -= s.Expiring60d * 8
	score -= s.Expiring90d * 4
	score -= s.SelfSigned * 2
	if score < 0 {
		score = 0
	}
	return score
}

func certExpRecs(s CertExpSummary, expired []CertExpEntry, expiring []CertExpEntry) []string {
	var recs []string

	if s.Expired > 0 {
		names := make([]string, 0, len(expired))
		for _, e := range expired {
			names = append(names, fmt.Sprintf("%s/%s", e.Namespace, e.Name))
			if len(names) >= 3 {
				break
			}
		}
		recs = append(recs, fmt.Sprintf("%d certificate(s) ALREADY EXPIRED: %s — renew immediately to avoid service disruption", s.Expired, strings.Join(names, ", ")))
	}
	if s.Expiring30d > 0 {
		recs = append(recs, fmt.Sprintf("%d certificate(s) expire within 30 days — renew now via cert-manager or manual rotation", s.Expiring30d))
	}
	if s.Expiring60d > 0 {
		recs = append(recs, fmt.Sprintf("%d certificate(s) expire within 60 days — schedule renewal within next 2 weeks", s.Expiring60d))
	}
	if s.Expiring90d > 0 {
		recs = append(recs, fmt.Sprintf("%d certificate(s) expire within 90 days — plan renewal timeline", s.Expiring90d))
	}
	if s.SelfSigned > 0 {
		recs = append(recs, fmt.Sprintf("%d self-signed certificate(s) detected — consider using a proper CA or cert-manager with Let's Encrypt", s.SelfSigned))
	}
	if s.TotalCerts > 0 && s.Expired == 0 && s.Expiring30d == 0 {
		recs = append(recs, "All certificates are healthy — consider implementing automated renewal with cert-manager")
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Certificate health score is %d/100 — multiple certificates need immediate attention", s.HealthScore))
	}

	return recs
}

func certExpGetOrCreateNS(m map[string]*CertExpNSStat, ns string) *CertExpNSStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &CertExpNSStat{Namespace: ns}
	m[ns] = e
	return e
}

func certExpRank(level string) int {
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

func certExpIssueRank(s string) int {
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

var _ = tls.Config{}
