package dashboard

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.70 Operations: Pod Condition Type Census, Node OS Name, Container Termination Exit Code
type PodCondTypeResult2370 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalConditions int            `json:"totalConditions"`
		ByType          map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handlePodCondType2370(w http.ResponseWriter, r *http.Request) {
	result := PodCondTypeResult2370{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cond := range pod.Status.Conditions {
			result.Summary.TotalConditions++
			result.Summary.ByType[string(cond.Type)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeOSNameResult2370 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOS       map[string]int `json:"byOperatingSystem"`
	} `json:"summary"`
}

func (s *Server) handleNodeOSName2370(w http.ResponseWriter, r *http.Request) {
	result := NodeOSNameResult2370{ScannedAt: time.Now()}
	result.Summary.ByOS = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByOS[node.Status.NodeInfo.OperatingSystem]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TermExitCodeResult2370 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalTerminated int            `json:"totalTerminated"`
		ByExitCode      map[string]int `json:"byExitCode"`
	} `json:"summary"`
}

func (s *Server) handleTermExitCode2370(w http.ResponseWriter, r *http.Request) {
	result := TermExitCodeResult2370{ScannedAt: time.Now()}
	result.Summary.ByExitCode = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalTerminated++
				result.Summary.ByExitCode[fmt.Sprintf("%d", cs.LastTerminationState.Terminated.ExitCode)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
