package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.88 Product: Pod Overhead Check, Container Image Without Tag, Service PublishNotReadyAddresses
type PodOverheadResult2488 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithOverhead int `json:"withOverhead"`
	} `json:"summary"`
}

func (s *Server) handlePodOverhead2488(w http.ResponseWriter, r *http.Request) {
	result := PodOverheadResult2488{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Overhead != nil && len(pod.Spec.Overhead) > 0 {
			result.Summary.WithOverhead++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageWithoutTagResult2488 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int `json:"totalImages"`
		WithoutTag  int `json:"withoutTag"`
	} `json:"summary"`
}

func (s *Server) handleImageWithoutTag2488(w http.ResponseWriter, r *http.Request) {
	result := ImageWithoutTagResult2488{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			hasTag := false
			for i := len(c.Image) - 1; i >= 0; i-- {
				if c.Image[i] == ':' {
					hasTag = true
					break
				}
				if c.Image[i] == '/' {
					break
				}
			}
			if !hasTag {
				result.Summary.WithoutTag++
			}
		}
	}
	score := 100
	if result.Summary.TotalImages > 0 {
		score = 100 - (result.Summary.WithoutTag*100)/result.Summary.TotalImages
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PublishNotReadyResult2488 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int `json:"totalServices"`
		PublishNR int `json:"publishNotReadyAddresses"`
	} `json:"summary"`
}

func (s *Server) handlePublishNotReady2488(w http.ResponseWriter, r *http.Request) {
	result := PublishNotReadyResult2488{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.PublishNotReadyAddresses {
			result.Summary.PublishNR++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
