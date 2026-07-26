package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.95 — Deployment Dimension (Round 19)
// 1. Restart Policy Audit — pod restart policy compliance
// 2. Revision History Audit — rollout history limit configuration
// 3. Container Env Health — env var configuration quality
// ============================================================

// ---------------------------------------------------------------
// 1. Restart Policy Audit
// ---------------------------------------------------------------

type RestartPolResult1995 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         RestartPolSummary1995 `json:"summary"`
	Issues          []RestartPolEntry1995 `json:"issues"`
	Recommendations []string              `json:"recommendations"`
}

type RestartPolSummary1995 struct {
	TotalPods     int `json:"totalPods"`
	AlwaysPolicy  int `json:"alwaysPolicy"`
	OnFailure     int `json:"onFailurePolicy"`
	NeverPolicy   int `json:"neverPolicy"`
	Misconfigured int `json:"misconfigured"`
}

type RestartPolEntry1995 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Policy    string `json:"restartPolicy"`
	Issue     string `json:"issue"`
}

func (s *Server) handleRestartPolicyAudit(w http.ResponseWriter, r *http.Request) {
	result := RestartPolResult1995{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		policy := string(pod.Spec.RestartPolicy)
		entry := RestartPolEntry1995{
			Pod: pod.Name, Namespace: pod.Namespace, Policy: policy,
		}

		switch policy {
		case "Always":
			result.Summary.AlwaysPolicy++
		case "OnFailure":
			result.Summary.OnFailure++
		case "Never":
			result.Summary.NeverPolicy++
			result.Summary.Misconfigured++
			entry.Issue = "RestartPolicy: Never — pods won't recover from crashes"
			result.Issues = append(result.Issues, entry)
			score -= 2
		}

		// Check for Job/CronJob pods with Always (should be OnFailure/Never)
		isJob := false
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "Job" {
				isJob = true
				break
			}
		}
		if isJob && policy == "Always" {
			result.Summary.Misconfigured++
			entry.Issue = "Job pod with RestartPolicy: Always (should be OnFailure/Never)"
			result.Issues = append(result.Issues, entry)
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d Always, %d OnFailure, %d Never, %d misconfigured", result.Summary.TotalPods, result.Summary.AlwaysPolicy, result.Summary.OnFailure, result.Summary.NeverPolicy, result.Summary.Misconfigured))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Revision History Audit
// ---------------------------------------------------------------

type RevHistResult1995 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         RevHistSummary1995 `json:"summary"`
	Issues          []RevHistEntry1995 `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

type RevHistSummary1995 struct {
	TotalDeployments int `json:"totalDeployments"`
	WithCustomLimit  int `json:"withCustomLimit"`
	UsingDefault     int `json:"usingDefault10"`
	TooLow           int `json:"tooLowHistoryLimit"`
}

type RevHistEntry1995 struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	HistoryLimit *int32 `json:"revisionHistoryLimit"`
}

func (s *Server) handleRevisionHistoryAudit(w http.ResponseWriter, r *http.Request) {
	result := RevHistResult1995{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		entry := RevHistEntry1995{
			Name: dep.Name, Namespace: dep.Namespace,
		}

		if dep.Spec.RevisionHistoryLimit != nil {
			limit := *dep.Spec.RevisionHistoryLimit
			entry.HistoryLimit = &limit
			result.Summary.WithCustomLimit++

			if limit < 2 {
				result.Summary.TooLow++
				result.Issues = append(result.Issues, entry)
				score -= 1
			}
		} else {
			result.Summary.UsingDefault++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d custom limit, %d default (10), %d too low", result.Summary.TotalDeployments, result.Summary.WithCustomLimit, result.Summary.UsingDefault, result.Summary.TooLow))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Env Health
// ---------------------------------------------------------------

type EnvHealthResult1995 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EnvHealthSummary1995 `json:"summary"`
	Issues          []EnvHealthEntry1995 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type EnvHealthSummary1995 struct {
	TotalContainers  int `json:"totalContainers"`
	WithEnv          int `json:"withEnvVars"`
	WithSecretRef    int `json:"withSecretRef"`
	WithConfigMapRef int `json:"withConfigMapRef"`
	Hardcoded        int `json:"hardcodedSensitive"`
}

type EnvHealthEntry1995 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	EnvName   string `json:"envName"`
	Issue     string `json:"issue"`
}

var sensitiveEnvNames1995 = map[string]bool{
	"PASSWORD": true, "PASS": true, "SECRET": true, "TOKEN": true,
	"API_KEY": true, "APIKEY": true, "PRIVATE_KEY": true,
	"ACCESS_KEY": true, "SECRET_KEY": true, "CREDENTIAL": true,
	"AUTH": true, "DB_PASSWORD": true,
}

func (s *Server) handleContainerEnvHealth(w http.ResponseWriter, r *http.Request) {
	result := EnvHealthResult1995{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			if len(c.Env) == 0 && len(c.EnvFrom) == 0 {
				continue
			}
			result.Summary.WithEnv++

			hasSecret := false
			hasConfigMap := false

			for _, env := range c.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.SecretKeyRef != nil {
						hasSecret = true
					}
					if env.ValueFrom.ConfigMapKeyRef != nil {
						hasConfigMap = true
					}
				} else if env.Value != "" {
					// Check for hardcoded sensitive values
					upper := env.Name
					if sensitiveEnvNames1995[upper] {
						result.Summary.Hardcoded++
						result.Issues = append(result.Issues, EnvHealthEntry1995{
							Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
							EnvName: env.Name,
							Issue:   fmt.Sprintf("Sensitive env var '%s' hardcoded — use Secret", env.Name),
						})
						score -= 2
					}
				}
			}
			for _, ef := range c.EnvFrom {
				if ef.SecretRef != nil {
					hasSecret = true
				}
				if ef.ConfigMapRef != nil {
					hasConfigMap = true
				}
			}

			if hasSecret {
				result.Summary.WithSecretRef++
			}
			if hasConfigMap {
				result.Summary.WithConfigMapRef++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with env, %d with Secret ref, %d with CM ref, %d hardcoded sensitive", result.Summary.TotalContainers, result.Summary.WithEnv, result.Summary.WithSecretRef, result.Summary.WithConfigMapRef, result.Summary.Hardcoded))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
