package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.84 — Operations Dimension (Round 50)
// 1. Pod Container Wait State Catalog
// 2. Node Kernel Version Distribution
// 3. Service ClusterIP Range Utilization
// ============================================================

type WaitStateResult2184 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers   int            `json:"totalContainers"`
		WaitingContainers int            `json:"waitingContainers"`
		ByReason          map[string]int `json:"byReason"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleWaitState2184(w http.ResponseWriter, r *http.Request) {
	result := WaitStateResult2184{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByReason = make(map[string]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Waiting != nil {
				result.Summary.WaitingContainers++
				reason := cs.State.Waiting.Reason
				if reason == "" {
					reason = "Unknown"
				}
				result.Summary.ByReason[reason]++
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Kernel Version Distribution
type KernelVerResult2184 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKernel   map[string]int `json:"byKernelVersion"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleKernelVer2184(w http.ResponseWriter, r *http.Request) {
	result := KernelVerResult2184{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByKernel = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByKernel[node.Status.NodeInfo.KernelVersion]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. ClusterIP Range Utilization
type ClusterIPUtilResult2184 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithClusterIP int `json:"withClusterIP"`
		WithNone      int `json:"headlessServices"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleClusterIPUtil2184(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPUtilResult2184{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
			result.Summary.WithClusterIP++
		} else {
			result.Summary.WithNone++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
