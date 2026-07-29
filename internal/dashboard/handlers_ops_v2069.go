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
// v20.69 — Operations Dimension (Round 31)
// 1. Container CPU Saturation — CPU limit vs request saturation
// 2. Memory Working Set Estimate — memory usage estimation
// 3. Pod Startup Phase Tracker — init/main container timing
// ============================================================

type CPUSatResult2069 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         CPUSatSummary2069 `json:"summary"`
	Saturated       []CPUSatEntry2069 `json:"saturatedContainers"`
	Recommendations []string          `json:"recommendations"`
}

type CPUSatSummary2069 struct {
	TotalContainers int `json:"totalContainers"`
	Saturated       int `json:"saturated"`
	NoLimits        int `json:"noLimits"`
}

type CPUSatEntry2069 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	CPUReq    float64 `json:"cpuRequestCores"`
	CPULim    float64 `json:"cpuLimitCores"`
}

func (s *Server) handleCPUSaturation(w http.ResponseWriter, r *http.Request) {
	result := CPUSatResult2069{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			req := c.Resources.Requests.Cpu()
			lim := c.Resources.Limits.Cpu()
			if lim.IsZero() {
				result.Summary.NoLimits++
				continue
			}
			reqVal := req.AsApproximateFloat64()
			limVal := lim.AsApproximateFloat64()
			if limVal > 0 && reqVal/limVal > 0.9 {
				result.Summary.Saturated++
				result.Saturated = append(result.Saturated, CPUSatEntry2069{
					Pod: pod.Name, Namespace: pod.Namespace, CPUReq: reqVal, CPULim: limVal,
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
	sort.Slice(result.Saturated, func(i, j int) bool { return result.Saturated[i].CPUReq > result.Saturated[j].CPUReq })

	if result.Summary.Saturated > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers have request/limit >90%% — no headroom for bursts", result.Summary.Saturated))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Memory Working Set Estimate
// ---------------------------------------------------------------

type MemWSetResult2069 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         MemWSetSummary2069 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type MemWSetSummary2069 struct {
	TotalPods    int     `json:"totalPods"`
	TotalMemReq  float64 `json:"totalMemReqGB"`
	TotalMemLim  float64 `json:"totalMemLimGB"`
	AvgMemPerPod float64 `json:"avgMemPerPodGB"`
}

func (s *Server) handleMemWSetEstimate(w http.ResponseWriter, r *http.Request) {
	result := MemWSetResult2069{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			if !c.Resources.Requests.Memory().IsZero() {
				result.Summary.TotalMemReq += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
			}
			if !c.Resources.Limits.Memory().IsZero() {
				result.Summary.TotalMemLim += c.Resources.Limits.Memory().AsApproximateFloat64() / 1e9
			}
		}
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.AvgMemPerPod = result.Summary.TotalMemReq / float64(result.Summary.TotalPods)
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Startup Phase Tracker
// ---------------------------------------------------------------

type StartupPhaseResult2069 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         StartupPhaseSummary2069 `json:"summary"`
	SlowStartup     []StartupPhaseEntry2069 `json:"slowStartupPods"`
	Recommendations []string                `json:"recommendations"`
}

type StartupPhaseSummary2069 struct {
	TotalPods   int `json:"totalPods"`
	WithInit    int `json:"withInitContainers"`
	SlowStartup int `json:"slowStartup"`
}

type StartupPhaseEntry2069 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleStartupPhaseTracker(w http.ResponseWriter, r *http.Request) {
	result := StartupPhaseResult2069{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if len(pod.Spec.InitContainers) > 0 {
			result.Summary.WithInit++
		}

		// Check if pod recently started and is still not fully ready
		if pod.Status.StartTime != nil {
			ageMin := now.Sub(pod.Status.StartTime.Time).Minutes()
			if ageMin < 2 {
				allReady := true
				for _, cs := range pod.Status.ContainerStatuses {
					if !cs.Ready {
						allReady = false
						break
					}
				}
				if !allReady {
					result.Summary.SlowStartup++
					result.SlowStartup = append(result.SlowStartup, StartupPhaseEntry2069{
						Pod: pod.Name, Namespace: pod.Namespace,
					})
					score -= 1
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.SlowStartup > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods slow to start — check init containers and probes", result.Summary.SlowStartup))
	}
	writeJSON(w, result)
}
