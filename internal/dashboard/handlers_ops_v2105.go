package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.05 — Operations Dimension (Round 37)
// 1. Node Allocatable Pod Ratio
// 2. Pod Container Count Distribution
// 3. Event Source Distribution
// ============================================================

type AllocPodResult2105 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AllocPodSummary2105 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type AllocPodSummary2105 struct {
	TotalNodes  int `json:"totalNodes"`
	TotalPods   int `json:"runningPods"`
	AvgPodRatio int `json:"avgPodRatioPct"`
}

func (s *Server) handleAllocPod2105(w http.ResponseWriter, r *http.Request) {
	result := AllocPodResult2105{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	totalCap := 0
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pods := node.Status.Allocatable.Pods()
		if pods != nil {
			totalCap += int(pods.AsApproximateFloat64())
		}
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	if totalCap > 0 {
		result.Summary.AvgPodRatio = result.Summary.TotalPods * 100 / totalCap
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Count Distribution
type CtnrCntResult2105 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CtnrCntSummary2105 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type CtnrCntSummary2105 struct {
	TotalPods     int `json:"totalPods"`
	MultiCtnrPods int `json:"multiContainerPods"`
	MaxCtnrPerPod int `json:"maxContainersPerPod"`
}

func (s *Server) handleCtnrCnt2105(w http.ResponseWriter, r *http.Request) {
	result := CtnrCntResult2105{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		cnt := len(pod.Spec.Containers)
		if cnt > 1 {
			result.Summary.MultiCtnrPods++
		}
		if cnt > result.Summary.MaxCtnrPerPod {
			result.Summary.MaxCtnrPerPod = cnt
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Source Distribution
type EvtSrcResult2105 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         EvtSrcSummary2105 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type EvtSrcSummary2105 struct {
	TotalEvents int            `json:"totalEvents"`
	ByComponent map[string]int `json:"byComponent"`
}

func (s *Server) handleEvtSrc2105(w http.ResponseWriter, r *http.Request) {
	result := EvtSrcResult2105{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	byComp := make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		comp := evt.Source.Component
		if comp == "" {
			comp = "unknown"
		}
		byComp[comp]++
	}
	result.Summary.ByComponent = byComp
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
