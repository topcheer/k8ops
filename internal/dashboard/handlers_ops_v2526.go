package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.26 Operations: Node Condition LastHeartbeatTime, Pod Pending Count, Container Termination Reason
type NodeHeartbeatResult2526 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		StaleNodes int `json:"staleHeartbeatNodes"`
	} `json:"summary"`
}

func (s *Server) handleNodeHeartbeat2526(w http.ResponseWriter, r *http.Request) {
	result := NodeHeartbeatResult2526{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.LastHeartbeatTime.Time.Before(cutoff) {
				result.Summary.StaleNodes++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodPendingCountResult2526 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Pending   int `json:"pendingPods"`
	} `json:"summary"`
}

func (s *Server) handlePodPendingCount2526(w http.ResponseWriter, r *http.Request) {
	result := PodPendingCountResult2526{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodPending {
			result.Summary.Pending++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.Pending > 0 {
		score = 100 - (result.Summary.Pending*100)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type TermReasonResult2526 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByReason        map[string]int `json:"byTerminationReason"`
	} `json:"summary"`
}

func (s *Server) handleTermReason2526(w http.ResponseWriter, r *http.Request) {
	result := TermReasonResult2526{ScannedAt: time.Now()}
	result.Summary.ByReason = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalContainers++
				reason := cs.LastTerminationState.Terminated.Reason
				if reason == "" {
					reason = "<unknown>"
				}
				result.Summary.ByReason[reason]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
