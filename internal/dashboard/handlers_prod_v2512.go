package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.12 Product: Pod OS Check, Container Image Versioned Tag, Service LoadBalancerSourceRanges
type PodOSResult2512 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByOS      map[string]int `json:"byOSName"`
	} `json:"summary"`
}

func (s *Server) handlePodOS2512(w http.ResponseWriter, r *http.Request) {
	result := PodOSResult2512{ScannedAt: time.Now()}
	result.Summary.ByOS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil {
			result.Summary.ByOS[string(pod.Spec.OS.Name)]++
		} else {
			result.Summary.ByOS["linux"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageVersionedTagResult2512 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages  int `json:"totalImages"`
		VersionedTag int `json:"versionedTag"`
	} `json:"summary"`
}

func (s *Server) handleImageVersionedTag2512(w http.ResponseWriter, r *http.Request) {
	result := ImageVersionedTagResult2512{ScannedAt: time.Now()}
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
			if hasTag {
				result.Summary.VersionedTag++
			}
		}
	}
	score := 100
	if result.Summary.TotalImages > 0 {
		score = result.Summary.VersionedTag * 100 / result.Summary.TotalImages
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type LBSourceRangesResult2512 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB    int `json:"totalLoadBalancerServices"`
		WithRanges int `json:"withSourceRanges"`
	} `json:"summary"`
}

func (s *Server) handleLBSourceRanges2512(w http.ResponseWriter, r *http.Request) {
	result := LBSourceRangesResult2512{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if len(svc.Spec.LoadBalancerSourceRanges) > 0 {
			result.Summary.WithRanges++
		}
	}
	score := 100
	if result.Summary.TotalLB > 0 {
		score = result.Summary.WithRanges * 100 / result.Summary.TotalLB
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
