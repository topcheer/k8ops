package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.07 — Deployment Dimension (Round 21)
// 1. Pod Ephemeral Storage Audit — ephemeral storage request/limit compliance
// 2. Deployment Condition History — condition transition tracking
// 3. Container Resources Gap — request vs limit vs recommendation
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Ephemeral Storage Audit
// ---------------------------------------------------------------

type EphStoreResult2007 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         EphStoreSummary2007 `json:"summary"`
	Issues          []EphStoreEntry2007 `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

type EphStoreSummary2007 struct {
	TotalContainers int `json:"totalContainers"`
	WithEphLimit    int `json:"withEphemeralLimit"`
	WithEphRequest  int `json:"withEphemeralRequest"`
	WithoutLimit    int `json:"withoutEphemeralLimit"`
}

type EphStoreEntry2007 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Issue     string `json:"issue"`
}

func (s *Server) handlePodEphStorage(w http.ResponseWriter, r *http.Request) {
	result := EphStoreResult2007{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasLimit := false
			hasRequest := false
			if c.Resources.Limits != nil {
				if _, ok := c.Resources.Limits[corev1.ResourceEphemeralStorage]; ok {
					hasLimit = true
				}
			}
			if c.Resources.Requests != nil {
				if _, ok := c.Resources.Requests[corev1.ResourceEphemeralStorage]; ok {
					hasRequest = true
				}
			}

			if hasLimit {
				result.Summary.WithEphLimit++
			} else {
				result.Summary.WithoutLimit++
				result.Issues = append(result.Issues, EphStoreEntry2007{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					Issue: "No ephemeral storage limit — risk of disk exhaustion",
				})
				score -= 1
			}
			if hasRequest {
				result.Summary.WithEphRequest++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with eph limit, %d with request, %d without", result.Summary.TotalContainers, result.Summary.WithEphLimit, result.Summary.WithEphRequest, result.Summary.WithoutLimit))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Deployment Condition History
// ---------------------------------------------------------------

type CondHistResult2007 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CondHistSummary2007 `json:"summary"`
	Deployments     []CondHistEntry2007 `json:"deployments"`
	Recommendations []string            `json:"recommendations"`
}

type CondHistSummary2007 struct {
	TotalDeployments int `json:"totalDeployments"`
	WithAvailable    int `json:"withAvailableCondition"`
	WithProgressing  int `json:"withProgressingCondition"`
	WithReplicaFail  int `json:"withReplicaFailure"`
}

type CondHistEntry2007 struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Available      bool   `json:"available"`
	Progressing    bool   `json:"progressing"`
	ReplicaFailure bool   `json:"replicaFailure"`
}

func (s *Server) handleDeployCondHist(w http.ResponseWriter, r *http.Request) {
	result := CondHistResult2007{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		entry := CondHistEntry2007{
			Name: dep.Name, Namespace: dep.Namespace,
		}

		for _, cond := range dep.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case appsv1.DeploymentAvailable:
				entry.Available = true
				result.Summary.WithAvailable++
			case appsv1.DeploymentProgressing:
				entry.Progressing = true
				result.Summary.WithProgressing++
			case appsv1.DeploymentReplicaFailure:
				entry.ReplicaFailure = true
				result.Summary.WithReplicaFail++
				score -= 3
			}
		}

		result.Deployments = append(result.Deployments, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d available, %d progressing, %d replica-failure", result.Summary.TotalDeployments, result.Summary.WithAvailable, result.Summary.WithProgressing, result.Summary.WithReplicaFail))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Resources Gap
// ---------------------------------------------------------------

type ResGapResult2007 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         ResGapSummary2007 `json:"summary"`
	Containers      []ResGapEntry2007 `json:"containers"`
	Recommendations []string          `json:"recommendations"`
}

type ResGapSummary2007 struct {
	TotalContainers int `json:"totalContainers"`
	WithRequest     int `json:"withResourceRequest"`
	WithLimit       int `json:"withResourceLimit"`
	WithoutRequest  int `json:"withoutRequest"`
	WithoutLimit    int `json:"withoutLimit"`
}

type ResGapEntry2007 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	HasReq    bool   `json:"hasRequest"`
	HasLimit  bool   `json:"hasLimit"`
}

func (s *Server) handleContainerResGap(w http.ResponseWriter, r *http.Request) {
	result := ResGapResult2007{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasReq := c.Resources.Requests.Cpu().Value() > 0 || c.Resources.Requests.Memory().Value() > 0
			hasLimit := c.Resources.Limits.Cpu().Value() > 0 || c.Resources.Limits.Memory().Value() > 0

			entry := ResGapEntry2007{
				Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				HasReq: hasReq, HasLimit: hasLimit,
			}

			if hasReq {
				result.Summary.WithRequest++
			} else {
				result.Summary.WithoutRequest++
				score -= 2
			}
			if hasLimit {
				result.Summary.WithLimit++
			} else {
				result.Summary.WithoutLimit++
			}

			if !hasReq || !hasLimit {
				result.Containers = append(result.Containers, entry)
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with request, %d with limit, %d without request, %d without limit", result.Summary.TotalContainers, result.Summary.WithRequest, result.Summary.WithLimit, result.Summary.WithoutRequest, result.Summary.WithoutLimit))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
