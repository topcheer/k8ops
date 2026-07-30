package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.47 — Operations Dimension (Round 44)
// 1. Node Unschedulable Status Tracker
// 2. Pod Container Status Ready Ratio
// 3. Event Last Timestamp Freshness
// ============================================================

type UnschedResult2147 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         UnschedSummary2147 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type UnschedSummary2147 struct {
	TotalNodes    int `json:"totalNodes"`
	Schedulable   int `json:"schedulable"`
	Unschedulable int `json:"unschedulable"`
}

func (s *Server) handleUnsched2147(w http.ResponseWriter, r *http.Request) {
	result := UnschedResult2147{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if node.Spec.Unschedulable {
			result.Summary.Unschedulable++
			score -= 5
		} else {
			result.Summary.Schedulable++
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Status Ready Ratio
type CtnrReadyResult2147 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CtnrReadySummary2147 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type CtnrReadySummary2147 struct {
	TotalContainers int `json:"totalContainers"`
	Ready           int `json:"ready"`
}

func (s *Server) handleCtnrReady2147(w http.ResponseWriter, r *http.Request) {
	result := CtnrReadyResult2147{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.Ready {
				result.Summary.Ready++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Last Timestamp Freshness
type EvtFreshResult2147 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         EvtFreshSummary2147 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type EvtFreshSummary2147 struct {
	TotalEvents int `json:"totalEvents"`
	FreshEvents int `json:"freshEvents"`
	StaleEvents int `json:"staleEvents"`
}

func (s *Server) handleEvtFresh2147(w http.ResponseWriter, r *http.Request) {
	result := EvtFreshResult2147{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		if !evt.LastTimestamp.IsZero() {
			if now.Sub(evt.LastTimestamp.Time).Hours() < 1 {
				result.Summary.FreshEvents++
			} else {
				result.Summary.StaleEvents++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
