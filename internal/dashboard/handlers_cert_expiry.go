package dashboard

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CertExpiryResult monitors TLS certificate lifecycle:
// expiry dates, CA chain health, renewal readiness, and expiring cert alerts.
type CertExpiryResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         TLSCertSummary `json:"summary"`
	ExpiringCerts   []CertDetail   `json:"expiringCerts"`
	AllCerts        []CertDetail   `json:"allCerts"`
	HealthScore     int            `json:"healthScore"`
	Grade           string         `json:"grade"`
	Recommendations []string       `json:"recommendations"`
}

type TLSCertSummary struct {
	TotalCerts     int `json:"totalCerts"`
	Expiring30Days int `json:"expiring30Days"`
	Expiring90Days int `json:"expiring90Days"`
	Expired        int `json:"expired"`
	SelfSigned     int `json:"selfSigned"`
	ValidCerts     int `json:"validCerts"`
}

type CertDetail struct {
	Name       string    `json:"name"`
	Namespace  string    `json:"namespace"`
	SecretName string    `json:"secretName"`
	CommonName string    `json:"commonName"`
	Issuer     string    `json:"issuer"`
	Expiry     time.Time `json:"expiry"`
	DaysLeft   int       `json:"daysLeft"`
	Status     string    `json:"status"`
	SelfSigned bool      `json:"selfSigned"`
}

// handleCertExpiry monitors TLS certificate expiry lifecycle.
// GET /api/operations/cert-expiry
func (s *Server) handleCertExpiry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CertExpiryResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	now := time.Now()
	thirtyDays := now.AddDate(0, 0, 30)
	ninetyDays := now.AddDate(0, 0, 90)

	for _, sec := range secrets.Items {
		if systemNS[sec.Namespace] {
			continue
		}
		if sec.Type != corev1.SecretTypeTLS {
			continue
		}

		certData, ok := sec.Data[corev1.TLSCertKey]
		if !ok {
			continue
		}

		block, _ := pem.Decode(certData)
		if block == nil {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}

		result.Summary.TotalCerts++
		daysLeft := int(cert.NotAfter.Sub(now).Hours() / 24)
		selfSigned := false
		if cert.Subject.String() == cert.Issuer.String() {
			selfSigned = true
			result.Summary.SelfSigned++
		}

		status := "valid"
		if cert.NotAfter.Before(now) {
			status = "expired"
			result.Summary.Expired++
		} else if cert.NotAfter.Before(thirtyDays) {
			status = "expiring-30d"
			result.Summary.Expiring30Days++
		} else if cert.NotAfter.Before(ninetyDays) {
			status = "expiring-90d"
			result.Summary.Expiring90Days++
		} else {
			result.Summary.ValidCerts++
		}

		commonName := cert.Subject.CommonName
		if commonName == "" {
			commonName = cert.Subject.String()
		}

		detail := CertDetail{
			Name:       sec.Name,
			Namespace:  sec.Namespace,
			SecretName: sec.Name,
			CommonName: commonName,
			Issuer:     cert.Issuer.CommonName,
			Expiry:     cert.NotAfter,
			DaysLeft:   daysLeft,
			Status:     status,
			SelfSigned: selfSigned,
		}

		result.AllCerts = append(result.AllCerts, detail)
		if status != "valid" {
			result.ExpiringCerts = append(result.ExpiringCerts, detail)
		}
	}

	// Score
	validRatio := 1.0
	if result.Summary.TotalCerts > 0 {
		validRatio = float64(result.Summary.ValidCerts) / float64(result.Summary.TotalCerts)
	}
	penalty := result.Summary.Expired*30 + result.Summary.Expiring30Days*10 + result.Summary.Expiring90Days*3
	score := int(validRatio*100) - penalty
	if score < 0 {
		score = 0
	}
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	// Sort by days left (ascending = most urgent first)
	sort.Slice(result.ExpiringCerts, func(i, j int) bool {
		return result.ExpiringCerts[i].DaysLeft < result.ExpiringCerts[j].DaysLeft
	})
	sort.Slice(result.AllCerts, func(i, j int) bool {
		return result.AllCerts[i].DaysLeft < result.AllCerts[j].DaysLeft
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Certificate health: %d/100 (grade %s) — %d total TLS certs", result.HealthScore, result.Grade, result.Summary.TotalCerts))
	if result.Summary.Expired > 0 {
		recs = append(recs, fmt.Sprintf("%d certificates already EXPIRED — renew immediately", result.Summary.Expired))
	}
	if result.Summary.Expiring30Days > 0 {
		recs = append(recs, fmt.Sprintf("%d certificates expiring within 30 days — schedule renewal now", result.Summary.Expiring30Days))
	}
	if result.Summary.Expiring90Days > 0 {
		recs = append(recs, fmt.Sprintf("%d certificates expiring within 90 days — plan renewal", result.Summary.Expiring90Days))
	}
	if result.Summary.SelfSigned > 0 {
		recs = append(recs, fmt.Sprintf("%d self-signed certificates — consider using cert-manager with Let's Encrypt or internal CA", result.Summary.SelfSigned))
	}
	if len(recs) == 1 {
		recs = append(recs, "All certificates are valid and within renewal window — maintain cert-manager automation")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
