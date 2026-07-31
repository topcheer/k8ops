package dashboard

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.58 Operations: Pod Container Terminated Signal, Node Kubelet Version, Event Type Distribution
type TermSignalResult2358 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalTerminated int            `json:"totalTerminated"`
		BySignal        map[string]int `json:"bySignal"`
	} `json:"summary"`
}

func (s *Server) handleTermSignal2358(w http.ResponseWriter, r *http.Request) {
	result := TermSignalResult2358{ScannedAt: time.Now()}
	result.Summary.BySignal = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalTerminated++
				result.Summary.BySignal[fmt.Sprintf("%d", cs.LastTerminationState.Terminated.Signal)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type KubeletVerResult2358 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeletVersion"`
	} `json:"summary"`
}

func (s *Server) handleKubeletVer2358(w http.ResponseWriter, r *http.Request) {
	result := KubeletVerResult2358{ScannedAt: time.Now()}
	result.Summary.ByVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByVersion[node.Status.NodeInfo.KubeletVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventTypeResult2358 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByType      map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleEventType2358(w http.ResponseWriter, r *http.Request) {
	result := EventTypeResult2358{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		t := evt.Type
		if t == "" {
			t = "Normal"
		}
		result.Summary.ByType[t]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
