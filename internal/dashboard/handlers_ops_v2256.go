package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.56 Operations: Pod Ready Containers Ratio, Node KubeVersion, Event Type Distribution
type ReadyCtnrRatioResult2256 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Ready           int `json:"ready"`
	} `json:"summary"`
}

func (s *Server) handleReadyCtnrRatio2256(w http.ResponseWriter, r *http.Request) {
	result := ReadyCtnrRatioResult2256{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.Ready {
				result.Summary.Ready++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeKubeVerResult2256 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKubeVer2256(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeVerResult2256{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByVersion = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByVersion[node.Status.NodeInfo.KubeletVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EvtTypeResult2256 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByType      map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleEvtType2256(w http.ResponseWriter, r *http.Request) {
	result := EvtTypeResult2256{ScannedAt: time.Now()}
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByType = make(map[string]int)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		result.Summary.ByType[evt.Type]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
