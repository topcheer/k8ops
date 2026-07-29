package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.99 — Operations Dimension (Round 36)
// 1. Container State Distribution — running/waiting/terminated
// 2. Node Container Runtime Map — runtime per node
// 3. Pod QoS Eviction Risk — BestEffort eviction priority
// ============================================================

type CtnrStateResult2099 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CtnrStateSummary2099 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type CtnrStateSummary2099 struct {
	TotalContainers int `json:"totalContainers"`
	Running         int `json:"running"`
	Waiting         int `json:"waiting"`
	Terminated      int `json:"terminated"`
}

func (s *Server) handleCtnrState2099(w http.ResponseWriter, r *http.Request) {
	result := CtnrStateResult2099{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Running != nil {
				result.Summary.Running++
			}
			if cs.State.Waiting != nil {
				result.Summary.Waiting++
			}
			if cs.State.Terminated != nil {
				result.Summary.Terminated++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Container Runtime Map
type RuntimeResult2099 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         RuntimeSummary2099 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type RuntimeSummary2099 struct {
	TotalNodes int            `json:"totalNodes"`
	Runtimes   map[string]int `json:"containerRuntimes"`
}

func (s *Server) handleRuntime2099(w http.ResponseWriter, r *http.Request) {
	result := RuntimeResult2099{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	runtimes := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		runtimes[node.Status.NodeInfo.ContainerRuntimeVersion]++
	}
	result.Summary.Runtimes = runtimes
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod QoS Eviction Risk
type QoSEvictResult2099 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         QoSEvictSummary2099 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type QoSEvictSummary2099 struct {
	TotalPods  int `json:"totalPods"`
	BestEffort int `json:"bestEffortPods"`
	Burstable  int `json:"burstablePods"`
}

func (s *Server) handleQoSEvict2099(w http.ResponseWriter, r *http.Request) {
	result := QoSEvictResult2099{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		allReq := true
		allLim := true
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests.Cpu().IsZero() {
				allReq = false
			}
			if c.Resources.Limits.Cpu().IsZero() {
				allLim = false
			}
		}
		if !allReq && !allLim {
			result.Summary.BestEffort++
		} else {
			result.Summary.Burstable++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
