package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.48 Operations: Node PID Pressure, Pod TerminationGracePeriod, Container Lifecycle Hooks
type NodePIDPressureResult2448 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		PIDPressure int `json:"pidPressure"`
	} `json:"summary"`
}

func (s *Server) handleNodePIDPressure2448(w http.ResponseWriter, r *http.Request) {
	result := NodePIDPressureResult2448{ScannedAt: time.Now()}
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

type TermGracePeriodResult2448 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		DefaultGrace int `json:"defaultGrace"`
		CustomGrace  int `json:"customGrace"`
	} `json:"summary"`
}

func (s *Server) handleTermGracePeriod2448(w http.ResponseWriter, r *http.Request) {
	result := TermGracePeriodResult2448{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		var gp int
		if pod.Spec.TerminationGracePeriodSeconds != nil {
			gp = int(*pod.Spec.TerminationGracePeriodSeconds)
		}
		if gp == 30 || gp == 0 {
			result.Summary.DefaultGrace++
		} else {
			result.Summary.CustomGrace++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LifecycleHooksResult2448 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithPostStart   int `json:"withPostStart"`
		WithPreStop     int `json:"withPreStop"`
	} `json:"summary"`
}

func (s *Server) handleLifecycleHooks2448(w http.ResponseWriter, r *http.Request) {
	result := LifecycleHooksResult2448{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Lifecycle != nil {
				if c.Lifecycle.PostStart != nil {
					result.Summary.WithPostStart++
				}
				if c.Lifecycle.PreStop != nil {
					result.Summary.WithPreStop++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
