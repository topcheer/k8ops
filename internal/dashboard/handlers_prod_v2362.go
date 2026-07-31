package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.62 Product: Pod ShareProcessNamespace, Container Resources Missing Audit, Service HealthCheck Port
type ShareProcNSResult2362 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithShare int `json:"withShareProcessNamespace"`
	} `json:"summary"`
}

func (s *Server) handleShareProcNS2362(w http.ResponseWriter, r *http.Request) {
	result := ShareProcNSResult2362{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
			result.Summary.WithShare++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MissingResResult2362 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithoutLimits   int `json:"withoutLimits"`
	} `json:"summary"`
}

func (s *Server) handleMissingRes2362(w http.ResponseWriter, r *http.Request) {
	result := MissingResResult2362{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
				result.Summary.WithoutLimits++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.WithoutLimits*40)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type HCPortResult2362 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithHCPort    int `json:"withHealthCheckPort"`
	} `json:"summary"`
}

func (s *Server) handleHCPort2362(w http.ResponseWriter, r *http.Request) {
	result := HCPortResult2362{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.HealthCheckNodePort != 0 {
			result.Summary.WithHCPort++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
