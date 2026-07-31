package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.80 Operations: Pod OOM Risk Detection, Node PID Pressure, Container Last Termination Reason
type OOMRiskResult2280 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithoutMemLimit int `json:"withoutMemLimit"`
		OOMKilled       int `json:"oomKilled"`
	} `json:"summary"`
}

func (s *Server) handleOOMRisk2280(w http.ResponseWriter, r *http.Request) {
	result := OOMRiskResult2280{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Resources.Limits.Memory().IsZero() {
				result.Summary.WithoutMemLimit++
			}
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
				result.Summary.OOMKilled++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		noLimitPct := result.Summary.WithoutMemLimit * 100 / result.Summary.TotalContainers
		score = 100 - noLimitPct/3
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PIDPressureResult2280 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		WithPressure int `json:"withPIDPressure"`
	} `json:"summary"`
}

func (s *Server) handlePIDPressure2280(w http.ResponseWriter, r *http.Request) {
	result := PIDPressureResult2280{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodePIDPressure && cond.Status == corev1.ConditionTrue {
				result.Summary.WithPressure++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.WithPressure*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type LastTermReasonResult2280 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalTerminated int            `json:"totalTerminated"`
		ByReason        map[string]int `json:"byReason"`
	} `json:"summary"`
}

func (s *Server) handleLastTermReason2280(w http.ResponseWriter, r *http.Request) {
	result := LastTermReasonResult2280{ScannedAt: time.Now()}
	result.Summary.ByReason = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalTerminated++
				reason := cs.LastTerminationState.Terminated.Reason
				if reason == "" {
					reason = "Unknown"
				}
				result.Summary.ByReason[reason]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
