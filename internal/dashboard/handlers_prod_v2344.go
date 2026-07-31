package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.44 Product: Pod HostUsers Audit, Container Port HostPort Audit, Service ExternalIP Catalog
type HostUsersResult2344 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithHostUsers int `json:"withHostUsers"`
	} `json:"summary"`
}

func (s *Server) handleHostUsers2344(w http.ResponseWriter, r *http.Request) {
	result := HostUsersResult2344{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil && *pod.Spec.SecurityContext.RunAsUser == 0 {
			result.Summary.WithHostUsers++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = 100 - (result.Summary.WithHostUsers*50)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type HostPortResult2344 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithHostPort    int `json:"withHostPort"`
	} `json:"summary"`
}

func (s *Server) handleHostPort2344(w http.ResponseWriter, r *http.Request) {
	result := HostPortResult2344{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, p := range c.Ports {
				if p.HostPort != 0 {
					result.Summary.WithHostPort++
					break
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExternalIPResult2344 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices  int `json:"totalServices"`
		WithExternalIP int `json:"withExternalIP"`
	} `json:"summary"`
}

func (s *Server) handleExternalIP2344(w http.ResponseWriter, r *http.Request) {
	result := ExternalIPResult2344{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if len(svc.Spec.ExternalIPs) > 0 {
			result.Summary.WithExternalIP++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
