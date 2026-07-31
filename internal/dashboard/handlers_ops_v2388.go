package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.88 Operations: Pod InitContainer Count, Node Allocatable Pods, Event Count By Namespace
type InitCtnrResult2388 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithInitCtnr int `json:"withInitContainers"`
	} `json:"summary"`
}

func (s *Server) handleInitCtnr2388(w http.ResponseWriter, r *http.Request) {
	result := InitCtnrResult2388{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.InitContainers) > 0 {
			result.Summary.WithInitCtnr++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeAllocPodsResult2388 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalAlloc int `json:"totalAllocatablePods"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocPods2388(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocPodsResult2388{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += int(node.Status.Allocatable.Pods().Value())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventByNSResult2388 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByNS        map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleEventByNS2388(w http.ResponseWriter, r *http.Request) {
	result := EventByNSResult2388{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		result.Summary.ByNS[evt.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
