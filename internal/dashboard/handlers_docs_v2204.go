package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.04 — Documentation Dimension (Round 53)
// 1. Node Feature Discovery Label Count
// 2. ConfigMap Immutable Status Catalog
// 3. Pod Priority Class Distribution
// ============================================================

type NodeFeatureLabelResult2204 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes       int            `json:"totalNodes"`
		AvgLabelsPerNode int            `json:"avgLabelsPerNode"`
		FeatureLabels    map[string]int `json:"featureLabels"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeFeatureLabel2204(w http.ResponseWriter, r *http.Request) {
	result := NodeFeatureLabelResult2204{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.FeatureLabels = make(map[string]int)
	totalLabels := 0
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		totalLabels += len(node.Labels)
		for k := range node.Labels {
			if containsStr2039(k, "feature") || containsStr2039(k, "node.kubernetes.io") || containsStr2039(k, "topology") {
				result.Summary.FeatureLabels[k]++
			}
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgLabelsPerNode = totalLabels / result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. CM Immutable Catalog
type CMImmutableCatResult2204 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs  int `json:"totalConfigMaps"`
		Immutable int `json:"immutable"`
		Mutable   int `json:"mutable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCMImmutableCat2204(w http.ResponseWriter, r *http.Request) {
	result := CMImmutableCatResult2204{ScannedAt: time.Now()}
	score := 100
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		if cm.Immutable != nil && *cm.Immutable {
			result.Summary.Immutable++
		} else {
			result.Summary.Mutable++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Priority Class Distribution
type PriorityDistResult2204 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByPriority map[string]int `json:"byPriorityClass"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePriorityDist2204(w http.ResponseWriter, r *http.Request) {
	result := PriorityDistResult2204{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPriority = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		pc := pod.Spec.PriorityClassName
		if pc == "" {
			pc = "none"
		}
		result.Summary.ByPriority[pc]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
