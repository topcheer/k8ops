package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.78 Product: Pod Spec TopologySpreadConstraints Count, Container Stdin Config, Service ClusterIP Detail
type TopologyCountResult2578 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int `json:"totalPods"`
		TotalConstraints int `json:"totalTopologyConstraints"`
	}
}

func (s *Server) handleTopologyCount2578(w http.ResponseWriter, r *http.Request) {
	result := TopologyCountResult2578{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalConstraints += len(pod.Spec.TopologySpreadConstraints)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StdinConfigResult2578 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdin       int `json:"withStdin"`
	}
}

func (s *Server) handleStdinConfig2578(w http.ResponseWriter, r *http.Request) {
	result := StdinConfigResult2578{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Stdin {
				result.Summary.WithStdin++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcClusterIPDetailResult2578 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int `json:"totalServices"`
		IPv4Count int `json:"ipv4Count"`
		IPv6Count int `json:"ipv6Count"`
	}
}

func (s *Server) handleSvcClusterIPDetail2578(w http.ResponseWriter, r *http.Request) {
	result := SvcClusterIPDetailResult2578{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		for _, ip := range svc.Spec.ClusterIPs {
			if ip != "" && ip != "None" {
				if len(ip) >= 2 && ip[:2] == "::" {
					result.Summary.IPv6Count++
				} else {
					result.Summary.IPv4Count++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
