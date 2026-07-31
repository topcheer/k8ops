package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.68 Operations: Pod Phase Distribution, Node Container Runtime, Container State Census
type PodPhaseResult2268 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPhase   map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handlePodPhase2268(w http.ResponseWriter, r *http.Request) {
	result := PodPhaseResult2268{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		result.Summary.ByPhase[string(pod.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeRuntimeResult2268 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRuntime  map[string]int `json:"byContainerRuntime"`
	} `json:"summary"`
}

func (s *Server) handleNodeRuntime2268(w http.ResponseWriter, r *http.Request) {
	result := NodeRuntimeResult2268{ScannedAt: time.Now()}
	result.Summary.ByRuntime = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByRuntime[node.Status.NodeInfo.ContainerRuntimeVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrStateResult2268 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByState         map[string]int `json:"byState"`
	} `json:"summary"`
}

func (s *Server) handleCtnrState2268(w http.ResponseWriter, r *http.Request) {
	result := CtnrStateResult2268{ScannedAt: time.Now()}
	result.Summary.ByState = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Running != nil {
				result.Summary.ByState["running"]++
			} else if cs.State.Waiting != nil {
				result.Summary.ByState["waiting"]++
			} else if cs.State.Terminated != nil {
				result.Summary.ByState["terminated"]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
