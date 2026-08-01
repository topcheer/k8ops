package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.76 Product: Pod TopologySpreadConstraints, Container Image Registry Summary, Service SessionAffinityConfig
type TopologySpreadResult2476 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int `json:"totalPods"`
		WithTopology     int `json:"withTopologySpread"`
		TotalConstraints int `json:"totalConstraints"`
	} `json:"summary"`
}

func (s *Server) handleTopologySpread2476(w http.ResponseWriter, r *http.Request) {
	result := TopologySpreadResult2476{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.TopologySpreadConstraints) > 0 {
			result.Summary.WithTopology++
			result.Summary.TotalConstraints += len(pod.Spec.TopologySpreadConstraints)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageRegistryResult2476 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int            `json:"totalImages"`
		ByRegistry  map[string]int `json:"byRegistry"`
	} `json:"summary"`
}

func (s *Server) handleImageRegistry2476(w http.ResponseWriter, r *http.Request) {
	result := ImageRegistryResult2476{ScannedAt: time.Now()}
	result.Summary.ByRegistry = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			reg := extractRegistry2476(c.Image)
			result.Summary.ByRegistry[reg]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

func extractRegistry2476(image string) string {
	for i := 0; i < len(image); i++ {
		if image[i] == '/' {
			return image[:i]
		}
	}
	return "docker.io"
}

type SessionAffinityCfgResult2476 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int `json:"totalServices"`
		WithConfig int `json:"withSessionAffinityConfig"`
	} `json:"summary"`
}

func (s *Server) handleSessionAffinityCfg2476(w http.ResponseWriter, r *http.Request) {
	result := SessionAffinityCfgResult2476{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.SessionAffinityConfig != nil {
			result.Summary.WithConfig++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
