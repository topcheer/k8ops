package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvConfigDriftResult is the deployment environment configuration drift audit.
type EnvConfigDriftResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         EnvConfigDriftSummary `json:"summary"`
	Deployments     []EnvConfigEntry      `json:"deployments"`
	Issues          []EnvConfigIssue      `json:"issues"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

type EnvConfigDriftSummary struct {
	TotalDeployments int `json:"totalDeployments"`
	WithConfigMapRef int `json:"withConfigMapRef"`
	WithSecretRef    int `json:"withSecretRef"`
	WithEnvVars      int `json:"withEnvVars"`
	MissingRefs      int `json:"missingRefs"`      // ConfigMap/Secret referenced but not found
	HardcodedSecrets int `json:"hardcodedSecrets"` // env var value that looks like a secret
	InconsistentEnvs int `json:"inconsistentEnvs"` // same deploy name, different envs across namespaces
}

type EnvConfigEntry struct {
	Name           string   `json:"name"`
	Namespace      string   `json:"namespace"`
	ConfigMapRefs  []string `json:"configMapRefs,omitempty"`
	SecretRefs     []string `json:"secretRefs,omitempty"`
	EnvVarCount    int      `json:"envVarCount"`
	HasMissingRefs bool     `json:"hasMissingRefs"`
	Status         string   `json:"status"`
}

type EnvConfigIssue struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

func (s *Server) handleEnvConfigDrift(w http.ResponseWriter, r *http.Request) {
	result := EnvConfigDriftResult{ScannedAt: time.Now()}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	// Build ConfigMap and Secret existence maps
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	cmExists := make(map[string]bool)
	if configmaps != nil {
		for _, cm := range configmaps.Items {
			cmExists[fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)] = true
		}
	}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	secretExists := make(map[string]bool)
	if secrets != nil {
		for _, sec := range secrets.Items {
			secretExists[fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)] = true
		}
	}

	// Track same deployment name across namespaces for env inconsistency

	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deployments.Items {
			if systemNS[dep.Namespace] {
				continue
			}
			result.Summary.TotalDeployments++

			entry := EnvConfigEntry{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Status:    "healthy",
			}

			cmRefs := []string{}
			secretRefs := []string{}
			envVarCount := 0
			hasMissing := false

			for _, c := range dep.Spec.Template.Spec.Containers {
				// Check env vars
				for _, ev := range c.Env {
					envVarCount++
					// Check for hardcoded secrets (heuristic)
					if ev.Value != "" {
						lower := strings.ToLower(ev.Name)
						if strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
							strings.Contains(lower, "token") || strings.Contains(lower, "key") ||
							strings.Contains(lower, "credential") {
							result.Summary.HardcodedSecrets++
							result.Issues = append(result.Issues, EnvConfigIssue{
								Namespace: dep.Namespace, Name: dep.Name,
								Issue:    fmt.Sprintf("Hardcoded secret in env var %s — use SecretKeyRef instead", ev.Name),
								Severity: "high",
							})
						}
					}
				}

				// Check ConfigMap env refs
				for _, cmRef := range c.EnvFrom {
					if cmRef.ConfigMapRef != nil {
						cmName := cmRef.ConfigMapRef.Name
						cmRefs = append(cmRefs, cmName)
						key := fmt.Sprintf("%s/%s", dep.Namespace, cmName)
						if !cmExists[key] {
							hasMissing = true
							result.Summary.MissingRefs++
							result.Issues = append(result.Issues, EnvConfigIssue{
								Namespace: dep.Namespace, Name: dep.Name,
								Issue:    fmt.Sprintf("ConfigMap %s referenced but not found", cmName),
								Severity: "critical",
							})
						}
					}
					if cmRef.SecretRef != nil {
						secName := cmRef.SecretRef.Name
						secretRefs = append(secretRefs, secName)
						key := fmt.Sprintf("%s/%s", dep.Namespace, secName)
						if !secretExists[key] {
							hasMissing = true
							result.Summary.MissingRefs++
							result.Issues = append(result.Issues, EnvConfigIssue{
								Namespace: dep.Namespace, Name: dep.Name,
								Issue:    fmt.Sprintf("Secret %s referenced but not found", secName),
								Severity: "critical",
							})
						}
					}
				}

				// Check env valueFrom refs
				for _, ev := range c.Env {
					if ev.ValueFrom != nil {
						if ev.ValueFrom.ConfigMapKeyRef != nil {
							cmName := ev.ValueFrom.ConfigMapKeyRef.Name
							cmRefs = append(cmRefs, cmName)
							key := fmt.Sprintf("%s/%s", dep.Namespace, cmName)
							if !cmExists[key] {
								hasMissing = true
								result.Summary.MissingRefs++
								result.Issues = append(result.Issues, EnvConfigIssue{
									Namespace: dep.Namespace, Name: dep.Name,
									Issue:    fmt.Sprintf("ConfigMap %s referenced in env but not found", cmName),
									Severity: "critical",
								})
							}
						}
						if ev.ValueFrom.SecretKeyRef != nil {
							secName := ev.ValueFrom.SecretKeyRef.Name
							secretRefs = append(secretRefs, secName)
							key := fmt.Sprintf("%s/%s", dep.Namespace, secName)
							if !secretExists[key] {
								hasMissing = true
								result.Summary.MissingRefs++
								result.Issues = append(result.Issues, EnvConfigIssue{
									Namespace: dep.Namespace, Name: dep.Name,
									Issue:    fmt.Sprintf("Secret %s referenced in env but not found", secName),
									Severity: "critical",
								})
							}
						}
					}
				}
			}

			if len(cmRefs) > 0 {
				result.Summary.WithConfigMapRef++
			}
			if len(secretRefs) > 0 {
				result.Summary.WithSecretRef++
			}
			if envVarCount > 0 {
				result.Summary.WithEnvVars++
			}

			entry.ConfigMapRefs = cmRefs
			entry.SecretRefs = secretRefs
			entry.EnvVarCount = envVarCount
			entry.HasMissingRefs = hasMissing
			if hasMissing {
				entry.Status = "broken"
			}

			result.Deployments = append(result.Deployments, entry)
		}
	}

	sort.Slice(result.Deployments, func(i, j int) bool {
		return result.Deployments[i].Status > result.Deployments[j].Status
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return result.Issues[i].Severity > result.Issues[j].Severity
	})

	if result.Summary.MissingRefs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ConfigMap/Secret references are broken — create missing resources or fix references", result.Summary.MissingRefs))
	}
	if result.Summary.HardcodedSecrets > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d hardcoded secrets in env vars — migrate to SecretKeyRef for security", result.Summary.HardcodedSecrets))
	}

	score := 100
	score -= result.Summary.MissingRefs * 15
	score -= result.Summary.HardcodedSecrets * 10
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}

var _ = appsv1.DeploymentSpec{}
var _ = corev1.EnvVar{}
