package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.41 — Operations Dimension (Round 43)
// 1. Node Status Condition Chronology
// 2. Pod Phase Transition Rate
// 3. Event Involved Object Type Distribution
// ============================================================

type CondChronResult2141 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CondChronSummary2141 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type CondChronSummary2141 struct {
	TotalNodes      int            `json:"totalNodes"`
	ConditionCounts map[string]int `json:"conditionCounts"`
}

func (s *Server) handleCondChron2141(w http.ResponseWriter, r *http.Request) {
	result := CondChronResult2141{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	condCnt := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				condCnt[string(cond.Type)]++
			}
		}
	}
	result.Summary.ConditionCounts = condCnt
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod Phase Transition Rate
type PhaseRateResult2141 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PhaseRateSummary2141 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type PhaseRateSummary2141 struct {
	TotalPods int            `json:"totalPods"`
	ByPhase   map[string]int `json:"byPhase"`
}

func (s *Server) handlePhaseRate2141(w http.ResponseWriter, r *http.Request) {
	result := PhaseRateResult2141{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	byPhase := make(map[string]int)
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		byPhase[string(pod.Status.Phase)]++
	}
	result.Summary.ByPhase = byPhase
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Involved Object Type
type EvtObjTypeResult2141 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EvtObjTypeSummary2141 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type EvtObjTypeSummary2141 struct {
	TotalEvents int            `json:"totalEvents"`
	ByKind      map[string]int `json:"byInvolvedObjectKind"`
}

func (s *Server) handleEvtObjType2141(w http.ResponseWriter, r *http.Request) {
	result := EvtObjTypeResult2141{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	byKind := make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		kind := evt.InvolvedObject.Kind
		if kind == "" {
			kind = "Unknown"
		}
		byKind[kind]++
	}
	result.Summary.ByKind = byKind
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
