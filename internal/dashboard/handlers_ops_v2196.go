package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.96 — Operations Dimension (Round 52)
// 1. Pod OOM Risk Forecast
// 2. Node Disk Pressure Forecast
// 3. Event Namespace Distribution
// ============================================================

type OOMRiskResult2196 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithoutMemLimit int `json:"withoutMemoryLimit"`
		AtRisk          int `json:"atRisk"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleOOMRisk2196(w http.ResponseWriter, r *http.Request) {
	result := OOMRiskResult2196{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Resources.Limits.Memory().IsZero() {
				result.Summary.WithoutMemLimit++
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.Summary.AtRisk = result.Summary.WithoutMemLimit
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Disk Pressure Forecast
type DiskPressureResult2196 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes       int `json:"totalNodes"`
		WithDiskPressure int `json:"withDiskPressure"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDiskPressure2196(w http.ResponseWriter, r *http.Request) {
	result := DiskPressureResult2196{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
				result.Summary.WithDiskPressure++
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

// 3. Event Namespace Distribution
type EvtNSDistResult2196 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByNamespace map[string]int `json:"byNamespace"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEvtNSDist2196(w http.ResponseWriter, r *http.Request) {
	result := EvtNSDistResult2196{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByNamespace = make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		ns := evt.Namespace
		if ns == "" {
			ns = "cluster"
		}
		result.Summary.ByNamespace[ns]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
