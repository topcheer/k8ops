package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.87 — Operations Dimension (Round 34)
// 1. Pod Restart Trend — increasing restart pattern
// 2. Node Heartbeat Freshness — node last heartbeat check
// 3. Event Severity Distribution — warning vs normal events
// ============================================================

type RestartTrendResult2087 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         RestartTrendSummary2087 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type RestartTrendSummary2087 struct {
	TotalPods     int   `json:"totalPods"`
	HighRestart   int   `json:"highRestartPods"`
	TotalRestarts int32 `json:"totalRestarts"`
}

func (s *Server) handleRestartTrend2087(w http.ResponseWriter, r *http.Request) {
	result := RestartTrendResult2087{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalRestarts += cs.RestartCount
			if cs.RestartCount > 10 {
				result.Summary.HighRestart++
				score -= 2
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.HighRestart > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods with >10 restarts", result.Summary.HighRestart))
	}
	writeJSON(w, result)
}

// 2. Node Heartbeat Freshness
type HeartbeatResult2087 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HeartbeatSummary2087 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type HeartbeatSummary2087 struct {
	TotalNodes   int `json:"totalNodes"`
	HealthyNodes int `json:"healthyNodes"`
	StaleNodes   int `json:"staleNodes"`
}

func (s *Server) handleHeartbeat2087(w http.ResponseWriter, r *http.Request) {
	result := HeartbeatResult2087{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		stale := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				ageMin := now.Sub(cond.LastHeartbeatTime.Time).Minutes()
				if ageMin > 5 {
					stale = true
				}
			}
		}
		if stale {
			result.Summary.StaleNodes++
			score -= 10
		} else {
			result.Summary.HealthyNodes++
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Severity Distribution
type EventSevResult2087 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         EventSevSummary2087 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type EventSevSummary2087 struct {
	TotalEvents   int `json:"totalEvents"`
	NormalEvents  int `json:"normalEvents"`
	WarningEvents int `json:"warningEvents"`
}

func (s *Server) handleEventSev2087(w http.ResponseWriter, r *http.Request) {
	result := EventSevResult2087{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		if evt.Type == corev1.EventTypeWarning {
			result.Summary.WarningEvents++
		} else {
			result.Summary.NormalEvents++
		}
	}
	if result.Summary.WarningEvents > result.Summary.NormalEvents/2 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
