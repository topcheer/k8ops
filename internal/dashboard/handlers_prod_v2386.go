package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.86 Product: Pod Tolerations Audit, Container Ports ContainerPort, Service Annotations Census
type TolerationResult2386 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int `json:"totalPods"`
		WithTolerations int `json:"withTolerations"`
	} `json:"summary"`
}

func (s *Server) handleToleration2386(w http.ResponseWriter, r *http.Request) {
	result := TolerationResult2386{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.Tolerations) > 0 {
			result.Summary.WithTolerations++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrPortResult2386 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalPorts      int `json:"totalContainerPorts"`
	} `json:"summary"`
}

func (s *Server) handleCtnrPort2386(w http.ResponseWriter, r *http.Request) {
	result := CtnrPortResult2386{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalPorts += len(c.Ports)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcAnnotResult2386 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithAnnot     int `json:"withAnnotations"`
	} `json:"summary"`
}

func (s *Server) handleSvcAnnot2386(w http.ResponseWriter, r *http.Request) {
	result := SvcAnnotResult2386{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if len(svc.Annotations) > 0 {
			result.Summary.WithAnnot++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
