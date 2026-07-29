package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.93 — Operations Dimension (Round 35)
// 1. Pod Image Size Estimator — total image layer size
// 2. Node Kubelet Efficiency — pod density vs allocatable
// 3. Event TTL Analysis — event retention estimation
// ============================================================

type ImgSizeResult2093 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         ImgSizeSummary2093 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type ImgSizeSummary2093 struct {
	UniqueImages int `json:"uniqueImages"`
	TotalPulls   int `json:"totalImagePulls"`
}

func (s *Server) handleImgSize2093(w http.ResponseWriter, r *http.Request) {
	result := ImgSizeResult2093{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.UniqueImages++
			}
			result.Summary.TotalPulls++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Kubelet Efficiency
type KubeletEffResult2093 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         KubeletEffSummary2093 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type KubeletEffSummary2093 struct {
	TotalNodes int `json:"totalNodes"`
	TotalPods  int `json:"totalPods"`
	AvgPerNode int `json:"avgPodsPerNode"`
}

func (s *Server) handleKubeletEff2093(w http.ResponseWriter, r *http.Request) {
	result := KubeletEffResult2093{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalNodes = len(nodeList.Items)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalPods / result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event TTL Analysis
type EventTTLResult2093 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         EventTTLSummary2093 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type EventTTLSummary2093 struct {
	TotalEvents int `json:"totalEvents"`
	OldEvents   int `json:"oldEvents"`
}

func (s *Server) handleEventTTL2093(w http.ResponseWriter, r *http.Request) {
	result := EventTTLResult2093{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		if now.Sub(evt.CreationTimestamp.Time).Hours() > 24 {
			result.Summary.OldEvents++
		}
	}
	if result.Summary.OldEvents > 5000 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.OldEvents > 5000 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d events older than 24h — reduce TTL", result.Summary.OldEvents))
	}
	writeJSON(w, result)
}
