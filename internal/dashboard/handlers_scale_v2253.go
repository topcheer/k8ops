package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.53 — Scalability & HA Dimension (Round 61)
// 1. Namespace Pod Density vs Limit Score
// 2. Node CPU Limit Commit Ratio
// 3. Cluster PVC Bound vs Pending Ratio
// ============================================================

type NSPodDensityResult2253 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		PodCount  int    `json:"podCount"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSPodDensity2253(w http.ResponseWriter, r *http.Request) {
	result := NSPodDensityResult2253{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsPods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			nsPods[pod.Namespace]++
		}
	}
	result.Summary.TotalNS = len(nsPods)
	for ns, cnt := range nsPods {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			PodCount  int    `json:"podCount"`
		}{ns, cnt})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].PodCount > result.TopNS[j].PodCount })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node CPU Limit Commit Ratio
type CPULimCommitResult2253 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableCPU"`
		TotalLim   float64 `json:"totalLimitedCPU"`
		CommitPct  int     `json:"commitPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCPULimCommit2253(w http.ResponseWriter, r *http.Request) {
	result := CPULimCommitResult2253{ScannedAt: time.Now()}
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
	if result.Summary.TotalAlloc > 0 {
		result.Summary.CommitPct = int(result.Summary.TotalLim / result.Summary.TotalAlloc * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. PVC Bound vs Pending
type PVCBoundPendingResult2253 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int `json:"totalPVCs"`
		Bound     int `json:"bound"`
		Pending   int `json:"pending"`
		BoundPct  int `json:"boundPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCBoundPending2253(w http.ResponseWriter, r *http.Request) {
	result := PVCBoundPendingResult2253{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.Bound++
		} else {
			result.Summary.Pending++
		}
	}
	if result.Summary.TotalPVCs > 0 {
		result.Summary.BoundPct = result.Summary.Bound * 100 / result.Summary.TotalPVCs
	}
	if result.Summary.Pending > 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
