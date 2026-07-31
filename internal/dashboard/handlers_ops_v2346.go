package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.46 Operations: Pod Unhealthy Container, Node Cond PID, Event Message Catalog
type UnhealthyResult2346 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Unhealthy       int `json:"unhealthyContainers"`
	} `json:"summary"`
}

func (s *Server) handleUnhealthy2346(w http.ResponseWriter, r *http.Request) {
	result := UnhealthyResult2346{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if !cs.Ready {
				result.Summary.Unhealthy++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.Unhealthy*100)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeCondPIDResult2346 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		PIDPressure int `json:"pidPressure"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondPID2346(w http.ResponseWriter, r *http.Request) {
	result := NodeCondPIDResult2346{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue {
				result.Summary.PIDPressure++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.PIDPressure*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type EventMsgResult2346 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		TopMessages map[string]int `json:"topMessages"`
	} `json:"summary"`
}

func (s *Server) handleEventMsg2346(w http.ResponseWriter, r *http.Request) {
	result := EventMsgResult2346{ScannedAt: time.Now()}
	result.Summary.TopMessages = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		msg := evt.Message
		if len(msg) > 80 {
			msg = msg[:80]
		}
		result.Summary.TopMessages[msg]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
