package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.36 — Product Dimension (Round 59)
// 1. Pod OS Feature Gate Audit
// 2. Container Resources Limit CPU Distribution
// 3. Service ClusterIP Block Catalog
// ============================================================

type OSFeatureGateResult2236 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByOSFeature map[string]int `json:"byOSFeatureGate"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleOSFeatureGate2236(w http.ResponseWriter, r *http.Request) {
	result := OSFeatureGateResult2236{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByOSFeature = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil {
			result.Summary.ByOSFeature[string(pod.Spec.OS.Name)]++
		} else {
			result.Summary.ByOSFeature["linux (default)"]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Limit CPU Distribution
type LimCPUDistResult2236 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		WithCPULimit    int     `json:"withCPULimit"`
		TotalLimitCPU   float64 `json:"totalLimitCPU"`
		AvgLimitCPU     float64 `json:"avgLimitCPU"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleLimCPUDist2236(w http.ResponseWriter, r *http.Request) {
	result := LimCPUDistResult2236{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			lim := c.Resources.Limits.Cpu().AsApproximateFloat64()
			if lim > 0 {
				result.Summary.WithCPULimit++
				result.Summary.TotalLimitCPU += lim
			}
		}
	}
	if result.Summary.WithCPULimit > 0 {
		result.Summary.AvgLimitCPU = result.Summary.TotalLimitCPU / float64(result.Summary.WithCPULimit)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. ClusterIP Block Catalog
type ClusterIPBlockResult2236 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByIPBlock     map[string]int `json:"byClusterIPBlock"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleClusterIPBlock2236(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPBlockResult2236{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByIPBlock = make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
			block := svc.Spec.ClusterIP
			if len(block) >= 7 {
				block = block[:7] + "x.xxx"
			}
			result.Summary.ByIPBlock[block]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
