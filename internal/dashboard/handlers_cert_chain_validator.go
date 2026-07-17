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

// CertChainValidatorResult validates TLS certificate chains from Secrets
// and Ingress hosts, checking expiry, chain completeness, and trust.
type CertChainValidatorResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         CertChainSummary `json:"summary"`
	BySecret        []CertChainEntry `json:"bySecret"`
	CriticalCerts   []CertChainEntry `json:"criticalCerts"`
	ValidationScore int              `json:"validationScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type CertChainSummary struct {
	TotalSecrets    int `json:"totalSecrets"`
	TLSSecrets      int `json:"tlsSecrets"`
	ValidChains     int `json:"validChains"`
	ExpiringSoon    int `json:"expiringSoon"`
	Expired         int `json:"expired"`
	ChainIncomplete int `json:"chainIncomplete"`
	SelfSigned      int `json:"selfSigned"`
	TotalHosts      int `json:"totalHostsCovered"`
}

type CertChainEntry struct {
	SecretName    string    `json:"secretName"`
	Namespace     string    `json:"namespace"`
	CertCN        string    `json:"certCN"`
	Issuer        string    `json:"issuer"`
	NotBefore     time.Time `json:"notBefore"`
	NotAfter      time.Time `json:"notAfter"`
	DaysRemaining int       `json:"daysRemaining"`
	IsExpired     bool      `json:"isExpired"`
	ChainComplete bool      `json:"chainComplete"`
	IsSelfSigned  bool      `json:"isSelfSigned"`
	KeySize       int       `json:"keySize"`
	Hosts         []string  `json:"coveredHosts"`
	Severity      string    `json:"severity"`
	Status        string    `json:"status"`
}

// handleCertChainValidator handles GET /api/security/cert-chain-validator
func (s *Server) handleCertChainValidator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CertChainValidatorResult{ScannedAt: time.Now()}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	// Build ingress -> secret -> hosts map
	ingressHosts := make(map[string][]string) // ns/secretName -> hosts
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	for _, ing := range ingresses.Items {
		for _, tlsEntry := range ing.Spec.TLS {
			if tlsEntry.SecretName != "" {
				key := ing.Namespace + "/" + tlsEntry.SecretName
				ingressHosts[key] = append(ingressHosts[key], tlsEntry.Hosts...)
			}
		}
	}

	for _, sec := range secrets.Items {
		if isSystemNamespace(sec.Namespace) {
			continue
		}
		if sec.Type != corev1.SecretTypeTLS {
			continue
		}

		result.Summary.TotalSecrets++
		result.Summary.TLSSecrets++

		certData, certOK := sec.Data[corev1.TLSCertKey]
		keyData, keyOK := sec.Data[corev1.TLSPrivateKeyKey]
		if !certOK || !keyOK {
			continue
		}

		entry := CertChainEntry{
			SecretName: sec.Name,
			Namespace:  sec.Namespace,
		}

		// Parse certificate
		hosts := ingressHosts[sec.Namespace+"/"+sec.Name]
		entry.Hosts = hosts
		result.Summary.TotalHosts += len(hosts)

		// Parse cert PEM
		block, rest := pem.Decode(certData)
		if block == nil {
			entry.Status = "invalid-pem"
			entry.Severity = "critical"
			result.CriticalCerts = append(result.CriticalCerts, entry)
			result.BySecret = append(result.BySecret, entry)
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			entry.Status = "parse-error"
			entry.Severity = "critical"
			result.CriticalCerts = append(result.CriticalCerts, entry)
			result.BySecret = append(result.BySecret, entry)
			continue
		}

		entry.CertCN = cert.Subject.CommonName
		entry.Issuer = cert.Issuer.CommonName
		entry.NotBefore = cert.NotBefore
		entry.NotAfter = cert.NotAfter
		entry.KeySize = cert.PublicKey.(interface{}).(interface{ Size() int }).Size() * 8
		entry.IsSelfSigned = cert.Subject.String() == cert.Issuer.String()

		// Calculate days remaining
		daysRemaining := int(time.Until(cert.NotAfter).Hours() / 24)
		entry.DaysRemaining = daysRemaining
		entry.IsExpired = daysRemaining < 0

		// Check chain completeness (simplified: check if there are multiple PEM blocks)
		remaining := rest
		hasIntermediate := false
		if nextBlock, _ := pem.Decode(remaining); nextBlock != nil {
			hasIntermediate = true
		}
		entry.ChainComplete = hasIntermediate || entry.IsSelfSigned

		// Validate key pair
		_, err = tls.X509KeyPair(certData, keyData)
		keyPairValid := err == nil

		// Determine severity
		switch {
		case entry.IsExpired:
			entry.Severity = "critical"
			entry.Status = "expired"
			result.Summary.Expired++
			result.CriticalCerts = append(result.CriticalCerts, entry)
		case daysRemaining < 7:
			entry.Severity = "critical"
			entry.Status = "expiring-soon"
			result.Summary.ExpiringSoon++
			result.CriticalCerts = append(result.CriticalCerts, entry)
		case daysRemaining < 30:
			entry.Severity = "warning"
			entry.Status = "expiring-warning"
			result.Summary.ExpiringSoon++
		case !entry.ChainComplete:
			entry.Severity = "warning"
			entry.Status = "chain-incomplete"
			result.Summary.ChainIncomplete++
		case entry.IsSelfSigned:
			entry.Severity = "info"
			entry.Status = "self-signed"
			result.Summary.SelfSigned++
		case !keyPairValid:
			entry.Severity = "critical"
			entry.Status = "key-mismatch"
			result.CriticalCerts = append(result.CriticalCerts, entry)
		default:
			entry.Severity = "none"
			entry.Status = "valid"
			result.Summary.ValidChains++
		}

		result.BySecret = append(result.BySecret, entry)
	}

	// Sort by days remaining ascending (closest to expiry first)
	sort.Slice(result.BySecret, func(i, j int) bool {
		return result.BySecret[i].DaysRemaining < result.BySecret[j].DaysRemaining
	})

	// Validation score
	if result.Summary.TLSSecrets > 0 {
		validRatio := float64(result.Summary.ValidChains) / float64(result.Summary.TLSSecrets)
		expiredPenalty := float64(result.Summary.Expired) / float64(result.Summary.TLSSecrets) * 50
		result.ValidationScore = int(validRatio*100 - expiredPenalty)
		if result.ValidationScore < 0 {
			result.ValidationScore = 0
		}
	} else {
		result.ValidationScore = 100 // No TLS secrets = nothing to worry about
	}

	switch {
	case result.ValidationScore >= 80:
		result.Grade = "A"
	case result.ValidationScore >= 60:
		result.Grade = "B"
	case result.ValidationScore >= 40:
		result.Grade = "C"
	case result.ValidationScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildCertChainRecs(&result)
	writeJSON(w, result)
}

func buildCertChainRecs(r *CertChainValidatorResult) []string {
	recs := []string{
		fmt.Sprintf("TLS 证书验证: %d 个证书, %d 有效, %d 即将过期, %d 已过期", r.Summary.TLSSecrets, r.Summary.ValidChains, r.Summary.ExpiringSoon, r.Summary.Expired),
	}
	if r.Summary.Expired > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个证书已过期, 服务可能无法访问", r.Summary.Expired))
	}
	if r.Summary.ExpiringSoon > 0 {
		recs = append(recs, fmt.Sprintf("%d 个证书在 30 天内过期", r.Summary.ExpiringSoon))
	}
	if r.Summary.ChainIncomplete > 0 {
		recs = append(recs, fmt.Sprintf("%d 个证书链不完整 (缺少中间证书), 可能导致客户端验证失败", r.Summary.ChainIncomplete))
	}
	if r.Summary.SelfSigned > 0 {
		recs = append(recs, fmt.Sprintf("%d 个自签名证书, 生产环境建议使用 CA 签发", r.Summary.SelfSigned))
	}
	if len(r.CriticalCerts) > 0 {
		top := r.CriticalCerts[0]
		hostInfo := strings.Join(top.Hosts, ", ")
		recs = append(recs, fmt.Sprintf("最高风险: %s/%s (%s, %s, 剩余 %d 天, hosts: %s)", top.Namespace, top.SecretName, top.Status, top.CertCN, top.DaysRemaining, hostInfo))
	}
	return recs
}
