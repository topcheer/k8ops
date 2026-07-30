package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.25 — Documentation Dimension (Round 40)
// 1. Node Feature Label Inventory
// 2. Pod Service Account Mapping
// 3. ConfigMap Immutable Flag Audit
// ============================================================

type FeatureLabelResult2125 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         FeatureLabelSummary2125 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type FeatureLabelSummary2125 struct {
	TotalNodes    int            `json:"totalNodes"`
	FeatureLabels map[string]int `json:"featureLabels"`
}

func (s *Server) handleFeatureLabel2125(w http.ResponseWriter, r *http.Request) {
	result := FeatureLabelResult2125{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	featLabels := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for k := range node.Labels {
			if containsStr2039(k, "feature") || containsStr2039(k, "node.kubernetes.io") {
				featLabels[k]++
			}
		}
	}
	result.Summary.FeatureLabels = featLabels
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod SA Mapping
type SAMappingResult2125 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SAMappingSummary2125 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type SAMappingSummary2125 struct {
	TotalPods int            `json:"totalPods"`
	BySA      map[string]int `json:"byServiceAccount"`
}

func (s *Server) handleSAMapping2125(w http.ResponseWriter, r *http.Request) {
	result := SAMappingResult2125{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	bySA := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sa := pod.Spec.ServiceAccountName
		if sa == "" {
			sa = "default"
		}
		bySA[pod.Namespace+"/"+sa]++
	}
	result.Summary.BySA = bySA
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. CM Immutable Audit
type CMImmutableResult2125 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CMImmutableSummary2125 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type CMImmutableSummary2125 struct {
	TotalCMs  int `json:"totalConfigMaps"`
	Immutable int `json:"immutable"`
}

func (s *Server) handleCMImmutable2125(w http.ResponseWriter, r *http.Request) {
	result := CMImmutableResult2125{ScannedAt: time.Now()}
	score := 100
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})

	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		if cm.Immutable != nil && *cm.Immutable {
			result.Summary.Immutable++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
