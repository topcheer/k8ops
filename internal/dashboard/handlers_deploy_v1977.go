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

// ============================================================
// v19.77 — Deployment Dimension (Round 16)
// 1. Init Container Dependency — ordering & failure impact analysis
// 2. Strategy Compliance — RollingUpdate vs Recreate configuration audit
// 3. Pull Secret Coverage — namespace image pull secret gap analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Init Container Dependency
// ---------------------------------------------------------------

type InitDepResult1977 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         InitDepSummary1977 `json:"summary"`
	PodsWithInit    []InitDepEntry1977 `json:"podsWithInit"`
	Recommendations []string           `json:"recommendations"`
}

type InitDepSummary1977 struct {
	TotalPods     int    `json:"totalPods"`
	PodsWithInit  int    `json:"podsWithInitContainers"`
	TotalInitCtrs int    `json:"totalInitContainers"`
	MaxInitCount  int    `json:"maxInitContainersInPod"`
	AvgInitTime   string `json:"estAvgInitTime"`
}

type InitDepEntry1977 struct {
	Pod         string   `json:"pod"`
	Namespace   string   `json:"namespace"`
	InitCount   int      `json:"initContainerCount"`
	Names       []string `json:"initContainerNames"`
	HasResource bool     `json:"hasResourceLimits"`
}

func (s *Server) handleInitContainerDep(w http.ResponseWriter, r *http.Request) {
	result := InitDepResult1977{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalInit int
	var maxInit int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		initCount := len(pod.Spec.InitContainers)
		if initCount == 0 {
			continue
		}

		result.Summary.PodsWithInit++
		totalInit += initCount
		if initCount > maxInit {
			maxInit = initCount
		}

		entry := InitDepEntry1977{
			Pod: pod.Name, Namespace: pod.Namespace,
			InitCount: initCount,
		}

		for _, ic := range pod.Spec.InitContainers {
			entry.Names = append(entry.Names, ic.Name)
			if !ic.Resources.Limits.Cpu().IsZero() {
				entry.HasResource = true
			}
		}

		if !entry.HasResource {
			score -= 2
		}
		if initCount > 5 {
			score -= 3
		}

		result.PodsWithInit = append(result.PodsWithInit, entry)
	}

	result.Summary.TotalInitCtrs = totalInit
	result.Summary.MaxInitCount = maxInit
	result.Summary.AvgInitTime = "~5-30s per container (estimated)"

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with init containers, %d total init containers, max %d per pod", result.Summary.PodsWithInit, totalInit, maxInit))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Strategy Compliance
// ---------------------------------------------------------------

type StrategyCompResult1977 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         StrategyCompSummary1977 `json:"summary"`
	Deployments     []StrategyCompEntry1977 `json:"deployments"`
	Issues          []StrategyCompIssue1977 `json:"issues"`
	Recommendations []string                `json:"recommendations"`
}

type StrategyCompSummary1977 struct {
	TotalDeployments   int `json:"totalDeployments"`
	RollingUpdate      int `json:"rollingUpdateStrategy"`
	Recreate           int `json:"recreateStrategy"`
	WithMaxSurge       int `json:"withMaxSurge"`
	WithMaxUnavailable int `json:"withMaxUnavailable"`
	NoStrategy         int `json:"withoutExplicitStrategy"`
}

type StrategyCompEntry1977 struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Strategy       string `json:"strategyType"`
	MaxSurge       string `json:"maxSurge"`
	MaxUnavailable string `json:"maxUnavailable"`
}

type StrategyCompIssue1977 struct {
	Name     string `json:"name"`
	Issue    string `json:"issue"`
	Severity string `json:"severity"`
}

func (s *Server) handleStrategyCompliance(w http.ResponseWriter, r *http.Request) {
	result := StrategyCompResult1977{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		entry := StrategyCompEntry1977{
			Name: dep.Name, Namespace: dep.Namespace,
			Strategy: string(dep.Spec.Strategy.Type),
			MaxSurge: "25%", MaxUnavailable: "25%",
		}

		if dep.Spec.Strategy.Type == "" {
			entry.Strategy = "RollingUpdate (default)"
			result.Summary.NoStrategy++
		}

		if dep.Spec.Strategy.Type == appsv1.RecreateDeploymentStrategyType {
			result.Summary.Recreate++
			result.Issues = append(result.Issues, StrategyCompIssue1977{
				Name: dep.Name, Issue: "Using Recreate strategy — causes downtime during updates",
				Severity: "medium",
			})
			score -= 2
		} else {
			result.Summary.RollingUpdate++
		}

		if dep.Spec.Strategy.RollingUpdate != nil {
			ru := dep.Spec.Strategy.RollingUpdate
			if ru.MaxSurge != nil {
				entry.MaxSurge = ru.MaxSurge.String()
				result.Summary.WithMaxSurge++
			}
			if ru.MaxUnavailable != nil {
				entry.MaxUnavailable = ru.MaxUnavailable.String()
				result.Summary.WithMaxUnavailable++
			}
			// High maxUnavailable = risky
			if ru.MaxUnavailable != nil && ru.MaxUnavailable.IntVal > 1 {
				result.Issues = append(result.Issues, StrategyCompIssue1977{
					Name: dep.Name, Issue: fmt.Sprintf("maxUnavailable=%s — aggressive rollout", entry.MaxUnavailable),
					Severity: "low",
				})
			}
		}

		result.Deployments = append(result.Deployments, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d RollingUpdate, %d Recreate, %d without explicit strategy", result.Summary.TotalDeployments, result.Summary.RollingUpdate, result.Summary.Recreate, result.Summary.NoStrategy))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pull Secret Coverage
// ---------------------------------------------------------------

type PullSecretResult1977 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PullSecretSummary1977   `json:"summary"`
	GapNamespaces   []PullSecretNSEntry1977 `json:"gapNamespaces"`
	Recommendations []string                `json:"recommendations"`
}

type PullSecretSummary1977 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithPullSecret  int `json:"namespacesWithPullSecret"`
	WithoutSecret   int `json:"namespacesWithoutPullSecret"`
	TotalSecrets    int `json:"totalImagePullSecrets"`
	UsingPrivateReg int `json:"namespacesUsingPrivateRegistry"`
}

type PullSecretNSEntry1977 struct {
	Namespace        string `json:"namespace"`
	HasSecret        bool   `json:"hasPullSecret"`
	PrivateRegImages int    `json:"privateRegistryImageCount"`
}

func (s *Server) handlePullSecretCoverage(w http.ResponseWriter, r *http.Request) {
	result := PullSecretResult1977{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build secret map: ns -> has docker-registry secret
	nsWithSecret := make(map[string]bool)
	for _, sec := range secretList.Items {
		if sec.Type == corev1.SecretTypeDockerConfigJson || sec.Type == corev1.SecretTypeDockercfg {
			nsWithSecret[sec.Namespace] = true
			result.Summary.TotalSecrets++
		}
	}

	// Detect private registry images per namespace
	nsPrivateReg := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			// Check if using private registry (not docker hub public)
			if !strings.Contains(img, "docker.io/") && !strings.HasPrefix(img, "library/") &&
				strings.Contains(img, ".") && !strings.Contains(img, "gcr.io/") &&
				!strings.Contains(img, "quay.io/") && !strings.Contains(img, "ghcr.io/") {
				nsPrivateReg[pod.Namespace]++
			}
		}
	}

	result.Summary.TotalNamespaces = len(nsList.Items)
	for _, ns := range nsList.Items {
		entry := PullSecretNSEntry1977{
			Namespace:        ns.Name,
			HasSecret:        nsWithSecret[ns.Name],
			PrivateRegImages: nsPrivateReg[ns.Name],
		}

		if entry.HasSecret {
			result.Summary.WithPullSecret++
			if entry.PrivateRegImages > 0 {
				result.Summary.UsingPrivateReg++
			}
		} else {
			result.Summary.WithoutSecret++
			if entry.PrivateRegImages > 0 {
				result.GapNamespaces = append(result.GapNamespaces, entry)
				score -= 3
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces: %d with pull secret, %d without, %d using private registry", result.Summary.TotalNamespaces, result.Summary.WithPullSecret, result.Summary.WithoutSecret, result.Summary.UsingPrivateReg))
	if len(result.GapNamespaces) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces using private registry without pull secret", len(result.GapNamespaces)))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
