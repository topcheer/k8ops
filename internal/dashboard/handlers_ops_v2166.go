package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.66 — Operations Dimension (Round 47)
// 1. Node Condition Summary
// 2. Pod Container Restart Timeline
// 3. Service Endpoints Health Check
// ============================================================

type NodeCondSummaryResult2166 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes      int            `json:"totalNodes"`
		ConditionCounts map[string]int `json:"conditionCounts"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeCondSummary2166(w http.ResponseWriter, r *http.Request) {
	result := NodeCondSummaryResult2166{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ConditionCounts = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				result.Summary.ConditionCounts[string(cond.Type)+":false"]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod Restart Timeline
type RestartTimelineResult2166 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int   `json:"totalPods"`
		TotalRestarts int32 `json:"totalRestarts"`
		MaxRestarts   int32 `json:"maxRestartsOnPod"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRestartTimeline2166(w http.ResponseWriter, r *http.Request) {
	result := RestartTimelineResult2166{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		var maxR int32
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalRestarts += cs.RestartCount
			if cs.RestartCount > maxR {
				maxR = cs.RestartCount
			}
		}
		if maxR > result.Summary.MaxRestarts {
			result.Summary.MaxRestarts = maxR
		}
	}
	if result.Summary.MaxRestarts > 10 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Service Endpoints Health
type SvcEpHealthResult2166 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		Healthy       int `json:"healthy"`
		Unhealthy     int `json:"unhealthy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSvcEpHealth2166(w http.ResponseWriter, r *http.Request) {
	result := SvcEpHealthResult2166{ScannedAt: time.Now()}
	score := 100
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	for _, ep := range epList.Items {
		result.Summary.TotalServices++
		totalAddrs := 0
		for _, sub := range ep.Subsets {
			totalAddrs += len(sub.Addresses)
		}
		if totalAddrs > 0 {
			result.Summary.Healthy++
		} else {
			result.Summary.Unhealthy++
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
