package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.74 Operations: Node CPU Allocatable vs Request, Pod Spec PriorityClassName Dist, Container Termination Signal
type NodeCPUAllocVsReqResult2574 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalCPUAllocatable"`
		TotalReq   float64 `json:"totalCPURequested"`
	}
}

func (s *Server) handleNodeCPUAllocVsReq2574(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUAllocVsReqResult2574{ScannedAt: time.Now()}
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
			result.Summary.TotalReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PriorityClassDistResult2574 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPC      map[string]int `json:"byPriorityClassName"`
	}
}

func (s *Server) handlePriorityClassDist2574(w http.ResponseWriter, r *http.Request) {
	result := PriorityClassDistResult2574{ScannedAt: time.Now()}
	result.Summary.ByPC = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		pc := pod.Spec.PriorityClassName
		if pc == "" {
			pc = "<none>"
		}
		result.Summary.ByPC[pc]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TermSignalResult2574 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		BySignal        map[string]int `json:"bySignal"`
	}
}

func (s *Server) handleTermSignal2574(w http.ResponseWriter, r *http.Request) {
	result := TermSignalResult2574{ScannedAt: time.Now()}
	result.Summary.BySignal = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalContainers++
				sig := intToStr(int(cs.LastTerminationState.Terminated.Signal))
				result.Summary.BySignal[sig]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
