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
// v20.38 — Deployment Dimension (Round 26)
// 1. Rollout Window Analyzer — deployment rollout timing analysis
// 2. Init Container Dependency Map — init container chain analysis
// 3. Probe Configuration Validator — readiness/liveness probe best practices
// ============================================================

// ---------------------------------------------------------------
// 1. Rollout Window Analyzer
// ---------------------------------------------------------------

type RolloutWindowResult2038 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         RolloutWindowSummary2038 `json:"summary"`
	SlowRollouts    []RolloutWindowEntry2038 `json:"slowRollouts"`
	Recommendations []string                 `json:"recommendations"`
}

type RolloutWindowSummary2038 struct {
	TotalDeploys   int `json:"totalDeployments"`
	WithConditions int `json:"withConditions"`
	Progressing    int `json:"progressing"`
	Available      int `json:"available"`
}

type RolloutWindowEntry2038 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	Updated   int32  `json:"updatedReplicas"`
	Ready     int32  `json:"readyReplicas"`
}

func (s *Server) handleRolloutWindow(w http.ResponseWriter, r *http.Request) {
	result := RolloutWindowResult2038{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++

		replicas := int32(0)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas == 0 {
			replicas = 1
		}

		ready := dep.Status.ReadyReplicas
		updated := dep.Status.UpdatedReplicas

		for _, cond := range dep.Status.Conditions {
			if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionTrue {
				result.Summary.Progressing++
				result.Summary.WithConditions++
			}
			if cond.Type == appsv1.DeploymentAvailable {
				if cond.Status == corev1.ConditionTrue {
					result.Summary.Available++
				}
				result.Summary.WithConditions++
			}
		}

		// Slow rollout: updated < replicas or ready < replicas
		if updated < replicas || (ready < replicas && replicas > 0) {
			result.SlowRollouts = append(result.SlowRollouts, RolloutWindowEntry2038{
				Name: dep.Name, Namespace: dep.Namespace,
				Replicas: replicas, Updated: updated, Ready: ready,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.SlowRollouts, func(i, j int) bool {
		return result.SlowRollouts[i].Namespace < result.SlowRollouts[j].Namespace
	})

	if result.Summary.Progressing > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments are still progressing — check rollout status", result.Summary.Progressing))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Init Container Dependency Map
// ---------------------------------------------------------------

type InitContainerResult2038 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         InitContainerSummary2038 `json:"summary"`
	HeavyInit       []InitContainerEntry2038 `json:"heavyInitContainers"`
	Recommendations []string                 `json:"recommendations"`
}

type InitContainerSummary2038 struct {
	TotalPods      int `json:"totalPods"`
	PodsWithInit   int `json:"podsWithInit"`
	TotalInitCtnrs int `json:"totalInitContainers"`
	HeavyInitCtnrs int `json:"heavyInitContainers"`
}

type InitContainerEntry2038 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleInitContainerMap(w http.ResponseWriter, r *http.Request) {
	result := InitContainerResult2038{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		initCtnrs := pod.Spec.InitContainers
		if len(initCtnrs) > 0 {
			result.Summary.PodsWithInit++
			result.Summary.TotalInitCtnrs += len(initCtnrs)

			for _, ic := range initCtnrs {
				// Flag init containers with high resource requests
				if !ic.Resources.Requests.Cpu().IsZero() {
					cpuReq := ic.Resources.Requests.Cpu().AsApproximateFloat64()
					if cpuReq > 1.0 {
						result.Summary.HeavyInitCtnrs++
						result.HeavyInit = append(result.HeavyInit, InitContainerEntry2038{
							Pod: pod.Name, Namespace: pod.Namespace, Container: ic.Name,
						})
						score -= 1
					}
				}
				// Flag too many init containers
				if len(initCtnrs) > 3 {
					result.Summary.HeavyInitCtnrs++
					result.HeavyInit = append(result.HeavyInit, InitContainerEntry2038{
						Pod: pod.Name, Namespace: pod.Namespace, Container: ic.Name,
					})
					score -= 1
					break
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.HeavyInit, func(i, j int) bool {
		return result.HeavyInit[i].Namespace < result.HeavyInit[j].Namespace
	})

	if result.Summary.HeavyInitCtnrs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d init containers are resource-heavy — optimize init chain for faster startup", result.Summary.HeavyInitCtnrs))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Probe Configuration Validator
// ---------------------------------------------------------------

type ProbeConfigResult2038 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ProbeConfigSummary2038 `json:"summary"`
	MissingProbes   []ProbeConfigEntry2038 `json:"missingProbes"`
	Recommendations []string               `json:"recommendations"`
}

type ProbeConfigSummary2038 struct {
	TotalContainers int `json:"totalContainers"`
	WithLiveness    int `json:"withLiveness"`
	WithReadiness   int `json:"withReadiness"`
	NoProbes        int `json:"noProbes"`
}

type ProbeConfigEntry2038 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Missing   string `json:"missing"`
}

func (s *Server) handleProbeConfigValidator(w http.ResponseWriter, r *http.Request) {
	result := ProbeConfigResult2038{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++

			hasLiveness := c.LivenessProbe != nil
			hasReadiness := c.ReadinessProbe != nil

			if hasLiveness {
				result.Summary.WithLiveness++
			}
			if hasReadiness {
				result.Summary.WithReadiness++
			}

			if !hasLiveness && !hasReadiness {
				result.Summary.NoProbes++
				result.MissingProbes = append(result.MissingProbes, ProbeConfigEntry2038{
					Pod: dep.Name, Namespace: dep.Namespace,
					Container: c.Name, Missing: "liveness+readiness",
				})
				score -= 2
			} else if !hasReadiness {
				result.Summary.NoProbes++
				result.MissingProbes = append(result.MissingProbes, ProbeConfigEntry2038{
					Pod: dep.Name, Namespace: dep.Namespace,
					Container: c.Name, Missing: "readiness",
				})
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.MissingProbes, func(i, j int) bool {
		return result.MissingProbes[i].Namespace < result.MissingProbes[j].Namespace
	})

	if result.Summary.NoProbes > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers missing probes — add readiness/liveness probes for reliability", result.Summary.NoProbes))
	}

	writeJSON(w, result)
}
