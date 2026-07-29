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
// v20.63 — Operations Dimension (Round 30)
// 1. Pod Ready Latency — time from creation to ready
// 2. Container Restart Velocity — restart rate trend
// 3. Event Noise Filter — high-frequency event detection
// ============================================================

type ReadyLatResult2063 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ReadyLatSummary2063 `json:"summary"`
	SlowPods        []ReadyLatEntry2063 `json:"slowPods"`
	Recommendations []string            `json:"recommendations"`
}

type ReadyLatSummary2063 struct {
	TotalPods int `json:"totalPods"`
	SlowPods  int `json:"slowPods"`
}

type ReadyLatEntry2063 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	StartMins int    `json:"startupMinutes"`
}

func (s *Server) handleReadyLatency2063(w http.ResponseWriter, r *http.Request) {
	result := ReadyLatResult2063{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Status.StartTime == nil {
			continue
		}
		result.Summary.TotalPods++

		startMins := int(now.Sub(pod.Status.StartTime.Time).Minutes())
		if startMins < 5 && pod.Status.ContainerStatuses != nil {
			allReady := true
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					allReady = false
					break
				}
			}
			if !allReady {
				result.Summary.SlowPods++
				result.SlowPods = append(result.SlowPods, ReadyLatEntry2063{
					Pod: pod.Name, Namespace: pod.Namespace, StartMins: startMins,
				})
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.SlowPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods slow to become ready — check probes and image pull", result.Summary.SlowPods))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Restart Velocity
// ---------------------------------------------------------------

type RestartVelResult2063 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         RestartVelSummary2063 `json:"summary"`
	HighVel         []RestartVelEntry2063 `json:"highVelocityPods"`
	Recommendations []string              `json:"recommendations"`
}

type RestartVelSummary2063 struct {
	TotalPods    int `json:"totalPods"`
	HighVelocity int `json:"highVelocityPods"`
}

type RestartVelEntry2063 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	Restarts  int32   `json:"restarts"`
	Rate      float64 `json:"restartsPerHour"`
}

func (s *Server) handleRestartVelocity(w http.ResponseWriter, r *http.Request) {
	result := RestartVelResult2063{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Status.StartTime == nil {
			continue
		}
		result.Summary.TotalPods++

		var totalRestarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
		}

		if totalRestarts > 0 {
			ageHours := now.Sub(pod.Status.StartTime.Time).Hours()
			if ageHours < 1 {
				ageHours = 1
			}
			rate := float64(totalRestarts) / ageHours

			if rate > 1.0 {
				result.Summary.HighVelocity++
				result.HighVel = append(result.HighVel, RestartVelEntry2063{
					Pod: pod.Name, Namespace: pod.Namespace,
					Restarts: totalRestarts, Rate: rate,
				})
				score -= 3
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.HighVel, func(i, j int) bool { return result.HighVel[i].Rate > result.HighVel[j].Rate })

	if result.Summary.HighVelocity > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have high restart velocity (>1/hr)", result.Summary.HighVelocity))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Event Noise Filter
// ---------------------------------------------------------------

type EventNoiseResult2063 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EventNoiseSummary2063 `json:"summary"`
	NoisyReasons    []EventNoiseEntry2063 `json:"noisyReasons"`
	Recommendations []string              `json:"recommendations"`
}

type EventNoiseSummary2063 struct {
	TotalEvents  int `json:"totalEvents"`
	NoisyReasons int `json:"noisyReasons"`
}

type EventNoiseEntry2063 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

func (s *Server) handleEventNoiseFilter2063(w http.ResponseWriter, r *http.Request) {
	result := EventNoiseResult2063{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	reasonCount := make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		reason := evt.Reason
		if reason == "" {
			reason = "Unknown"
		}
		reasonCount[reason]++
	}

	for reason, count := range reasonCount {
		if count > 50 {
			result.Summary.NoisyReasons++
			result.NoisyReasons = append(result.NoisyReasons, EventNoiseEntry2063{
				Reason: reason, Count: count,
			})
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NoisyReasons, func(i, j int) bool { return result.NoisyReasons[i].Count > result.NoisyReasons[j].Count })

	if result.Summary.NoisyReasons > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d noisy event reasons (>50 occurrences) — investigate root causes", result.Summary.NoisyReasons))
	}
	writeJSON(w, result)
}
