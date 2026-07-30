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
// v21.46 — Deployment Dimension (Round 44)
// 1. Pod Ephemeral Container Audit
// 2. Deployment Min Ready Seconds Validator
// 3. Container Image Pull Secrets Coverage
// ============================================================

type EphemeralResult2146 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         EphemeralSummary2146 `json:"summary"`
	WithEphemeral   []EphemeralEntry2146 `json:"withEphemeral"`
	Recommendations []string             `json:"recommendations"`
}

type EphemeralSummary2146 struct {
	TotalPods     int `json:"totalPods"`
	WithEphemeral int `json:"withEphemeral"`
}

type EphemeralEntry2146 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleEphemeral2146(w http.ResponseWriter, r *http.Request) {
	result := EphemeralResult2146{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.EphemeralContainers) > 0 {
			result.Summary.WithEphemeral++
			result.WithEphemeral = append(result.WithEphemeral, EphemeralEntry2146{Pod: pod.Name, Namespace: pod.Namespace})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.WithEphemeral, func(i, j int) bool { return result.WithEphemeral[i].Namespace < result.WithEphemeral[j].Namespace })
	writeJSON(w, result)
}

// 2. Min Ready Seconds
type MinReadyResult2146 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         MinReadySummary2146 `json:"summary"`
	Custom          []MinReadyEntry2146 `json:"customMinReady"`
	Recommendations []string            `json:"recommendations"`
}

type MinReadySummary2146 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithCustom   int `json:"withCustomMinReadySeconds"`
}

type MinReadyEntry2146 struct {
	Name            string `json:"name"`
	MinReadySeconds int32  `json:"minReadySeconds"`
}

func (s *Server) handleMinReady2146(w http.ResponseWriter, r *http.Request) {
	result := MinReadyResult2146{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.MinReadySeconds > 0 {
			result.Summary.WithCustom++
			result.Custom = append(result.Custom, MinReadyEntry2146{Name: dep.Name, MinReadySeconds: dep.Spec.MinReadySeconds})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Custom, func(i, j int) bool { return result.Custom[i].MinReadySeconds > result.Custom[j].MinReadySeconds })
	writeJSON(w, result)
}

// 3. ImagePullSecrets Coverage
type PullSecretCovResult2146 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         PullSecretCovSummary2146 `json:"summary"`
	Missing         []PullSecretCovEntry2146 `json:"missingPullSecrets"`
	Recommendations []string                 `json:"recommendations"`
}

type PullSecretCovSummary2146 struct {
	TotalPods  int `json:"totalPods"`
	WithSecret int `json:"withImagePullSecrets"`
}

type PullSecretCovEntry2146 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePullSecretCov2146(w http.ResponseWriter, r *http.Request) {
	result := PullSecretCovResult2146{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.ImagePullSecrets) > 0 {
			result.Summary.WithSecret++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WithSecret == 0 && result.Summary.TotalPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("No pods with imagePullSecrets — add for private registries"))
	}
	writeJSON(w, result)
}
