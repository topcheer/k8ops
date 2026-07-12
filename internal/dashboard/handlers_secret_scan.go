package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecretExpResult is the secret data exposure & environment variable credential leak scan.
type SecretExpResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         SecretExpSummary   `json:"summary"`
	ExposedSecrets  []SecretScanEntry  `json:"exposedSecrets"`
	EnvVarLeaks     []EnvLeakEntry     `json:"envVarLeaks"`
	ByNamespace     []SecretScanNSStat `json:"byNamespace"`
	Recommendations []string           `json:"recommendations"`
}

// SecretExpSummary aggregates secret exposure statistics.
type SecretExpSummary struct {
	TotalSecrets        int `json:"totalSecrets"`
	SecretsMounted      int `json:"secretsMounted"`      // mounted as volumes
	SecretsAsEnv        int `json:"secretsAsEnv"`        // used as environment variables
	SecretsAsEnvPlain   int `json:"secretsAsEnvPlain"`   // SecretKeyRef without envFrom
	ExposedPlainSecrets int `json:"exposedPlainSecrets"` // secrets with sensitive keys mounted as env
	StaleSecrets        int `json:"staleSecrets"`        // secrets older than 90 days
	UnreferencedSecrets int `json:"unreferencedSecrets"` // not mounted or env-referenced
	HealthScore         int `json:"healthScore"`
}

// SecretScanEntry describes a potentially exposed secret.
type SecretScanEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	MountType string `json:"mountType"` // volume, env, envFrom
	Age       string `json:"age"`
	RiskLevel string `json:"riskLevel"`
}

// EnvLeakEntry describes a credential detected in environment variables.
type EnvLeakEntry struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	Container    string `json:"container"`
	EnvVarName   string `json:"envVarName"`
	HasValue     bool   `json:"hasValue"`     // inline value (not from SecretKeyRef)
	DetectedType string `json:"detectedType"` // password, token, key, etc.
	Severity     string `json:"severity"`
}

// SecretScanNSStat shows secret exposure per namespace.
type SecretScanNSStat struct {
	Namespace      string `json:"namespace"`
	TotalSecrets   int    `json:"totalSecrets"`
	ExposedSecrets int    `json:"exposedSecrets"`
	EnvVarLeaks    int    `json:"envVarLeaks"`
	IsSystem       bool   `json:"isSystem"`
}

// handleSecretScan scans for secret data exposure and environment variable credential leaks.
// GET /api/security/secret-scan
func (s *Server) handleSecretScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build secret reference map: ns/name -> referenced (by volume or env)
	secretRefs := map[string]bool{}
	envSecretRefs := map[string]bool{}

	for _, pod := range pods.Items {
		// Check volume mounts
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil && vol.Secret.SecretName != "" {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.Secret.SecretName)
				secretRefs[key] = true
			}
		}

		// Check env vars and envFrom
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, env.ValueFrom.SecretKeyRef.Name)
					secretRefs[key] = true
					envSecretRefs[key] = true

					// Check if env var name looks like a credential
					if isCredentialEnvVar(env.Name) {
						result_EnvVarLeaks = append(result_EnvVarLeaks, EnvLeakEntry{
							PodName:      pod.Name,
							Namespace:    pod.Namespace,
							Container:    c.Name,
							EnvVarName:   env.Name,
							HasValue:     false,
							DetectedType: classifyCredential(env.Name),
							Severity:     "medium",
						})
					}
				}

				// Check for inline credential values (not from secret)
				if env.Value != "" && isCredentialValue(env.Name, env.Value) {
					result_EnvVarLeaks = append(result_EnvVarLeaks, EnvLeakEntry{
						PodName:      pod.Name,
						Namespace:    pod.Namespace,
						Container:    c.Name,
						EnvVarName:   env.Name,
						HasValue:     true,
						DetectedType: classifyCredential(env.Name),
						Severity:     "high",
					})
				}
			}

			// Check envFrom
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil && ef.SecretRef.Name != "" {
					key := fmt.Sprintf("%s/%s", pod.Namespace, ef.SecretRef.Name)
					secretRefs[key] = true
					envSecretRefs[key] = true
				}
			}
		}
	}

	now := time.Now()
	result := SecretExpResult{ScannedAt: now}
	result.Summary.TotalSecrets = len(secrets.Items)
	nsStats := map[string]*SecretScanNSStat{}

	// Pre-declare the env var leaks slice properly
	result.EnvVarLeaks = result_EnvVarLeaks
	result_EnvVarLeaks = nil

	for _, sec := range secrets.Items {
		nsStat, ok := nsStats[sec.Namespace]
		if !ok {
			nsStat = &SecretScanNSStat{Namespace: sec.Namespace, IsSystem: isSystemNamespace(sec.Namespace)}
			nsStats[sec.Namespace] = nsStat
		}
		nsStat.TotalSecrets++

		key := fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)
		isReferenced := secretRefs[key]
		isEnvRef := envSecretRefs[key]

		age := now.Sub(sec.CreationTimestamp.Time)
		isStale := age > 90*24*time.Hour

		if isEnvRef {
			result.Summary.SecretsAsEnv++
			result.Summary.SecretsAsEnvPlain++
		}
		if isReferenced && !isEnvRef {
			result.Summary.SecretsMounted++
		}

		if isStale {
			result.Summary.StaleSecrets++
		}

		if !isReferenced {
			result.Summary.UnreferencedSecrets++
		}

		// Check for sensitive keys exposed as env vars
		if isEnvRef {
			hasSensitiveKey := false
			for k := range sec.Data {
				if isSensitiveKey(k) {
					hasSensitiveKey = true
					break
				}
			}
			if hasSensitiveKey {
				result.Summary.ExposedPlainSecrets++
				nsStat.ExposedSecrets++
				risk := "medium"
				if isStale {
					risk = "high"
				}
				result.ExposedSecrets = append(result.ExposedSecrets, SecretScanEntry{
					Name:      sec.Name,
					Namespace: sec.Namespace,
					Type:      string(sec.Type),
					MountType: "env",
					Age:       formatDuration(age),
					RiskLevel: risk,
				})
			}
		}
	}

	// Update namespace leak counts
	for _, leak := range result.EnvVarLeaks {
		nsStat, ok := nsStats[leak.Namespace]
		if ok {
			nsStat.EnvVarLeaks++
		} else {
			nsStats[leak.Namespace] = &SecretScanNSStat{
				Namespace:   leak.Namespace,
				EnvVarLeaks: 1,
			}
		}
	}

	// Build namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ExposedSecrets+result.ByNamespace[i].EnvVarLeaks >
			result.ByNamespace[j].ExposedSecrets+result.ByNamespace[j].EnvVarLeaks
	})

	// Sort exposed secrets by risk
	sort.Slice(result.ExposedSecrets, func(i, j int) bool {
		riskOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return riskOrder[result.ExposedSecrets[i].RiskLevel] < riskOrder[result.ExposedSecrets[j].RiskLevel]
	})
	if len(result.ExposedSecrets) > 30 {
		result.ExposedSecrets = result.ExposedSecrets[:30]
	}

	// Sort env var leaks by severity (inline first)
	sort.Slice(result.EnvVarLeaks, func(i, j int) bool {
		if result.EnvVarLeaks[i].HasValue != result.EnvVarLeaks[j].HasValue {
			return result.EnvVarLeaks[i].HasValue
		}
		return result.EnvVarLeaks[i].Severity < result.EnvVarLeaks[j].Severity
	})
	if len(result.EnvVarLeaks) > 30 {
		result.EnvVarLeaks = result.EnvVarLeaks[:30]
	}

	result.Summary.HealthScore = secretScanScore(result.Summary)
	result.Recommendations = secretScanRecommendations(&result)

	writeJSON(w, result)
}

// Package-level temp slice for collecting env var leaks during pod iteration.
var result_EnvVarLeaks []EnvLeakEntry

// isCredentialEnvVar checks if an env var name looks like a credential.
func isCredentialEnvVar(name string) bool {
	lower := strings.ToLower(name)
	credentialPatterns := []string{"password", "passwd", "pwd", "token", "secret", "api_key", "apikey", "access_key", "private_key", "credential", "auth"}
	for _, p := range credentialPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// isCredentialValue checks if an env var has an inline value that looks like a credential.
func isCredentialValue(name, value string) bool {
	if !isCredentialEnvVar(name) {
		return false
	}
	// Only flag if the value is non-empty and looks like a real credential
	return len(value) > 0
}

// classifyCredential returns the type of credential detected.
func classifyCredential(name string) string {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "password") || strings.Contains(lower, "passwd") || strings.Contains(lower, "pwd") {
		return "password"
	}
	if strings.Contains(lower, "token") {
		return "token"
	}
	if strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") {
		return "api_key"
	}
	if strings.Contains(lower, "private_key") || strings.Contains(lower, "privatekey") {
		return "private_key"
	}
	if strings.Contains(lower, "access_key") || strings.Contains(lower, "accesskey") {
		return "access_key"
	}
	if strings.Contains(lower, "credential") {
		return "credential"
	}
	return "secret"
}

// isSensitiveKey checks if a Secret data key looks sensitive.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	sensitive := []string{"password", "passwd", "pwd", "token", "secret", "key", "credential", "cert", "private", "auth", "pass"}
	for _, s := range sensitive {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// secretScanScore computes a 0-100 health score.
func secretScanScore(s SecretExpSummary) int {
	if s.TotalSecrets == 0 {
		return 100
	}

	score := 100

	if s.ExposedPlainSecrets > 0 {
		score -= min(20, s.ExposedPlainSecrets*3)
	}

	if s.StaleSecrets > 0 {
		ratio := float64(s.StaleSecrets) / float64(s.TotalSecrets)
		score -= int(ratio * 20)
	}

	if s.UnreferencedSecrets > 0 {
		ratio := float64(s.UnreferencedSecrets) / float64(s.TotalSecrets)
		score -= int(ratio * 15)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// secretScanRecommendations generates actionable recommendations.
func secretScanRecommendations(r *SecretExpResult) []string {
	var recs []string

	inlineCount := 0
	for _, leak := range r.EnvVarLeaks {
		if leak.HasValue {
			inlineCount++
		}
	}
	if inlineCount > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d credential(s) detected as inline environment variables — move to Kubernetes Secrets or external secret management",
			inlineCount,
		))
	}

	if r.Summary.ExposedPlainSecrets > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d Secret(s) with sensitive keys are exposed as environment variables — consider using volume mounts with read-only access",
			r.Summary.ExposedPlainSecrets,
		))
	}

	if r.Summary.StaleSecrets > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d Secret(s) are older than 90 days — rotate credentials regularly",
			r.Summary.StaleSecrets,
		))
	}

	if r.Summary.UnreferencedSecrets > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d Secret(s) are not referenced by any pod — clean up unused secrets to reduce attack surface",
			r.Summary.UnreferencedSecrets,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "Secret management is healthy — no credential leaks or exposure risks detected")
	}

	return recs
}
