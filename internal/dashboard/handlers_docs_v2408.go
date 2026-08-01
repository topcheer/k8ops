package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.08 Documentation: Node Allocatable Ephemeral Storage, Pod Spec Subdomain, ConfigMap BinaryData
type AllocEphemeralResult2408 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalGB    float64 `json:"totalAllocatableEphemeralGB"`
	} `json:"summary"`
}

func (s *Server) handleAllocEphemeral2408(w http.ResponseWriter, r *http.Request) {
	result := AllocEphemeralResult2408{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalGB += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodSubdomainResult2408 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithSubdomain int `json:"withSubdomain"`
	} `json:"summary"`
}

func (s *Server) handlePodSubdomain2408(w http.ResponseWriter, r *http.Request) {
	result := PodSubdomainResult2408{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Subdomain != "" {
			result.Summary.WithSubdomain++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CMBinaryResult2408 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs    int `json:"totalConfigMaps"`
		WithBinData int `json:"withBinaryData"`
	} `json:"summary"`
}

func (s *Server) handleCMBinary2408(w http.ResponseWriter, r *http.Request) {
	result := CMBinaryResult2408{ScannedAt: time.Now()}
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		if len(cm.BinaryData) > 0 {
			result.Summary.WithBinData++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
