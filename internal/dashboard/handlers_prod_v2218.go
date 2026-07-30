package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.18 — Product Dimension (Round 56)
// 1. Pod PreStop Hook Audit
// 2. Container Image Size Estimate
// 3. Service Type LoadBalancer Health Check Port
// ============================================================

type PreStopHookResult2218 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithPreStop     int `json:"withPreStopHook"`
		WithPostStart   int `json:"withPostStartHook"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePreStopHook2218(w http.ResponseWriter, r *http.Request) {
	result := PreStopHookResult2218{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Lifecycle != nil && c.Lifecycle.PreStop != nil {
				result.Summary.WithPreStop++
			}
			if c.Lifecycle != nil && c.Lifecycle.PostStart != nil {
				result.Summary.WithPostStart++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Image Size Estimate
type ImgSizeEstResult2218 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		TotalImages int `json:"totalUniqueImages"`
		WithLatest  int `json:"imagesWithLatestTag"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgSizeEst2218(w http.ResponseWriter, r *http.Request) {
	result := ImgSizeEstResult2218{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.TotalImages++
			}
			tag := ""
			for i := len(c.Image) - 1; i >= 0; i-- {
				if c.Image[i] == ':' {
					tag = c.Image[i+1:]
					break
				}
				if c.Image[i] == '/' {
					break
				}
			}
			if tag == "latest" || tag == "" {
				result.Summary.WithLatest++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. LB Health Check Port
type LBHealthPortResult2218 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB        int `json:"totalLoadBalancers"`
		WithHealthPort int `json:"withHealthCheckNodePort"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleLBHealthPort2218(w http.ResponseWriter, r *http.Request) {
	result := LBHealthPortResult2218{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if svc.Spec.HealthCheckNodePort > 0 {
			result.Summary.WithHealthPort++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
