package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v24.70 Product: Pod Affinity Rule Count, Container Image Latest Tag Ratio, Service Session Affinity Config
type AffinityRuleResult2470 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithAffinity int `json:"withAffinity"`
		WithAntiAff  int `json:"withAntiAffinity"`
	} `json:"summary"`
}

func (s *Server) handleAffinityRule2470(w http.ResponseWriter, r *http.Request) {
	result := AffinityRuleResult2470{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Affinity != nil {
			if pod.Spec.Affinity.PodAffinity != nil {
				result.Summary.WithAffinity++
			}
			if pod.Spec.Affinity.PodAntiAffinity != nil {
				result.Summary.WithAntiAff++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageLatestTagResult2470 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int `json:"totalImages"`
		LatestTag   int `json:"latestTagCount"`
	} `json:"summary"`
}

func (s *Server) handleImageLatestTag2470(w http.ResponseWriter, r *http.Request) {
	result := ImageLatestTagResult2470{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			parts := strings.Split(c.Image, ":")
			if len(parts) < 2 || parts[len(parts)-1] == "latest" {
				result.Summary.LatestTag++
			}
		}
	}
	score := 100
	if result.Summary.TotalImages > 0 {
		score = 100 - (result.Summary.LatestTag*100)/result.Summary.TotalImages
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SessionAffinityResult2470 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int            `json:"totalServices"`
		ByAffinity map[string]int `json:"bySessionAffinity"`
	} `json:"summary"`
}

func (s *Server) handleSessionAffinity2470(w http.ResponseWriter, r *http.Request) {
	result := SessionAffinityResult2470{ScannedAt: time.Now()}
	result.Summary.ByAffinity = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.ByAffinity[string(svc.Spec.SessionAffinity)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
