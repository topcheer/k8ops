package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.90 — Operations Dimension (Round 51)
// 1. Pod QoS Burstable Analysis
// 2. Node Container Runtime Version
// 3. Event Message Frequency Catalog
// ============================================================

type BurstableResult2190 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		Guaranteed int `json:"guaranteed"`
		Burstable  int `json:"burstable"`
		BestEffort int `json:"bestEffort"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleBurstable2190(w http.ResponseWriter, r *http.Request) {
	result := BurstableResult2190{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		switch pod.Status.QOSClass {
		case corev1.PodQOSGuaranteed:
			result.Summary.Guaranteed++
		case corev1.PodQOSBurstable:
			result.Summary.Burstable++
		case corev1.PodQOSBestEffort:
			result.Summary.BestEffort++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Runtime Version
type CRVerResult2190 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byRuntime"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCRVer2190(w http.ResponseWriter, r *http.Request) {
	result := CRVerResult2190{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByRuntime = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByRuntime[node.Status.NodeInfo.ContainerRuntimeVersion]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Event Message Frequency
type EvtMsgFreqResult2190 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int `json:"totalEvents"`
		UniqueMsgs  int `json:"uniqueMessages"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEvtMsgFreq2190(w http.ResponseWriter, r *http.Request) {
	result := EvtMsgFreqResult2190{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	msgSet := make(map[string]bool)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		msgSet[evt.Message] = true
	}
	result.Summary.UniqueMsgs = len(msgSet)
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
