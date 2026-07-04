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

// SecretRotationResult is the full secret lifecycle audit output.
type SecretRotationResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         SecretRotationSummary `json:"summary"`
	Secrets         []SecretAuditEntry    `json:"secrets"`
	ByNamespace     []SecretNsStat        `json:"byNamespace"`
	ByType          []SecretTypeStat      `json:"byType"`
	Recommendations []string              `json:"recommendations"`
}

// SecretRotationSummary aggregates cluster-wide secret metrics.
type SecretRotationSummary struct {
	TotalSecrets     int `json:"totalSecrets"`
	StaleSecrets     int `json:"staleSecrets"`     // >90 days old
	VeryStaleSecrets int `json:"veryStaleSecrets"` // >180 days old
	UnusedSecrets    int `json:"unusedSecrets"`    // not referenced by any pod
	TLSSecrets       int `json:"tlsSecrets"`
	ExpiringTLS      int `json:"expiringTLS"` // TLS cert expiring <30 days
	ExpiredTLS       int `json:"expiredTLS"`  // TLS cert already expired
	DockerSecrets    int `json:"dockerSecrets"`
	SATokens         int `json:"saTokens"`      // legacy service-account-token
	RotationScore    int `json:"rotationScore"` // 0 (worst) to 100 (best)
}

// SecretAuditEntry describes one secret with lifecycle info.
type SecretAuditEntry struct {
	Name           string            `json:"name"`
	Namespace      string            `json:"namespace"`
	Type           corev1.SecretType `json:"type"`
	AgeDays        int               `json:"ageDays"`
	IsStale        bool              `json:"isStale"`
	IsVeryStale    bool              `json:"isVeryStale"`
	IsUnused       bool              `json:"isUnused"`
	ReferencedBy   int               `json:"referencedBy"` // number of pods using it
	HasTLSExpiry   bool              `json:"hasTLSExpiry"`
	TLSExpiry      string            `json:"tlsExpiry,omitempty"`
	TLSExpired     bool              `json:"tlsExpired"`
	TLSDaysToExp   int               `json:"tlsDaysToExpiry,omitempty"`
	IsDockerSecret bool              `json:"isDockerSecret"`
	IsSAToken      bool              `json:"isSAToken"`
	SensitiveName  bool              `json:"sensitiveName"` // name contains password/key/token/secret
	RiskLevel      string            `json:"riskLevel"`     // critical / high / medium / low
}

// SecretNsStat aggregates per-namespace secret metrics.
type SecretNsStat struct {
	Namespace   string `json:"namespace"`
	Total       int    `json:"total"`
	Stale       int    `json:"stale"`
	Unused      int    `json:"unused"`
	TLSExpiring int    `json:"tlsExpiring"`
}

// SecretTypeStat aggregates per-type counts.
type SecretTypeStat struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// handleSecretRotationAudit audits all secrets for rotation compliance.
// GET /api/security/secrets/rotation?namespace=xxx
func (s *Server) handleSecretRotationAudit(w http.ResponseWriter, r *http.Request) {
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

	// Get pods to check which secrets are actually referenced
	pods, _ := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})

	// Build set of referenced secret names per namespace
	referencedSecrets := buildReferencedSecretSet(pods)

	result := SecretRotationResult{
		ScannedAt: time.Now(),
	}

	// Per-type and per-ns aggregation
	typeCounts := make(map[corev1.SecretType]int)
	nsMap := make(map[string]*SecretNsStat)
	now := time.Now()

	// TLS expiry tracking for scoring
	tlsOk := 0
	tlsChecked := 0

	for i := range secrets.Items {
		sec := &secrets.Items[i]
		ageDays := int(now.Sub(sec.CreationTimestamp.Time).Hours() / 24)

		entry := SecretAuditEntry{
			Name:      sec.Name,
			Namespace: sec.Namespace,
			Type:      sec.Type,
			AgeDays:   ageDays,
		}

		// Staleness
		if ageDays > 180 {
			entry.IsVeryStale = true
			entry.IsStale = true
		} else if ageDays > 90 {
			entry.IsStale = true
		}

		// Usage tracking
		nsKey := fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)
		refCount := referencedSecrets[nsKey]
		entry.ReferencedBy = refCount
		if refCount == 0 {
			entry.IsUnused = true
		}

		// Type-specific checks
		switch sec.Type {
		case corev1.SecretTypeTLS:
			entry.HasTLSExpiry = true
			tlsChecked++
			days, expired, expiryStr := checkTLSExpiry(sec, now)
			entry.TLSDaysToExp = days
			entry.TLSExpired = expired
			entry.TLSExpiry = expiryStr
		case corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg:
			entry.IsDockerSecret = true
		case corev1.SecretTypeServiceAccountToken:
			entry.IsSAToken = true
		}

		// Sensitive name detection
		lowerName := strings.ToLower(sec.Name)
		for _, kw := range []string{"password", "passwd", "key", "token", "secret", "credential", "cert"} {
			if strings.Contains(lowerName, kw) {
				entry.SensitiveName = true
				break
			}
		}

		// Risk level
		entry.RiskLevel = assessSecretRisk(entry)

		// Update summary
		result.Summary.TotalSecrets++
		typeCounts[sec.Type]++

		nsStat := getOrCreateSecretNs(nsMap, sec.Namespace)
		nsStat.Total++

		if entry.IsStale {
			result.Summary.StaleSecrets++
			nsStat.Stale++
		}
		if entry.IsVeryStale {
			result.Summary.VeryStaleSecrets++
		}
		if entry.IsUnused {
			result.Summary.UnusedSecrets++
			nsStat.Unused++
		}
		if entry.HasTLSExpiry {
			result.Summary.TLSSecrets++
			tlsOk++
			if entry.TLSExpired {
				result.Summary.ExpiredTLS++
			} else if entry.TLSDaysToExp < 30 {
				result.Summary.ExpiringTLS++
				nsStat.TLSExpiring++
			}
		}
		if entry.IsDockerSecret {
			result.Summary.DockerSecrets++
		}
		if entry.IsSAToken {
			result.Summary.SATokens++
		}

		result.Secrets = append(result.Secrets, entry)
	}

	// Calculate rotation score
	result.Summary.RotationScore = calculateRotationScore(result.Summary)

	// Build type stats
	for t, count := range typeCounts {
		result.ByType = append(result.ByType, SecretTypeStat{
			Type:  string(t),
			Count: count,
		})
	}
	sort.Slice(result.ByType, func(i, j int) bool {
		return result.ByType[i].Count > result.ByType[j].Count
	})

	// Build namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		if result.ByNamespace[i].Stale != result.ByNamespace[j].Stale {
			return result.ByNamespace[i].Stale > result.ByNamespace[j].Stale
		}
		return result.ByNamespace[i].Total > result.ByNamespace[j].Total
	})

	// Sort secrets by risk level then age
	sort.Slice(result.Secrets, func(i, j int) bool {
		ri := secretRiskRank(result.Secrets[i].RiskLevel)
		rj := secretRiskRank(result.Secrets[j].RiskLevel)
		if ri != rj {
			return ri < rj
		}
		return result.Secrets[i].AgeDays > result.Secrets[j].AgeDays
	})

	// Recommendations
	result.Recommendations = generateSecretRecommendations(result)

	writeJSON(w, result)
}

// buildReferencedSecretSet returns a map of "ns/name" -> reference count.
func buildReferencedSecretSet(pods *corev1.PodList) map[string]int {
	refSet := make(map[string]int)
	if pods == nil {
		return refSet
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		// Volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.Secret.SecretName)
				refSet[key]++
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil {
						key := fmt.Sprintf("%s/%s", pod.Namespace, src.Secret.Name)
						refSet[key]++
					}
				}
			}
		}

		// Env vars
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, env.ValueFrom.SecretKeyRef.Name)
					refSet[key]++
				}
			}
			for _, envFrom := range c.EnvFrom {
				if envFrom.SecretRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, envFrom.SecretRef.Name)
					refSet[key]++
				}
			}
		}

		// Image pull secrets
		for _, ips := range pod.Spec.ImagePullSecrets {
			key := fmt.Sprintf("%s/%s", pod.Namespace, ips.Name)
			refSet[key]++
		}
	}

	return refSet
}

// checkTLSExpiry extracts and checks the TLS certificate expiry from a secret.
func checkTLSExpiry(secret *corev1.Secret, now time.Time) (daysToExpiry int, expired bool, expiryStr string) {
	certData, ok := secret.Data[corev1.TLSCertKey]
	if !ok {
		return 0, false, ""
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return 0, false, ""
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0, false, ""
	}

	expiry := cert.NotAfter
	days := int(expiry.Sub(now).Hours() / 24)
	expired = days < 0
	return days, expired, expiry.Format(time.RFC3339)
}

// assessSecretRisk determines risk level for a secret.
func assessSecretRisk(entry SecretAuditEntry) string {
	// Critical: expired TLS or very stale docker/SA token
	if entry.TLSExpired {
		return "critical"
	}
	if entry.IsVeryStale && (entry.IsDockerSecret || entry.IsSAToken) {
		return "critical"
	}

	// High: expiring TLS or stale + unused + sensitive name
	if entry.HasTLSExpiry && entry.TLSDaysToExp < 30 && !entry.TLSExpired {
		return "high"
	}
	if entry.IsStale && entry.IsUnused && entry.SensitiveName {
		return "high"
	}

	// Medium: stale or unused docker secret
	if entry.IsStale && entry.IsDockerSecret {
		return "medium"
	}
	if entry.IsUnused && entry.SensitiveName {
		return "medium"
	}

	// Low: stale but in use, or unused but non-sensitive
	if entry.IsStale {
		return "low"
	}
	if entry.IsUnused {
		return "low"
	}

	return "low"
}

// calculateRotationScore computes 0-100 based on secret health.
func calculateRotationScore(s SecretRotationSummary) int {
	if s.TotalSecrets == 0 {
		return 100
	}
	score := 100

	// Expired TLS: -15 each
	score -= s.ExpiredTLS * 15
	// Expiring TLS: -5 each
	score -= s.ExpiringTLS * 5
	// Very stale: -3 each
	score -= s.VeryStaleSecrets * 3
	// Stale: -1 each
	score -= (s.StaleSecrets - s.VeryStaleSecrets) * 1
	// Unused: -2 each
	score -= s.UnusedSecrets * 2

	if score < 0 {
		score = 0
	}
	return score
}

// generateSecretRecommendations produces actionable advice.
func generateSecretRecommendations(result SecretRotationResult) []string {
	var recs []string
	s := result.Summary

	if s.ExpiredTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d TLS certificate secret(s) have ALREADY EXPIRED — renew immediately to prevent service disruption", s.ExpiredTLS))
	}
	if s.ExpiringTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d TLS certificate secret(s) expiring within 30 days — schedule renewal now", s.ExpiringTLS))
	}
	if s.VeryStaleSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d secret(s) older than 180 days — review and rotate as part of security policy", s.VeryStaleSecrets))
	}
	if s.UnusedSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d secret(s) are not referenced by any pod — clean up to reduce attack surface", s.UnusedSecrets))
	}
	if s.SATokens > 0 {
		recs = append(recs, fmt.Sprintf("%d legacy service-account-token secret(s) detected — Kubernetes 1.24+ uses projected tokens; clean up unused ones", s.SATokens))
	}
	if s.RotationScore < 50 {
		recs = append(recs, fmt.Sprintf("Secret rotation score is %d/100 — implement automated rotation policies for improved security posture", s.RotationScore))
	}

	// Top namespace by stale count
	if len(result.ByNamespace) > 0 && result.ByNamespace[0].Stale > 0 {
		recs = append(recs, fmt.Sprintf("Namespace %q has %d stale secret(s) — prioritize rotation in this namespace", result.ByNamespace[0].Namespace, result.ByNamespace[0].Stale))
	}

	return recs
}

func getOrCreateSecretNs(m map[string]*SecretNsStat, ns string) *SecretNsStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &SecretNsStat{Namespace: ns}
	m[ns] = e
	return e
}

func secretRiskRank(level string) int {
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
