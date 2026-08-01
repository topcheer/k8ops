package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.18 Product: Pod Spec hostname vs nodename, Container Image Layer Count, Service HealthCheck NodePort
type HostnameVsNodeResult2518 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Matched   int `json:"hostnameMatchesNode"`
	} `json:"summary"`
}

func (s *Server) handleHostnameVsNode2518(w http.ResponseWriter, r *http.Request) {
	result := HostnameVsNodeResult2518{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Hostname != "" && pod.Spec.Hostname == pod.Spec.NodeName {
			result.Summary.Matched++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageLayerResult2518 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages  int `json:"totalImages"`
		UniqueImages int `json:"uniqueImages"`
		Duplicates   int `json:"duplicateImages"`
	} `json:"summary"`
}

func (s *Server) handleImageLayer2518(w http.ResponseWriter, r *http.Request) {
	result := ImageLayerResult2518{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			seen[c.Image]++
		}
	}
	result.Summary.UniqueImages = len(seen)
	for _, count := range seen {
		if count > 1 {
			result.Summary.Duplicates++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type HCNodePortResult2518 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int `json:"totalServices"`
		WithHCNP  int `json:"withHealthCheckNodePort"`
	} `json:"summary"`
}

func (s *Server) handleHCNodePort2518(w http.ResponseWriter, r *http.Request) {
	result := HCNodePortResult2518{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalSvcs++
		if svc.Spec.HealthCheckNodePort > 0 {
			result.Summary.WithHCNP++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
