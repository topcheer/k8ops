package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.32 Product: Pod Spec HostUserspace, Container Resource Ephemeral Storage Request Detail, Service TrafficDistribution
type PodHostUserspace2632Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostUser  int `json:"withHostUserspace"`
	} `json:"summary"`
}

func (s *Server) handlePodHostUserspace2632(w http.ResponseWriter, r *http.Request) {
	result := PodHostUserspace2632Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostUserspace != nil && *pod.Spec.HostUserspace {
			result.Summary.HostUser++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EphemeralReqDetail2632Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalEphemeral  float64 `json:"totalEphemeralReqGB"`
	} `json:"summary"`
}

func (s *Server) handleEphemeralReqDetail2632(w http.ResponseWriter, r *http.Request) {
	result := EphemeralReqDetail2632Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalEphemeral += c.Resources.Requests.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcTrafficDist2632Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByPolicy  map[string]int `json:"byTrafficDistribution"`
	} `json:"summary"`
}

func (s *Server) handleSvcTrafficDist2632(w http.ResponseWriter, r *http.Request) {
	result := SvcTrafficDist2632Result{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.TrafficDistribution != nil {
			result.Summary.ByPolicy[*svc.Spec.TrafficDistribution]++
		} else {
			result.Summary.ByPolicy["<default>"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
