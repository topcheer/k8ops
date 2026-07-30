package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.17 — Scalability & HA Dimension (Round 55)
// 1. Namespace Memory Commit Distribution
// 2. Node Pod Capacity Headroom Score
// 3. Cluster Deployment Spread Analysis
// ============================================================

type NSMemCommitResult2217 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		MemReqGB  float64 `json:"memRequestGB"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSMemCommit2217(w http.ResponseWriter, r *http.Request) {
	result := NSMemCommitResult2217{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsMem[pod.Namespace] += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsMem)
	for ns, mem := range nsMem {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			MemReqGB  float64 `json:"memRequestGB"`
		}{ns, mem})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].MemReqGB > result.TopNS[j].MemReqGB })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Pod Capacity Headroom
type PodCapHRResult2217 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		TotalCap     int `json:"totalPodCapacity"`
		TotalPods    int `json:"runningPods"`
		HeadroomPods int `json:"headroomPods"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePodCapHR2217(w http.ResponseWriter, r *http.Request) {
	result := PodCapHRResult2217{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pods := node.Status.Allocatable.Pods()
		if pods != nil {
			result.Summary.TotalCap += int(pods.AsApproximateFloat64())
		}
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	result.Summary.HeadroomPods = result.Summary.TotalCap - result.Summary.TotalPods
	if result.Summary.HeadroomPods < 10 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Deployment Spread Analysis
type DepSpreadResult2217 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int            `json:"totalDeployments"`
		ByNS         map[string]int `json:"byNamespace"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDepSpread2217(w http.ResponseWriter, r *http.Request) {
	result := DepSpreadResult2217{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByNS = make(map[string]int)
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		result.Summary.ByNS[dep.Namespace]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
