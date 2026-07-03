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

// SecretExposureReport summarizes secret usage and exposure risks.
type SecretExposureReport struct {
	Summary  SecretExposureSummary `json:"summary"`
	Secrets  []SecretInfo          `json:"secrets"`
	Exposed  []ExposedEnvVar       `json:"exposedEnvVars"`
	Findings []SecurityFinding     `json:"findings"`
}

// SecretExposureSummary holds aggregate stats.
type SecretExposureSummary struct {
	TotalSecrets   int            `json:"totalSecrets"`
	ByType         map[string]int `json:"byType"`
	UnusedSecrets  int            `json:"unusedSecrets"`
	ExposedEnvVars int            `json:"exposedEnvVars"`
	NeedsRotation  int            `json:"needsRotation"`
}

// SecretInfo describes a Kubernetes Secret.
type SecretInfo struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Type       string   `json:"type"`
	DataKeys   []string `json:"dataKeys"`
	KeyCount   int      `json:"keyCount"`
	CreatedAt  string   `json:"createdAt"`
	Age        string   `json:"age"`
	UsedByPods int      `json:"usedByPods"`
}

// ExposedEnvVar represents a potentially sensitive env var that is
// hardcoded in a pod spec instead of using a Secret reference.
type ExposedEnvVar struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	EnvVar    string `json:"envVar"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// handleSecretExposure scans for secret exposure risks.
// GET /api/security/secrets
func (s *Server) handleSecretExposure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	secrets, err := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	report := analyzeSecretExposure(secrets.Items, pods.Items)

	writeJSON(w, report)
}

// sensitiveKeyPatterns are lowercase substrings that suggest a secret value.
var sensitiveKeyPatterns = []string{
	"password", "passwd", "pwd",
	"token", "apikey", "api_key", "api-key",
	"secret", "credential",
	"private_key", "privatekey",
	"access_key", "accesskey",
	"auth", "bearer",
}

// sensitiveEnvPatterns are env var names that look hardcoded secrets.
var sensitiveEnvPatterns = sensitiveKeyPatterns

// analyzeSecretExposure inspects secrets and pods for exposure risks.
func analyzeSecretExposure(secrets []corev1.Secret, pods []corev1.Pod) SecretExposureReport {
	report := SecretExposureReport{
		Summary: SecretExposureSummary{
			ByType: map[string]int{},
		},
	}

	// 1. Build secret info and usage map
	secretUsage := map[string]int{} // "ns/name" -> pod count

	for _, sec := range secrets {
		if isSystemNamespace(sec.Namespace) {
			continue
		}

		keys := make([]string, 0, len(sec.Data))
		for k := range sec.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		report.Summary.TotalSecrets++
		report.Summary.ByType[string(sec.Type)]++

		// Check for rotation needs (>90 days old)
		age := time.Since(sec.CreationTimestamp.Time)
		needsRotation := age > 90*24*time.Hour

		if needsRotation {
			report.Summary.NeedsRotation++
		}

		info := SecretInfo{
			Name:      sec.Name,
			Namespace: sec.Namespace,
			Type:      string(sec.Type),
			DataKeys:  keys,
			KeyCount:  len(keys),
			CreatedAt: sec.CreationTimestamp.Format(time.RFC3339),
			Age:       formatDuration(age),
		}
		report.Secrets = append(report.Secrets, info)

		// Flag Opaque secrets with sensitive key names
		if sec.Type == corev1.SecretTypeOpaque {
			for _, k := range keys {
				lk := strings.ToLower(k)
				for _, pat := range sensitiveKeyPatterns {
					if strings.Contains(lk, pat) {
						report.Findings = append(report.Findings, SecurityFinding{
							Severity:  "medium",
							Category:  "Secrets",
							Resource:  fmt.Sprintf("%s/secret/%s", sec.Namespace, sec.Name),
							Namespace: sec.Namespace,
							Detail:    fmt.Sprintf("Secret %q contains key %q that looks sensitive — ensure it's not logged or exposed", sec.Name, k),
							Fix:       "Use external secret management (Vault, Sealed Secrets) for production credentials",
						})
						break
					}
				}
			}
		}

		// Rotation finding
		if needsRotation {
			report.Findings = append(report.Findings, SecurityFinding{
				Severity:  "low",
				Category:  "Secrets",
				Resource:  fmt.Sprintf("%s/secret/%s", sec.Namespace, sec.Name),
				Namespace: sec.Namespace,
				Detail:    fmt.Sprintf("Secret %q is %d days old — consider rotating", sec.Name, int(age.Hours()/24)),
				Fix:       "Rotate secrets periodically (recommended: every 90 days)",
			})
		}
	}

	// 2. Scan pods for hardcoded sensitive env vars
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				// Skip env vars using secretKeyRef or configMapKeyRef
				if env.ValueFrom != nil {
					if env.ValueFrom.SecretKeyRef != nil {
						// Track secret usage
						key := fmt.Sprintf("%s/%s", pod.Namespace, env.ValueFrom.SecretKeyRef.Name)
						secretUsage[key]++
						continue
					}
					continue
				}

				// Check if env var name looks sensitive and has a hardcoded value
				envLower := strings.ToLower(env.Name)
				for _, pat := range sensitiveEnvPatterns {
					if strings.Contains(envLower, pat) && env.Value != "" {
						// Determine severity based on value characteristics
						sev := "high"
						detail := fmt.Sprintf("Container %q has hardcoded %q env var (should use Secret)", c.Name, env.Name)

						// Check if value looks like a real secret (not a placeholder)
						val := env.Value
						if strings.Contains(val, "XXXX") || strings.Contains(val, "CHANGE") ||
							strings.Contains(val, "your-") || strings.Contains(val, "example") {
							sev = "low"
							detail = fmt.Sprintf("Container %q has %q env var with placeholder value", c.Name, env.Name)
						}

						report.Exposed = append(report.Exposed, ExposedEnvVar{
							Pod:       pod.Name,
							Namespace: pod.Namespace,
							Container: c.Name,
							EnvVar:    env.Name,
							Severity:  sev,
							Detail:    detail,
						})
						report.Summary.ExposedEnvVars++

						// Also add as finding
						report.Findings = append(report.Findings, SecurityFinding{
							Severity:  sev,
							Category:  "Secrets",
							Resource:  fmt.Sprintf("%s/pod/%s/container/%s", pod.Namespace, pod.Name, c.Name),
							Namespace: pod.Namespace,
							Detail:    detail,
							Fix:       "Move this value to a Kubernetes Secret and reference via secretKeyRef",
						})
						break
					}
				}
			}

			// Check envFrom for secret refs (track usage)
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, ef.SecretRef.Name)
					secretUsage[key]++
				}
			}
		}

		// Check volumes for secret references
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.Secret.SecretName)
				secretUsage[key]++
			}
		}

		// Check imagePullSecrets
		for _, ips := range pod.Spec.ImagePullSecrets {
			key := fmt.Sprintf("%s/%s", pod.Namespace, ips.Name)
			secretUsage[key]++
		}
	}

	// 3. Update usedByPods count and find unused secrets
	for i := range report.Secrets {
		key := fmt.Sprintf("%s/%s", report.Secrets[i].Namespace, report.Secrets[i].Name)
		report.Secrets[i].UsedByPods = secretUsage[key]
		if secretUsage[key] == 0 {
			report.Summary.UnusedSecrets++
		}
	}

	// Sort secrets: unused first, then by age
	sort.Slice(report.Secrets, func(i, j int) bool {
		if (report.Secrets[i].UsedByPods == 0) != (report.Secrets[j].UsedByPods == 0) {
			return report.Secrets[i].UsedByPods == 0
		}
		return report.Secrets[i].Age > report.Secrets[j].Age
	})

	// Sort findings by severity
	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
	sort.Slice(report.Findings, func(i, j int) bool {
		return sevOrder[report.Findings[i].Severity] < sevOrder[report.Findings[j].Severity]
	})

	return report
}

// formatDuration is defined in server.go — reused.
