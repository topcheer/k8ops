package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.11 — Scalability & HA Dimension (Round 54)
// 1. Namespace CPU Commit Distribution
// 2. Node Pod Scheduling Gap Score
// 3. Cluster Image Pull Efficiency
// ============================================================

type NSCPUCommitResult2211 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPUReq    float64 `json:"cpuRequest"`
		PodCount  int     `json:"podCount"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSCPUCommit2211(w http.ResponseWriter, r *http.Request) {
	result := NSCPUCommitResult2211{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsCPU := make(map[string]float64)
	nsPods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nsPods[pod.Namespace]++
		for _, c := range pod.Spec.Containers {
			nsCPU[pod.Namespace] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsCPU)
	for ns := range nsCPU {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPUReq    float64 `json:"cpuRequest"`
			PodCount  int     `json:"podCount"`
		}{ns, nsCPU[ns], nsPods[ns]})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPUReq > result.TopNS[j].CPUReq })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Pod Scheduling Gap
type SchedGapResult2211 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalPods  int `json:"totalPods"`
		TotalCap   int `json:"totalPodCapacity"`
		GapPods    int `json:"gapPods"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSchedGap2211(w http.ResponseWriter, r *http.Request) {
	result := SchedGapResult2211{ScannedAt: time.Now()}
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
	result.Summary.GapPods = result.Summary.TotalCap - result.Summary.TotalPods
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Image Pull Efficiency
type ImgPullEffResult2211 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods        int `json:"totalPods"`
		TotalImages      int `json:"totalUniqueImages"`
		PullPolicyAlways int `json:"pullPolicyAlways"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgPullEff2211(w http.ResponseWriter, r *http.Request) {
	result := ImgPullEffResult2211{ScannedAt: time.Now()}
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
			if c.ImagePullPolicy == corev1.PullAlways {
				result.Summary.PullPolicyAlways++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
