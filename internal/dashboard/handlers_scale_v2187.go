package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.87 — Scalability & HA Dimension (Round 50)
// 1. Node CPU Limit Headroom
// 2. Namespace Deployment Distribution
// 3. Pod Resource Efficiency Score
// ============================================================

type CPULimHRResult2187 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableCPU"`
		TotalLim   float64 `json:"totalLimitedCPU"`
		Headroom   float64 `json:"headroomCPU"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCPULimHR2187(w http.ResponseWriter, r *http.Request) {
	result := CPULimHRResult2187{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalLim += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.Headroom = result.Summary.TotalAlloc - result.Summary.TotalLim
	if result.Summary.Headroom < 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. NS Deployment Distribution
type NSDepDistResult2187 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		Deploys   int    `json:"deployments"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSDepDist2187(w http.ResponseWriter, r *http.Request) {
	result := NSDepDistResult2187{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsDep := make(map[string]int)
	for _, dep := range deployList.Items {
		nsDep[dep.Namespace]++
	}
	result.Summary.TotalNS = len(nsDep)
	for ns, cnt := range nsDep {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			Deploys   int    `json:"deployments"`
		}{ns, cnt})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].Deploys > result.TopNS[j].Deploys })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Resource Efficiency Score
type ResEffScoreResult2187 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithReq         int `json:"withRequests"`
		WithLim         int `json:"withLimits"`
		BothSet         int `json:"bothSet"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleResEffScore2187(w http.ResponseWriter, r *http.Request) {
	result := ResEffScoreResult2187{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			hasReq := !c.Resources.Requests.Cpu().IsZero()
			hasLim := !c.Resources.Limits.Cpu().IsZero()
			if hasReq {
				result.Summary.WithReq++
			}
			if hasLim {
				result.Summary.WithLim++
			}
			if hasReq && hasLim {
				result.Summary.BothSet++
			}
		}
	}
	if result.Summary.BothSet < result.Summary.TotalContainers/2 && result.Summary.TotalContainers > 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
