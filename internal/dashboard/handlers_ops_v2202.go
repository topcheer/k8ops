package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.02 — Operations Dimension (Round 53)
// 1. Pod Container State Terminal Distribution
// 2. Node PID Pressure Detector
// 3. Service Endpoint Slice Ready Ratio
// ============================================================

type TerminalDistResult2202 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int         `json:"totalContainers"`
		Terminal        int         `json:"terminalState"`
		ByExitCode      map[int]int `json:"byExitCode"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleTerminalDist2202(w http.ResponseWriter, r *http.Request) {
	result := TerminalDistResult2202{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByExitCode = make(map[int]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Terminated != nil {
				result.Summary.Terminal++
				result.Summary.ByExitCode[int(cs.State.Terminated.ExitCode)]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node PID Pressure
type PIDPressureResult2202 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes      int `json:"totalNodes"`
		WithPIDPressure int `json:"withPIDPressure"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePIDPressure2202(w http.ResponseWriter, r *http.Request) {
	result := PIDPressureResult2202{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue {
				result.Summary.WithPIDPressure++
				score -= 10
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Endpoint Slice Ready Ratio
type EPSReadyRatioResult2202 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSlices    int `json:"totalSlices"`
		TotalEndpoints int `json:"totalEndpoints"`
		ReadyEndpoints int `json:"readyEndpoints"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEPSReadyRatio2202(w http.ResponseWriter, r *http.Request) {
	result := EPSReadyRatioResult2202{ScannedAt: time.Now()}
	score := 100
	epList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	for _, eps := range epList.Items {
		result.Summary.TotalSlices++
		for _, ep := range eps.Endpoints {
			result.Summary.TotalEndpoints += len(ep.Addresses)
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				result.Summary.ReadyEndpoints += len(ep.Addresses)
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
