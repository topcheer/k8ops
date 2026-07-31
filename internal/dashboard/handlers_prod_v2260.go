package dashboard

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.60 Product: Container Port Catalog, Pod QoS Distribution, Resource Limit Adherence
type CtnrPortCatalogResult2260 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByPort          map[string]int `json:"byPort"`
	} `json:"summary"`
}

func (s *Server) handleCtnrPortCatalog2260(w http.ResponseWriter, r *http.Request) {
	result := CtnrPortCatalogResult2260{ScannedAt: time.Now()}
	result.Summary.ByPort = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, p := range c.Ports {
				result.Summary.ByPort[string(p.Protocol)+":"+itoa(int(p.ContainerPort))]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodQoSDistResult2260 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByQoS     map[string]int `json:"byQoS"`
	} `json:"summary"`
}

func (s *Server) handlePodQoSDist2260(w http.ResponseWriter, r *http.Request) {
	result := PodQoSDistResult2260{ScannedAt: time.Now()}
	result.Summary.ByQoS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByQoS[string(pod.Status.QOSClass)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResLimitAdherenceResult2260 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithLimits      int `json:"withLimits"`
		WithRequests    int `json:"withRequests"`
	} `json:"summary"`
}

func (s *Server) handleResLimitAdherence2260(w http.ResponseWriter, r *http.Request) {
	result := ResLimitAdherenceResult2260{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if !c.Resources.Limits.Cpu().IsZero() || !c.Resources.Limits.Memory().IsZero() {
				result.Summary.WithLimits++
			}
			if !c.Resources.Requests.Cpu().IsZero() || !c.Resources.Requests.Memory().IsZero() {
				result.Summary.WithRequests++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.WithLimits * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
