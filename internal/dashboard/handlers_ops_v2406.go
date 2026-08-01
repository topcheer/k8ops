package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.06 Operations: Pod Container Restarts High, Node BootID Census, Event Involved Object Kind
type HighRestartsResult2406 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		HighRestarts    int `json:"highRestarts"`
	} `json:"summary"`
}

func (s *Server) handleHighRestarts2406(w http.ResponseWriter, r *http.Request) {
	result := HighRestartsResult2406{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.RestartCount > 5 {
				result.Summary.HighRestarts++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.HighRestarts*50)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type BootIDResult2406 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		UniqueBoots int `json:"uniqueBootIDs"`
	} `json:"summary"`
}

func (s *Server) handleBootID2406(w http.ResponseWriter, r *http.Request) {
	result := BootIDResult2406{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		seen[node.Status.NodeInfo.BootID] = true
	}
	result.Summary.UniqueBoots = len(seen)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventObjKindResult2406 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByKind      map[string]int `json:"byInvolvedObjectKind"`
	} `json:"summary"`
}

func (s *Server) handleEventObjKind2406(w http.ResponseWriter, r *http.Request) {
	result := EventObjKindResult2406{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		result.Summary.ByKind[evt.InvolvedObject.Kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
