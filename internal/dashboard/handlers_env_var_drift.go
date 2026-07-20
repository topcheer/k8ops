package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnvVarDriftResult detects environment variable inconsistencies across same-name workloads in different namespaces.
type EnvVarDriftResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         EnvVarDriftSummary `json:"summary"`
	ByWorkload      []EnvVarDriftEntry `json:"byWorkload"`
	Inconsistencies []EnvVarDriftIssue `json:"inconsistencies"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type EnvVarDriftSummary struct {
	TotalDeployments  int `json:"totalDeployments"`
	WithEnvVars       int `json:"withEnvVars"`
	SecretEnvCount    int `json:"secretEnvCount"`
	ConfigMapEnvCount int `json:"configMapEnvCount"`
	DriftDetected     int `json:"driftDetected"`
	HardcodedSecrets  int `json:"hardcodedSecrets"`
}

type EnvVarDriftEntry struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	EnvVarCount int      `json:"envVarCount"`
	EnvNames    []string `json:"envNames"`
	HasSecret   bool     `json:"hasSecretEnv"`
	HasCM       bool     `json:"hasConfigMapEnv"`
	RiskLevel   string   `json:"riskLevel"`
}

type EnvVarDriftIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	EnvName   string `json:"envName"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleEnvVarDriftDetect handles GET /api/product/env-var-drift-detect
func (s *Server) handleEnvVarDriftDetect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := EnvVarDriftResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	// Build workload env var map by name for cross-namespace drift detection
	nameToEnvs := make(map[string][]map[string]string) // deploy name -> []{ns: envlist}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		entry := EnvVarDriftEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
		}

		var envNames []string
		for _, c := range dep.Spec.Template.Spec.Containers {
			for _, e := range c.Env {
				envNames = append(envNames, e.Name)
				entry.EnvVarCount++

				// Check for hardcoded secrets
				if e.Value != "" && (containsStr1876(e.Name, "PASSWORD") ||
					containsStr1876(e.Name, "SECRET") ||
					containsStr1876(e.Name, "TOKEN") ||
					containsStr1876(e.Name, "KEY")) {
					result.Summary.HardcodedSecrets++
					result.Inconsistencies = append(result.Inconsistencies, EnvVarDriftIssue{
						Name: dep.Name, Namespace: dep.Namespace,
						EnvName: e.Name, Issue: "hardcoded secret in env var",
						Severity: "critical",
					})
				}

				// Check for ConfigMap/Secret references
				if e.ValueFrom != nil {
					if e.ValueFrom.SecretKeyRef != nil {
						entry.HasSecret = true
						result.Summary.SecretEnvCount++
					}
					if e.ValueFrom.ConfigMapKeyRef != nil {
						entry.HasCM = true
						result.Summary.ConfigMapEnvCount++
					}
				}
			}
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					entry.HasSecret = true
					result.Summary.SecretEnvCount++
				}
				if ef.ConfigMapRef != nil {
					entry.HasCM = true
					result.Summary.ConfigMapEnvCount++
				}
			}
		}

		entry.EnvNames = envNames
		if entry.EnvVarCount > 0 {
			result.Summary.WithEnvVars++
		}

		// Risk level
		switch {
		case result.Summary.HardcodedSecrets > 0 && entry.HasSecret:
			entry.RiskLevel = "critical"
		case entry.EnvVarCount > 20:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		// Track for cross-namespace drift
		nameToEnvs[dep.Name] = append(nameToEnvs[dep.Name], map[string]string{dep.Namespace: fmt.Sprintf("%v", envNames)})

		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Detect drift across namespaces
	for name, envSets := range nameToEnvs {
		if len(envSets) < 2 {
			continue
		}
		first := envSets[0]
		for _, nsEnv := range envSets[1:] {
			for ns1, e1 := range first {
				for ns2, e2 := range nsEnv {
					if ns1 != ns2 && e1 != e2 {
						result.Summary.DriftDetected++
						result.Inconsistencies = append(result.Inconsistencies, EnvVarDriftIssue{
							Name: name, Namespace: ns1 + "/" + ns2,
							EnvName: "cross-ns-drift", Issue: fmt.Sprintf("%s has different env vars in %s vs %s", name, ns1, ns2),
							Severity: "medium",
						})
					}
				}
			}
		}
	}

	sort.Slice(result.ByWorkload, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "medium": 1, "low": 2}
		return rank[result.ByWorkload[i].RiskLevel] < rank[result.ByWorkload[j].RiskLevel]
	})

	if result.Summary.TotalDeployments > 0 {
		result.HealthScore = 100
		result.HealthScore -= result.Summary.HardcodedSecrets * 20
		result.HealthScore -= result.Summary.DriftDetected * 5
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("环境变量漂移: %d 部署, %d 有 env, %d Secret env, %d CM env, %d 硬编码密钥, %d 漂移",
			result.Summary.TotalDeployments, result.Summary.WithEnvVars,
			result.Summary.SecretEnvCount, result.Summary.ConfigMapEnvCount,
			result.Summary.HardcodedSecrets, result.Summary.DriftDetected),
	}
	if result.Summary.HardcodedSecrets > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个硬编码密钥, 应迁移到 Secret", result.Summary.HardcodedSecrets))
	}
	writeJSON(w, result)
}
