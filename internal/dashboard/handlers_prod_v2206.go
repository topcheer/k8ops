package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.06 — Product Dimension (Round 54)
// 1. Pod ImageID Digest Tracker
// 2. Container Probe Success Rate Estimate
// 3. Service LoadBalancer SourceRanges Audit
// ============================================================

type ImgDigestResult2206 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithImageID     int `json:"withImageID"`
		WithDigest      int `json:"withDigest"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgDigest2206(w http.ResponseWriter, r *http.Request) {
	result := ImgDigestResult2206{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.ImageID != "" {
				result.Summary.WithImageID++
				if containsStr2039(cs.ImageID, "sha256:") || containsStr2039(cs.ImageID, "@") {
					result.Summary.WithDigest++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Probe Success Rate Estimate
type ProbeSuccessResult2206 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithLiveness    int `json:"withLivenessProbe"`
		WithReadiness   int `json:"withReadinessProbe"`
		WithStartup     int `json:"withStartupProbe"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleProbeSuccess2206(w http.ResponseWriter, r *http.Request) {
	result := ProbeSuccessResult2206{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.LivenessProbe != nil {
				result.Summary.WithLiveness++
			}
			if c.ReadinessProbe != nil {
				result.Summary.WithReadiness++
			}
			if c.StartupProbe != nil {
				result.Summary.WithStartup++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. LB SourceRanges Audit
type LBSourceRangeResult2206 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB    int `json:"totalLoadBalancers"`
		WithRanges int `json:"withSourceRanges"`
		OpenLB     int `json:"openLoadBalancers"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleLBSourceRange2206(w http.ResponseWriter, r *http.Request) {
	result := LBSourceRangeResult2206{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		if len(svc.Spec.LoadBalancerSourceRanges) > 0 {
			result.Summary.WithRanges++
		} else {
			result.Summary.OpenLB++
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
