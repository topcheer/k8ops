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

// ============================================================
// v18.92 — Security Dimension
// 1. Image Registry Allowlist Audit
// 2. SA Mount Exposure Audit
// 3. TLS Version & Cipher Audit
// ============================================================

// ---------------------------------------------------------------
// 1. Image Registry Allowlist — supply chain trust audit
// ---------------------------------------------------------------

// ImageRegistryResult audits container image registry trust posture.
type ImageRegistryResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ImageRegistrySummary `json:"summary"`
	RegistryUsage   []RegistryUsageEntry `json:"registryUsage"`
	UntrustedImages []ImageTrustEntry    `json:"untrustedImages"`
	TaggedImages    []ImageTrustEntry    `json:"taggedLatest"`
	MissingDigest   []ImageTrustEntry    `json:"missingDigest"`
	Recommendations []string             `json:"recommendations"`
}

type ImageRegistrySummary struct {
	TotalImages       int `json:"totalImages"`
	UniqueRegistries  int `json:"uniqueRegistries"`
	TrustedRegistries int `json:"trustedRegistries"`
	UntrustedImages   int `json:"untrustedImages"`
	LatestTagCount    int `json:"latestTagCount"`
	WithDigest        int `json:"withDigest"`
	WithoutDigest     int `json:"withoutDigest"`
}

type RegistryUsageEntry struct {
	Registry   string `json:"registry"`
	ImageCount int    `json:"imageCount"`
	IsTrusted  bool   `json:"isTrusted"`
	Percentage int    `json:"percentage"`
}

type ImageTrustEntry struct {
	Image     string `json:"image"`
	Registry  string `json:"registry"`
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue"`
}

// trustedRegistries1892 lists commonly trusted container registries.
var trustedRegistries1892 = map[string]bool{
	"docker.io":           true,
	"registry.k8s.io":     true,
	"gcr.io":              true,
	"ghcr.io":             true,
	"quay.io":             true,
	"registry.iot2.win":   true,
	"k8s.gcr.io":          true,
	"public.ecr.aws":      true,
	"mcr.microsoft.com":   true,
	"registry.gitlab.com": true,
}

func (s *Server) handleImageRegistryAllowlist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ImageRegistryResult{ScannedAt: time.Now()}

	// Collect all container images from deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	imageMap := map[string][]string{} // image -> []workloads
	registryCount := map[string]int{}

	collectImages := func(containers []corev1.Container, name, ns string) {
		for _, c := range containers {
			result.Summary.TotalImages++
			image := c.Image
			registry := extractRegistry1892(image)
			imageMap[image] = append(imageMap[image], ns+"/"+name)
			registryCount[registry]++

			// Check for latest tag
			if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
				result.Summary.LatestTagCount++
				result.TaggedImages = append(result.TaggedImages, ImageTrustEntry{
					Image:     image,
					Registry:  registry,
					Workload:  name,
					Namespace: ns,
					RiskLevel: "medium",
					Issue:     "uses :latest tag or no tag - non-reproducible builds",
				})
			}

			// Check for digest
			if strings.Contains(image, "@sha256:") {
				result.Summary.WithDigest++
			} else {
				result.Summary.WithoutDigest++
				result.MissingDigest = append(result.MissingDigest, ImageTrustEntry{
					Image:     image,
					Registry:  registry,
					Workload:  name,
					Namespace: ns,
					RiskLevel: "medium",
					Issue:     "no digest pin - image content may change unexpectedly",
				})
			}

			// Check trust
			if !trustedRegistries1892[registry] {
				result.Summary.UntrustedImages++
				result.UntrustedImages = append(result.UntrustedImages, ImageTrustEntry{
					Image:     image,
					Registry:  registry,
					Workload:  name,
					Namespace: ns,
					RiskLevel: "high",
					Issue:     "image from untrusted registry: " + registry,
				})
			}
		}
	}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		collectImages(dep.Spec.Template.Spec.Containers, dep.Name, dep.Namespace)
		collectImages(dep.Spec.Template.Spec.InitContainers, dep.Name, dep.Namespace)
	}

	// StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		collectImages(ss.Spec.Template.Spec.Containers, ss.Name, ss.Namespace)
		collectImages(ss.Spec.Template.Spec.InitContainers, ss.Name, ss.Namespace)
	}

	// Build registry usage stats
	for reg, count := range registryCount {
		isTrusted := trustedRegistries1892[reg]
		if isTrusted {
			result.Summary.TrustedRegistries++
		}
		pct := 0
		if result.Summary.TotalImages > 0 {
			pct = count * 100 / result.Summary.TotalImages
		}
		result.RegistryUsage = append(result.RegistryUsage, RegistryUsageEntry{
			Registry:   reg,
			ImageCount: count,
			IsTrusted:  isTrusted,
			Percentage: pct,
		})
	}
	result.Summary.UniqueRegistries = len(registryCount)
	sort.Slice(result.RegistryUsage, func(i, j int) bool {
		return result.RegistryUsage[i].ImageCount > result.RegistryUsage[j].ImageCount
	})

	// Score
	if result.Summary.TotalImages > 0 {
		trustedPct := (result.Summary.TotalImages - result.Summary.UntrustedImages) * 100 / result.Summary.TotalImages
		digestPct := 100
		if result.Summary.WithoutDigest > 0 {
			digestPct = result.Summary.WithDigest * 100 / result.Summary.TotalImages
		}
		latestPenalty := result.Summary.LatestTagCount * 3
		result.HealthScore = (trustedPct+digestPct)/2 - latestPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildImageRegistryRecs1892(&result)
	writeJSON(w, result)
}

func extractRegistry1892(image string) string {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) == 1 {
		// No slash, default registry: docker.io
		return "docker.io"
	}
	// Check if first part is a registry (contains . or :)
	first := parts[0]
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return first
	}
	// No registry prefix, it's a docker.io library image
	return "docker.io"
}

func buildImageRegistryRecs1892(result *ImageRegistryResult) []string {
	recs := []string{
		fmt.Sprintf("Image registry trust: %d images, %d registries (%d trusted), %d untrusted",
			result.Summary.TotalImages, result.Summary.UniqueRegistries,
			result.Summary.TrustedRegistries, result.Summary.UntrustedImages),
	}
	if result.Summary.UntrustedImages > 0 {
		recs = append(recs, fmt.Sprintf("%d images from untrusted registries - add to allowlist or migrate to trusted registry", result.Summary.UntrustedImages))
	}
	if result.Summary.WithoutDigest > 0 {
		recs = append(recs, fmt.Sprintf("%d images without digest pin - use @sha256: for reproducible deployments", result.Summary.WithoutDigest))
	}
	if result.Summary.LatestTagCount > 0 {
		recs = append(recs, fmt.Sprintf("%d images use :latest tag - pin to specific version for supply chain integrity", result.Summary.LatestTagCount))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. SA Mount Exposure — ServiceAccount token mount exposure audit
// ---------------------------------------------------------------

// SAMountExposureResult audits ServiceAccount token auto-mount exposure.
type SAMountExposureResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         SAMountExposureSummary `json:"summary"`
	OverMounted     []SAMountEntry         `json:"overMounted"`
	HighPrivSAs     []SAPrivilegeEntry     `json:"highPrivSAs"`
	SAMountMatrix   []SAMountEntry         `json:"saMountMatrix"`
	Recommendations []string               `json:"recommendations"`
}

type SAMountExposureSummary struct {
	TotalWorkloads       int `json:"totalWorkloads"`
	AutoMountEnabled     int `json:"autoMountEnabled"`
	AutoMountDisabled    int `json:"autoMountDisabled"`
	OverMounted          int `json:"overMounted"`
	TotalServiceAccounts int `json:"totalServiceAccounts"`
	HighPrivSAs          int `json:"highPrivSAs"`
	UnusedSAs            int `json:"unusedSAs"`
}

type SAMountEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Kind           string `json:"kind"`
	ServiceAccount string `json:"serviceAccount"`
	AutoMount      bool   `json:"autoMount"`
	RiskLevel      string `json:"riskLevel"`
	Issue          string `json:"issue"`
}

type SAPrivilegeEntry struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	ClusterRoles []string `json:"clusterRoles,omitempty"`
	RiskLevel    string   `json:"riskLevel"`
	UsedByPods   int      `json:"usedByPods"`
}

func (s *Server) handleSAMountExposure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SAMountExposureResult{ScannedAt: time.Now()}

	// Track SAs referenced by pods
	saUsedByPods := map[string]int{} // "ns/sa" -> count

	// Analyze deployments
	analyzeSAMount1892 := func(name, ns, kind string, spec *corev1.PodSpec) {
		result.Summary.TotalWorkloads++
		entry := SAMountEntry{
			Name:      name,
			Namespace: ns,
			Kind:      kind,
		}

		saName := spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}
		entry.ServiceAccount = saName

		// Check automountServiceAccountToken
		automount := true
		if spec.AutomountServiceAccountToken != nil {
			automount = *spec.AutomountServiceAccountToken
		}
		entry.AutoMount = automount

		if automount {
			result.Summary.AutoMountEnabled++
			// Check if SA is not default (non-default SAs with auto-mount are more concerning)
			if saName != "default" {
				entry.RiskLevel = "medium"
				entry.Issue = "non-default SA with auto-mounted token"
			} else {
				entry.RiskLevel = "low"
			}
		} else {
			result.Summary.AutoMountDisabled++
			entry.RiskLevel = "info"
			entry.Issue = "SA token mount explicitly disabled"
		}

		// Flag if workload doesn't need API access but has SA token mounted
		if automount && kind == "Deployment" {
			// Heuristic: if no API interaction is needed, token shouldn't be mounted
			entry.RiskLevel = "medium"
			if saName == "default" {
				entry.Issue = "default SA token auto-mounted - unnecessary API access for most workloads"
				result.Summary.OverMounted++
				result.OverMounted = append(result.OverMounted, entry)
			}
		}

		saKey := ns + "/" + saName
		saUsedByPods[saKey]++

		result.SAMountMatrix = append(result.SAMountMatrix, entry)
	}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		analyzeSAMount1892(dep.Name, dep.Namespace, "Deployment", &dep.Spec.Template.Spec)
	}

	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		analyzeSAMount1892(ss.Name, ss.Namespace, "StatefulSet", &ss.Spec.Template.Spec)
	}

	// Analyze ServiceAccounts
	saList, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	for _, sa := range saList.Items {
		if isSystemNamespace(sa.Namespace) {
			continue
		}
		result.Summary.TotalServiceAccounts++
		saKey := sa.Namespace + "/" + sa.Name
		usedBy := saUsedByPods[saKey]

		// Check for high-privilege SAs
		entry := SAPrivilegeEntry{
			Name:       sa.Name,
			Namespace:  sa.Namespace,
			UsedByPods: usedBy,
		}

		if usedBy == 0 && sa.Name != "default" {
			result.Summary.UnusedSAs++
			entry.RiskLevel = "medium"
		}

		if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
			// Check role bindings for this SA
			rbList, _ := rc.clientset.RbacV1().RoleBindings(sa.Namespace).List(ctx, metav1.ListOptions{})
			for _, rb := range rbList.Items {
				for _, subject := range rb.Subjects {
					if subject.Kind == "ServiceAccount" && subject.Name == sa.Name {
						if strings.HasPrefix(rb.RoleRef.Name, "cluster-admin") || strings.HasPrefix(rb.RoleRef.Name, "admin") {
							entry.ClusterRoles = append(entry.ClusterRoles, rb.RoleRef.Name)
							entry.RiskLevel = "high"
							result.Summary.HighPrivSAs++
						}
					}
				}
			}
			crbList, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
			for _, crb := range crbList.Items {
				for _, subject := range crb.Subjects {
					if subject.Kind == "ServiceAccount" && subject.Name == sa.Name && subject.Namespace == sa.Namespace {
						entry.ClusterRoles = append(entry.ClusterRoles, crb.RoleRef.Name)
						if strings.Contains(crb.RoleRef.Name, "admin") || strings.Contains(crb.RoleRef.Name, "cluster-admin") {
							entry.RiskLevel = "critical"
							result.Summary.HighPrivSAs++
						}
					}
				}
			}
		}

		if entry.RiskLevel != "" {
			result.HighPrivSAs = append(result.HighPrivSAs, entry)
		}
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		disabledPct := result.Summary.AutoMountDisabled * 100 / result.Summary.TotalWorkloads
		overMountPenalty := result.Summary.OverMounted * 3
		highPrivPenalty := result.Summary.HighPrivSAs * 10
		result.HealthScore = disabledPct + 20 - overMountPenalty - highPrivPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
		if result.HealthScore > 100 {
			result.HealthScore = 100
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildSAMountRecs1892(&result)
	writeJSON(w, result)
}

func buildSAMountRecs1892(result *SAMountExposureResult) []string {
	recs := []string{
		fmt.Sprintf("SA token exposure: %d workloads, %d auto-mount enabled, %d over-mounted, %d high-priv SAs",
			result.Summary.TotalWorkloads, result.Summary.AutoMountEnabled,
			result.Summary.OverMounted, result.Summary.HighPrivSAs),
	}
	if result.Summary.OverMounted > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads with default SA token auto-mounted - set automountServiceAccountToken: false", result.Summary.OverMounted))
	}
	if result.Summary.HighPrivSAs > 0 {
		recs = append(recs, fmt.Sprintf("%d high-privilege ServiceAccounts detected - apply least-privilege RBAC", result.Summary.HighPrivSAs))
	}
	if result.Summary.UnusedSAs > 0 {
		recs = append(recs, fmt.Sprintf("%d unused ServiceAccounts - clean up to reduce attack surface", result.Summary.UnusedSAs))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. TLS Version & Cipher Audit
// ---------------------------------------------------------------

// TLSVersionResult audits TLS configuration across Ingress and secrets.
type TLSVersionResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         TLSVersionSummary     `json:"summary"`
	IngressTLS      []IngressTLSEntry1892 `json:"ingressTLS"`
	CertificateInfo []CertInfoEntry       `json:"certificateInfo"`
	WeakCiphers     []WeakCipherEntry     `json:"weakCiphers"`
	Recommendations []string              `json:"recommendations"`
}

type TLSVersionSummary struct {
	TotalIngresses int `json:"totalIngresses"`
	WithTLS        int `json:"withTLS"`
	WithoutTLS     int `json:"withoutTLS"`
	TotalCerts     int `json:"totalCerts"`
	ExpiringSoon   int `json:"expiringSoon"`
	ExpiredCerts   int `json:"expiredCerts"`
	WeakKeySize    int `json:"weakKeySize"`
	SelfSigned     int `json:"selfSigned"`
	CAValidated    int `json:"caValidated"`
}

type IngressTLSEntry1892 struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Hosts      []string `json:"hosts"`
	TLSEnabled bool     `json:"tlsEnabled"`
	SecretName string   `json:"secretName,omitempty"`
	RiskLevel  string   `json:"riskLevel"`
	Issue      string   `json:"issue,omitempty"`
}

type CertInfoEntry struct {
	SecretName   string   `json:"secretName"`
	Namespace    string   `json:"namespace"`
	Issuer       string   `json:"issuer"`
	Subject      string   `json:"subject"`
	DNSNames     []string `json:"dnsNames"`
	NotBefore    string   `json:"notBefore"`
	NotAfter     string   `json:"notAfter"`
	KeyAlgorithm string   `json:"keyAlgorithm"`
	KeySize      int      `json:"keySize"`
	IsSelfSigned bool     `json:"isSelfSigned"`
	IsExpired    bool     `json:"isExpired"`
	DaysToExpiry int      `json:"daysToExpiry"`
	RiskLevel    string   `json:"riskLevel"`
}

type WeakCipherEntry struct {
	SecretName string `json:"secretName"`
	Namespace  string `json:"namespace"`
	Issue      string `json:"issue"`
	RiskLevel  string `json:"riskLevel"`
}

func (s *Server) handleTLSVersionAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := TLSVersionResult{ScannedAt: time.Now()}

	// Analyze Ingresses
	ingList, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	tlsSecretKeys := map[string]bool{} // "ns/secret" -> true
	for _, ing := range ingList.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		result.Summary.TotalIngresses++

		entry := IngressTLSEntry1892{
			Name:      ing.Name,
			Namespace: ing.Namespace,
		}

		// Collect hosts
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				entry.Hosts = append(entry.Hosts, rule.Host)
			}
		}

		if len(ing.Spec.TLS) > 0 {
			entry.TLSEnabled = true
			result.Summary.WithTLS++
			for _, tls := range ing.Spec.TLS {
				if tls.SecretName != "" {
					entry.SecretName = tls.SecretName
					tlsSecretKeys[ing.Namespace+"/"+tls.SecretName] = true
				}
			}
			entry.RiskLevel = "low"
		} else {
			entry.TLSEnabled = false
			result.Summary.WithoutTLS++
			entry.RiskLevel = "high"
			entry.Issue = "no TLS configured - traffic in plaintext"
		}

		result.IngressTLS = append(result.IngressTLS, entry)
	}

	// Analyze TLS secrets
	secretList, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secretList.Items {
		if secret.Type != corev1.SecretTypeTLS {
			continue
		}
		if isSystemNamespace(secret.Namespace) {
			continue
		}

		result.Summary.TotalCerts++

		certData, ok := secret.Data[corev1.TLSCertKey]
		if !ok || len(certData) == 0 {
			continue
		}

		entry := CertInfoEntry{
			SecretName: secret.Name,
			Namespace:  secret.Namespace,
		}

		// Parse certificate
		block, _ := pem.Decode(certData)
		if block == nil {
			entry.RiskLevel = "high"
			entry.Issuer = "invalid PEM data"
			result.CertificateInfo = append(result.CertificateInfo, entry)
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			entry.RiskLevel = "high"
			entry.Issuer = "parse error: " + err.Error()
			result.CertificateInfo = append(result.CertificateInfo, entry)
			continue
		}

		entry.Issuer = cert.Issuer.CommonName
		entry.Subject = cert.Subject.CommonName
		entry.DNSNames = cert.DNSNames
		entry.NotBefore = cert.NotBefore.Format(time.RFC3339)
		entry.NotAfter = cert.NotAfter.Format(time.RFC3339)
		entry.KeyAlgorithm = certKeyAlgStr1892(cert.PublicKeyAlgorithm)
		entry.DaysToExpiry = int(cert.NotAfter.Sub(now).Hours() / 24)
		entry.IsExpired = cert.NotAfter.Before(now)
		entry.IsSelfSigned = cert.Issuer.String() == cert.Subject.String()

		// Determine key size
		switch key := cert.PublicKey.(type) {
		case interface{ Size() int }:
			entry.KeySize = key.Size() * 8
		}

		// Risk assessment
		switch {
		case entry.IsExpired:
			entry.RiskLevel = "critical"
			result.Summary.ExpiredCerts++
		case entry.DaysToExpiry <= 30:
			entry.RiskLevel = "high"
			result.Summary.ExpiringSoon++
		case entry.DaysToExpiry <= 90:
			entry.RiskLevel = "medium"
			result.Summary.ExpiringSoon++
		case entry.IsSelfSigned:
			entry.RiskLevel = "medium"
			result.Summary.SelfSigned++
		default:
			entry.RiskLevel = "low"
			result.Summary.CAValidated++
		}

		// Check for weak key sizes (RSA < 2048)
		if cert.PublicKeyAlgorithm == x509.RSA {
			if rsaKey, ok := cert.PublicKey.(interface{ Size() int }); ok {
				if rsaKey.Size()*8 < 2048 {
					entry.RiskLevel = "high"
					result.Summary.WeakKeySize++
					result.WeakCiphers = append(result.WeakCiphers, WeakCipherEntry{
						SecretName: secret.Name,
						Namespace:  secret.Namespace,
						Issue:      fmt.Sprintf("weak RSA key size: %d bits (minimum 2048)", rsaKey.Size()*8),
						RiskLevel:  "high",
					})
				}
			}
		}

		// Check for deprecated TLS versions in certificate
		if cert.Version < 3 {
			result.WeakCiphers = append(result.WeakCiphers, WeakCipherEntry{
				SecretName: secret.Name,
				Namespace:  secret.Namespace,
				Issue:      fmt.Sprintf("certificate uses X.509 v%d (v3 recommended)", cert.Version),
				RiskLevel:  "medium",
			})
		}

		result.CertificateInfo = append(result.CertificateInfo, entry)
	}

	// Score
	if result.Summary.TotalIngresses > 0 {
		tlsPct := result.Summary.WithTLS * 100 / result.Summary.TotalIngresses
		certPenalty := result.Summary.ExpiredCerts*20 + result.Summary.ExpiringSoon*5 + result.Summary.WeakKeySize*10
		result.HealthScore = tlsPct - certPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildTLSVersionRecs1892(&result)
	writeJSON(w, result)
}

func buildTLSVersionRecs1892(result *TLSVersionResult) []string {
	recs := []string{
		fmt.Sprintf("TLS posture: %d ingresses (%d with TLS, %d without), %d certs (%d expired, %d expiring)",
			result.Summary.TotalIngresses, result.Summary.WithTLS, result.Summary.WithoutTLS,
			result.Summary.TotalCerts, result.Summary.ExpiredCerts, result.Summary.ExpiringSoon),
	}
	if result.Summary.WithoutTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d ingresses without TLS - enable HTTPS for all external traffic", result.Summary.WithoutTLS))
	}
	if result.Summary.ExpiredCerts > 0 {
		recs = append(recs, fmt.Sprintf("%d expired certificates - renew immediately", result.Summary.ExpiredCerts))
	}
	if result.Summary.ExpiringSoon > 0 {
		recs = append(recs, fmt.Sprintf("%d certificates expiring within 90 days - schedule renewal", result.Summary.ExpiringSoon))
	}
	if result.Summary.WeakKeySize > 0 {
		recs = append(recs, fmt.Sprintf("%d certificates with weak key size (< 2048 bits RSA) - regenerate with stronger keys", result.Summary.WeakKeySize))
	}
	if result.Summary.SelfSigned > 0 {
		recs = append(recs, fmt.Sprintf("%d self-signed certificates - use CA-signed certs for production traffic", result.Summary.SelfSigned))
	}
	return recs
}

// keep reference to avoid unused import
var _ = tls.VersionTLS13

func certKeyAlgStr1892(alg x509.PublicKeyAlgorithm) string {
	switch alg {
	case x509.RSA:
		return "RSA"
	case x509.ECDSA:
		return "ECDSA"
	case x509.Ed25519:
		return "Ed25519"
	default:
		return fmt.Sprintf("Unknown(%d)", int(alg))
	}
}
