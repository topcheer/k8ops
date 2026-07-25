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
// v19.84 — Operations Dimension (Round 17)
// 1. Pod Probe Latency — readiness/liveness probe configuration analysis
// 2. Image Pull Duration — image pull time estimator from size & layers
// 3. Config Reload Health — ConfigMap/Secret mount reload tracking
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Probe Latency
// ---------------------------------------------------------------

type ProbeLatResult1984 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ProbeLatSummary1984 `json:"summary"`
	Issues          []ProbeLatEntry1984 `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

type ProbeLatSummary1984 struct {
	TotalContainers int `json:"totalContainers"`
	WithLiveness    int `json:"withLivenessProbe"`
	WithReadiness   int `json:"withReadinessProbe"`
	WithStartup     int `json:"withStartupProbe"`
	WithoutProbes   int `json:"withoutProbes"`
}

type ProbeLatEntry1984 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Issue     string `json:"issue"`
}

func (s *Server) handlePodProbeLatency(w http.ResponseWriter, r *http.Request) {
	result := ProbeLatResult1984{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasLiveness := c.LivenessProbe != nil
			hasReadiness := c.ReadinessProbe != nil
			hasStartup := c.StartupProbe != nil

			if hasLiveness {
				result.Summary.WithLiveness++
			}
			if hasReadiness {
				result.Summary.WithReadiness++
			}
			if hasStartup {
				result.Summary.WithStartup++
			}
			if !hasLiveness && !hasReadiness {
				result.Summary.WithoutProbes++
				result.Issues = append(result.Issues, ProbeLatEntry1984{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					Issue: "No liveness or readiness probe configured",
				})
				score -= 2
			}

			// Check aggressive probe intervals
			if hasLiveness && c.LivenessProbe.PeriodSeconds < 10 {
				result.Issues = append(result.Issues, ProbeLatEntry1984{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					Issue: fmt.Sprintf("Liveness probe too frequent: %ds interval", c.LivenessProbe.PeriodSeconds),
				})
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d liveness, %d readiness, %d startup, %d without probes", result.Summary.TotalContainers, result.Summary.WithLiveness, result.Summary.WithReadiness, result.Summary.WithStartup, result.Summary.WithoutProbes))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Image Pull Duration
// ---------------------------------------------------------------

type ImgPullResult1984 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         ImgPullSummary1984 `json:"summary"`
	SlowImages      []ImgPullEntry1984 `json:"slowImages"`
	Recommendations []string           `json:"recommendations"`
}

type ImgPullSummary1984 struct {
	TotalImages    int     `json:"totalUniqueImages"`
	AvgPullTimeSec float64 `json:"avgPullTimeSec"`
	WithPullPolicy int     `json:"withExplicitPullPolicy"`
	UsingAlways    int     `json:"usingAlwaysPullPolicy"`
}

type ImgPullEntry1984 struct {
	Image      string  `json:"image"`
	EstPullSec float64 `json:"estPullSec"`
	PullPolicy string  `json:"pullPolicy"`
}

func (s *Server) handleImagePullDuration(w http.ResponseWriter, r *http.Request) {
	result := ImgPullResult1984{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	imageMap := make(map[string]*ImgPullEntry1984)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			entry, ok := imageMap[img]
			if !ok {
				// Estimate pull time: ~1s per 10MB of typical image
				estSec := 5.0 // base
				// Heuristic: images with more tags/layers take longer
				estSec += float64(len(img)) * 0.1
				entry = &ImgPullEntry1984{
					Image: img, EstPullSec: estSec,
				}
				imageMap[img] = entry
				result.Summary.TotalImages++
			}

			policy := string(c.ImagePullPolicy)
			if policy != "" {
				result.Summary.WithPullPolicy++
				entry.PullPolicy = policy
				if policy == "Always" {
					result.Summary.UsingAlways++
					score -= 1
				}
			}
		}
	}

	var totalPull float64
	for _, e := range imageMap {
		totalPull += e.EstPullSec
		if e.EstPullSec > 30 {
			result.SlowImages = append(result.SlowImages, *e)
		}
	}
	if result.Summary.TotalImages > 0 {
		result.Summary.AvgPullTimeSec = totalPull / float64(result.Summary.TotalImages)
	}

	sort.Slice(result.SlowImages, func(i, j int) bool {
		return result.SlowImages[i].EstPullSec > result.SlowImages[j].EstPullSec
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images, avg pull ~%.0fs, %d with Always policy", result.Summary.TotalImages, result.Summary.AvgPullTimeSec, result.Summary.UsingAlways))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Config Reload Health
// ---------------------------------------------------------------

type ConfigReloadResult1984 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ConfigReloadSummary1984 `json:"summary"`
	Pods            []ConfigReloadEntry1984 `json:"pods"`
	Recommendations []string                `json:"recommendations"`
}

type ConfigReloadSummary1984 struct {
	TotalPods         int `json:"totalPods"`
	PodsWithCMRef     int `json:"podsWithConfigMapRef"`
	PodsWithSecretRef int `json:"podsWithSecretRef"`
	WithReloader      int `json:"withReloaderAnnotation"`
	WithoutReloader   int `json:"withoutReloaderAnnotation"`
}

type ConfigReloadEntry1984 struct {
	Pod         string `json:"pod"`
	Namespace   string `json:"namespace"`
	HasCM       bool   `json:"hasConfigMap"`
	HasSecret   bool   `json:"hasSecret"`
	HasReloader bool   `json:"hasReloader"`
}

func (s *Server) handleConfigReloadHealth(w http.ResponseWriter, r *http.Request) {
	result := ConfigReloadResult1984{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasCM := false
		hasSecret := false
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				hasCM = true
			}
			if vol.Secret != nil {
				hasSecret = true
			}
		}
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.ConfigMapKeyRef != nil {
						hasCM = true
					}
					if env.ValueFrom.SecretKeyRef != nil {
						hasSecret = true
					}
				}
			}
		}

		// Check for reloader annotations
		hasReloader := false
		for k := range pod.Annotations {
			if k == "reloader.stakater.com/auto" || k == "configmap.reloader.stakater.com/reload" {
				hasReloader = true
				break
			}
		}

		entry := ConfigReloadEntry1984{
			Pod: pod.Name, Namespace: pod.Namespace,
			HasCM: hasCM, HasSecret: hasSecret, HasReloader: hasReloader,
		}

		if hasCM {
			result.Summary.PodsWithCMRef++
		}
		if hasSecret {
			result.Summary.PodsWithSecretRef++
		}
		if hasReloader {
			result.Summary.WithReloader++
		}
		if (hasCM || hasSecret) && !hasReloader {
			result.Summary.WithoutReloader++
			score -= 1
		}

		result.Pods = append(result.Pods, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with CM, %d with Secret, %d with reloader, %d without", result.Summary.TotalPods, result.Summary.PodsWithCMRef, result.Summary.PodsWithSecretRef, result.Summary.WithReloader, result.Summary.WithoutReloader))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
