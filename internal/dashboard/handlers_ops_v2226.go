package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.26 — Operations Dimension (Round 57)
// 1. Pod Container Termination Signal Distribution
// 2. Node ImageGCPressure Detector
// 3. Service Port Protocol Coverage
// ============================================================

type TermSignalResult2226 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int         `json:"totalContainers"`
		BySignal        map[int]int `json:"byTerminationSignal"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleTermSignal2226(w http.ResponseWriter, r *http.Request) {
	result := TermSignalResult2226{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.BySignal = make(map[int]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Terminated != nil {
				result.Summary.BySignal[int(cs.State.Terminated.Signal)]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. ImageGCPressure Detector
type ImgGCResult2226 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int `json:"totalNodes"`
		WithGCPressure int `json:"withImageGCPressure"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgGC2226(w http.ResponseWriter, r *http.Request) {
	result := ImgGCResult2226{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if string(cond.Reason) == "ImageGCFailed" || string(cond.Message) == "ImageGCFailed" {
				result.Summary.WithGCPressure++
				score -= 5
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

// 3. Port Protocol Coverage
type PortProtoCovResult2226 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByProtocol    map[string]int `json:"byProtocol"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePortProtoCov2226(w http.ResponseWriter, r *http.Request) {
	result := PortProtoCovResult2226{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByProtocol = make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, p := range svc.Spec.Ports {
			result.Summary.ByProtocol[string(p.Protocol)]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
