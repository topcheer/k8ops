package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.08 — Operations Dimension (Round 54)
// 1. Pod Network Policy Match Count
// 2. Node Memory Allocatable vs Capacity
// 3. Container Last Termination Reason Catalog
// ============================================================

type NPMatchResult2208 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		WithNetPol int `json:"coveredByNetworkPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNPMatch2208(w http.ResponseWriter, r *http.Request) {
	result := NPMatchResult2208{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	nsHasNP := make(map[string]bool)
	for _, np := range npList.Items {
		nsHasNP[np.Namespace] = true
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if nsHasNP[pod.Namespace] {
			result.Summary.WithNetPol++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Mem Alloc vs Capacity
type MemCapAllocResult2208 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalCapGB   float64 `json:"totalCapacityGB"`
		TotalAllocGB float64 `json:"totalAllocatableGB"`
		GapPct       int     `json:"gapPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleMemCapAlloc2208(w http.ResponseWriter, r *http.Request) {
	result := MemCapAllocResult2208{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
		result.Summary.TotalAllocGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	if result.Summary.TotalCapGB > 0 {
		result.Summary.GapPct = int((1 - result.Summary.TotalAllocGB/result.Summary.TotalCapGB) * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Last Termination Reason
type LastTermReasonResult2208 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		WithLastTerm    int            `json:"withLastTermination"`
		ByReason        map[string]int `json:"byReason"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleLastTermReason2208(w http.ResponseWriter, r *http.Request) {
	result := LastTermReasonResult2208{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByReason = make(map[string]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.WithLastTerm++
				reason := cs.LastTerminationState.Terminated.Reason
				if reason == "" {
					reason = "Unknown"
				}
				result.Summary.ByReason[reason]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
