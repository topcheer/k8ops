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
// v21.16 — Deployment Dimension (Round 39)
// 1. Container Probe Timeout Analysis
// 2. Pod Init Container Resource Audit
// 3. Deployment MaxUnavailable Validator
// ============================================================

type ProbeTimeoutResult2116 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ProbeTimeoutSummary2116 `json:"summary"`
	NoProbe         []ProbeTimeoutEntry2116 `json:"noProbeContainers"`
	Recommendations []string                `json:"recommendations"`
}

type ProbeTimeoutSummary2116 struct {
	TotalContainers int `json:"totalContainers"`
	WithLiveness    int `json:"withLivenessProbe"`
	WithReadiness   int `json:"withReadinessProbe"`
}

type ProbeTimeoutEntry2116 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleProbeTimeout2116(w http.ResponseWriter, r *http.Request) {
	result := ProbeTimeoutResult2116{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.LivenessProbe != nil {
				result.Summary.WithLiveness++
			}
			if c.ReadinessProbe != nil {
				result.Summary.WithReadiness++
			}
			if c.LivenessProbe == nil && c.ReadinessProbe == nil {
				result.NoProbe = append(result.NoProbe, ProbeTimeoutEntry2116{Pod: pod.Name, Namespace: pod.Namespace})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NoProbe, func(i, j int) bool { return result.NoProbe[i].Namespace < result.NoProbe[j].Namespace })
	writeJSON(w, result)
}

// 2. Init Container Resource
type InitResResult2116 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         InitResSummary2116 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type InitResSummary2116 struct {
	TotalPods   int `json:"totalPods"`
	WithInit    int `json:"withInitContainers"`
	InitWithRes int `json:"initWithResources"`
}

func (s *Server) handleInitRes2116(w http.ResponseWriter, r *http.Request) {
	result := InitResResult2116{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.InitContainers) > 0 {
			result.Summary.WithInit++
			for _, ic := range pod.Spec.InitContainers {
				if !ic.Resources.Requests.Cpu().IsZero() {
					result.Summary.InitWithRes++
					break
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. MaxUnavailable Validator
type MaxUnavailResult2116 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         MaxUnavailSummary2116 `json:"summary"`
	Risky           []MaxUnavailEntry2116 `json:"riskyDeployments"`
	Recommendations []string              `json:"recommendations"`
}

type MaxUnavailSummary2116 struct {
	TotalDeploys int `json:"totalDeployments"`
	RiskyCount   int `json:"riskyCount"`
}

type MaxUnavailEntry2116 struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	MaxUnavailStr string `json:"maxUnavailable"`
}

func (s *Server) handleMaxUnavail2116(w http.ResponseWriter, r *http.Request) {
	result := MaxUnavailResult2116{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if dep.Spec.Strategy.RollingUpdate != nil && dep.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
			mu := dep.Spec.Strategy.RollingUpdate.MaxUnavailable
			if mu.IntVal == 0 && mu.Type == 0 {
				// 0 maxUnavailable is fine
			} else if mu.IntVal > 1 {
				result.Summary.RiskyCount++
				result.Risky = append(result.Risky, MaxUnavailEntry2116{Name: dep.Name, Namespace: dep.Namespace, MaxUnavailStr: fmt.Sprintf("%d", mu.IntVal)})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Risky, func(i, j int) bool { return result.Risky[i].Namespace < result.Risky[j].Namespace })
	writeJSON(w, result)
}
