package dashboard

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CertTransparencyResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         CertTransSummary `json:"summary"`
	Certificates    []CertTransEntry `json:"certificates"`
	ExpiringCerts   []CertTransEntry `json:"expiringCerts"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type CertTransSummary struct {
	TotalCerts    int `json:"totalCerts"`
	ValidCerts    int `json:"validCerts"`
	Expiring30d   int `json:"expiring30Days"`
	Expiring7d    int `json:"expiring7Days"`
	ExpiredCerts  int `json:"expiredCerts"`
	SelfSigned    int `json:"selfSigned"`
	WildcardCerts int `json:"wildcardCerts"`
}

type CertTransEntry struct {
	Host         string    `json:"host"`
	Namespace    string    `json:"namespace"`
	Source       string    `json:"source"`
	Issuer       string    `json:"issuer"`
	Subject      string    `json:"subject"`
	NotBefore    time.Time `json:"notBefore"`
	NotAfter     time.Time `json:"notAfter"`
	DaysLeft     int       `json:"daysLeft"`
	IsSelfSigned bool      `json:"isSelfSigned"`
	IsWildcard   bool      `json:"isWildcard"`
	RiskLevel    string    `json:"riskLevel"`
}

func (s *Server) handleCertTransparencyMonitor(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	ctx := r.Context()
	result := CertTransparencyResult{ScannedAt: time.Now()}

	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	hostSet := make(map[string][]string)
	for _, ing := range ingresses.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		for _, tlsCfg := range ing.Spec.TLS {
			for _, host := range tlsCfg.Hosts {
				hostSet[host] = append(hostSet[host], ing.Namespace)
			}
		}
	}
	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) || svc.Spec.Type != "LoadBalancer" {
			continue
		}
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.Hostname != "" {
				hostSet[ing.Hostname] = append(hostSet[ing.Hostname], svc.Namespace)
			}
		}
	}

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	for host, nss := range hostSet {
		result.Summary.TotalCerts++
		entry := CertTransEntry{Host: host, Namespace: nss[0], Source: "ingress"}

		conn, err := tls.DialWithDialer(dialer, "tcp", host+":443", &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			entry.RiskLevel = "high"
			entry.Issuer = "unreachable"
			result.Summary.ExpiredCerts++
			result.ExpiringCerts = append(result.ExpiringCerts, entry)
			result.Certificates = append(result.Certificates, entry)
			continue
		}

		certs := conn.ConnectionState().PeerCertificates
		conn.Close()
		if len(certs) == 0 {
			continue
		}

		cert := certs[0]
		entry.Issuer = cert.Issuer.CommonName
		entry.Subject = cert.Subject.CommonName
		entry.NotBefore = cert.NotBefore
		entry.NotAfter = cert.NotAfter
		entry.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
		entry.IsSelfSigned = cert.Issuer.CommonName == cert.Subject.CommonName
		entry.IsWildcard = strings.Contains(cert.Subject.CommonName, "*")

		if entry.IsSelfSigned {
			result.Summary.SelfSigned++
		}
		if entry.IsWildcard {
			result.Summary.WildcardCerts++
		}

		switch {
		case entry.DaysLeft < 0:
			entry.RiskLevel = "critical"
			result.Summary.ExpiredCerts++
			result.ExpiringCerts = append(result.ExpiringCerts, entry)
		case entry.DaysLeft < 7:
			entry.RiskLevel = "critical"
			result.Summary.Expiring7d++
			result.Summary.Expiring30d++
			result.ExpiringCerts = append(result.ExpiringCerts, entry)
		case entry.DaysLeft < 30:
			entry.RiskLevel = "high"
			result.Summary.Expiring30d++
			result.ExpiringCerts = append(result.ExpiringCerts, entry)
		case entry.DaysLeft < 90:
			entry.RiskLevel = "medium"
			result.Summary.ValidCerts++
		default:
			entry.RiskLevel = "low"
			result.Summary.ValidCerts++
		}
		result.Certificates = append(result.Certificates, entry)
	}

	sort.Slice(result.Certificates, func(i, j int) bool {
		return result.Certificates[i].DaysLeft < result.Certificates[j].DaysLeft
	})

	if result.Summary.TotalCerts > 0 {
		result.HealthScore = result.Summary.ValidCerts * 100 / result.Summary.TotalCerts
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("证书透明度: %d 证书, %d 有效, %d 30天内过期, %d 7天内, %d 已过期, %d 自签名",
			result.Summary.TotalCerts, result.Summary.ValidCerts,
			result.Summary.Expiring30d, result.Summary.Expiring7d,
			result.Summary.ExpiredCerts, result.Summary.SelfSigned),
	}
	if result.Summary.Expiring7d > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个证书 7 天内过期, 紧急续期", result.Summary.Expiring7d))
	}
	writeJSON(w, result)
}
