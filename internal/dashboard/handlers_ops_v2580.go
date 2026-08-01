package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.80 Operations: Node Allocatable vs Capacity Pods Ratio, Pod Spec ImagePullSecrets Detail, Container EnvFrom Count
type NodeAllocCapRatioResult2580 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		AvgRatio   float64 `json:"avgAllocVsCapRatio"`
	}
}

func (s *Server) handleNodeAllocCapRatio2580(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocCapRatioResult2580{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	var totalRatio float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cap := node.Status.Capacity.Pods().Value()
		alloc := node.Status.Allocatable.Pods().Value()
		if cap > 0 {
			totalRatio += float64(alloc) / float64(cap) * 100
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgRatio = totalRatio / float64(result.Summary.TotalNodes)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImagePullSecretsDetailResult2580 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithPullSecs int `json:"withImagePullSecrets"`
	}
}

func (s *Server) handleImagePullSecretsDetail2580(w http.ResponseWriter, r *http.Request) {
	result := ImagePullSecretsDetailResult2580{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.ImagePullSecrets) > 0 {
			result.Summary.WithPullSecs++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EnvFromCountResult2580 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalEnvFrom    int `json:"totalEnvFrom"`
	}
}

func (s *Server) handleEnvFromCount2580(w http.ResponseWriter, r *http.Request) {
	result := EnvFromCountResult2580{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalEnvFrom += len(c.EnvFrom)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
