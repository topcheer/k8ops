package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.20 — Operations Dimension (Round 56)
// 1. Pod SecurityContext SupplementalGroups Distribution
// 2. Node Allocatable Pods Per Node
// 3. Event Reason Top Frequency
// ============================================================

type SuppGroupsDistResult2220 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByGroupID map[string]int `json:"byGroupID"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSuppGroupsDist2220(w http.ResponseWriter, r *http.Request) {
	result := SuppGroupsDistResult2220{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByGroupID = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil {
			for _, g := range pod.Spec.SecurityContext.SupplementalGroups {
				result.Summary.ByGroupID[fmt.Sprintf("%d", g)]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Allocatable Pods Per Node
type AllocPodsPerNodeResult2220 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalCap   int `json:"totalPodCapacity"`
		AvgPerNode int `json:"avgPodsPerNode"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleAllocPodsPerNode2220(w http.ResponseWriter, r *http.Request) {
	result := AllocPodsPerNodeResult2220{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pods := node.Status.Allocatable.Pods()
		if pods != nil {
			result.Summary.TotalCap += int(pods.AsApproximateFloat64())
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgPerNode = result.Summary.TotalCap / result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Reason Top Frequency
type EvtReasonFreqResult2220 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByReason    map[string]int `json:"byReason"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEvtReasonFreq2220(w http.ResponseWriter, r *http.Request) {
	result := EvtReasonFreqResult2220{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByReason = make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		reason := evt.Reason
		if reason == "" {
			reason = "Unknown"
		}
		result.Summary.ByReason[reason]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
