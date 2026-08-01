package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.12 Operations: Pod GracePeriod, Node Memory Capacity GB, Event Source Component
type GracePeriodResult2412 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithGrace int `json:"withCustomGracePeriod"`
	} `json:"summary"`
}

func (s *Server) handleGracePeriod2412(w http.ResponseWriter, r *http.Request) {
	result := GracePeriodResult2412{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.TerminationGracePeriodSeconds != nil {
			result.Summary.WithGrace++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemCapResult2412 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalMemGB float64 `json:"totalCapacityMemGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemCap2412(w http.ResponseWriter, r *http.Request) {
	result := NodeMemCapResult2412{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalMemGB += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventSourceResult2412 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByComponent map[string]int `json:"bySourceComponent"`
	} `json:"summary"`
}

func (s *Server) handleEventSource2412(w http.ResponseWriter, r *http.Request) {
	result := EventSourceResult2412{ScannedAt: time.Now()}
	result.Summary.ByComponent = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		result.Summary.ByComponent[evt.Source.Component]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
