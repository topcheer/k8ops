package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.10 Operations: Pod Ephemeral Container Count, Node Unschedulable Census, Event Source Component
type EphemeralResult2310 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithEphemeral int `json:"withEphemeralContainers"`
	} `json:"summary"`
}

func (s *Server) handleEphemeral2310(w http.ResponseWriter, r *http.Request) {
	result := EphemeralResult2310{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.EphemeralContainers) > 0 {
			result.Summary.WithEphemeral++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type UnschedulableResult2310 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int `json:"totalNodes"`
		Unschedulable int `json:"unschedulable"`
	} `json:"summary"`
}

func (s *Server) handleUnschedulable2310(w http.ResponseWriter, r *http.Request) {
	result := UnschedulableResult2310{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		if node.Spec.Unschedulable {
			result.Summary.Unschedulable++
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.Unschedulable*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type EventSourceResult2310 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByComponent map[string]int `json:"bySourceComponent"`
	} `json:"summary"`
}

func (s *Server) handleEventSource2310(w http.ResponseWriter, r *http.Request) {
	result := EventSourceResult2310{ScannedAt: time.Now()}
	result.Summary.ByComponent = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		comp := evt.Source.Component
		if comp == "" {
			comp = evt.Source.Host
		}
		result.Summary.ByComponent[comp]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
