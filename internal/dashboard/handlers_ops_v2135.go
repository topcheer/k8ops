package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.35 — Operations Dimension (Round 42)
// 1. Node Ready Transition Counter
// 2. Pod Container Age Distribution
// 3. Service Endpoint Ready Ratio
// ============================================================

type NodeTransResult2135 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NodeTransSummary2135 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type NodeTransSummary2135 struct {
	TotalNodes    int `json:"totalNodes"`
	ReadyNodes    int `json:"readyNodes"`
	NotReadyNodes int `json:"notReadyNodes"`
}

func (s *Server) handleNodeTrans2135(w http.ResponseWriter, r *http.Request) {
	result := NodeTransResult2135{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if ready {
			result.Summary.ReadyNodes++
		} else {
			result.Summary.NotReadyNodes++
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Age Distribution
type CtnrAgeResult2135 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CtnrAgeSummary2135 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type CtnrAgeSummary2135 struct {
	TotalPods int `json:"totalPods"`
	OldPods   int `json:"oldPods"`
}

func (s *Server) handleCtnrAge2135(w http.ResponseWriter, r *http.Request) {
	result := CtnrAgeResult2135{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Status.StartTime != nil {
			ageDays := int(now.Sub(pod.Status.StartTime.Time).Hours() / 24)
			if ageDays > 180 {
				result.Summary.OldPods++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Endpoint Ready Ratio
type EPReadyResult2135 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         EPReadySummary2135 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type EPReadySummary2135 struct {
	TotalServices int `json:"totalServices"`
	HealthySvcs   int `json:"healthyServices"`
	UnhealthySvcs int `json:"unhealthyServices"`
}

func (s *Server) handleEPReady2135(w http.ResponseWriter, r *http.Request) {
	result := EPReadyResult2135{ScannedAt: time.Now()}
	score := 100
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})

	for _, ep := range epList.Items {
		result.Summary.TotalServices++
		totalAddrs := 0
		for _, sub := range ep.Subsets {
			totalAddrs += len(sub.Addresses)
		}
		if totalAddrs > 0 {
			result.Summary.HealthySvcs++
		} else {
			result.Summary.UnhealthySvcs++
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
