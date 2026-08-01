package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.28 Product: Pod TopologySpread Constraints, Container StdinOnce, Service PublishNotReadyAddresses
type TopologySpreadResult2428 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		WithTopoSpread int `json:"withTopologySpreadConstraints"`
	} `json:"summary"`
}

func (s *Server) handleTopologySpread2428(w http.ResponseWriter, r *http.Request) {
	result := TopologySpreadResult2428{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.TopologySpreadConstraints) > 0 {
			result.Summary.WithTopoSpread++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StdinOnceResult2428 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdinOnce   int `json:"withStdinOnce"`
	} `json:"summary"`
}

func (s *Server) handleStdinOnce2428(w http.ResponseWriter, r *http.Request) {
	result := StdinOnceResult2428{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StdinOnce {
				result.Summary.WithStdinOnce++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PublishNotReadyResult2428 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices   int `json:"totalServices"`
		PublishNotReady int `json:"publishNotReadyAddresses"`
	} `json:"summary"`
}

func (s *Server) handlePublishNotReady2428(w http.ResponseWriter, r *http.Request) {
	result := PublishNotReadyResult2428{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.PublishNotReadyAddresses {
			result.Summary.PublishNotReady++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
