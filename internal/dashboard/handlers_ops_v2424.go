package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v24.24 Operations: Pod Completed Status, Node Cond OutofDisk, Container Image Latest Count
type PodCompletedResult2424 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Completed int `json:"completedPhase"`
	} `json:"summary"`
}

func (s *Server) handlePodCompleted2424(w http.ResponseWriter, r *http.Request) {
	result := PodCompletedResult2424{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodSucceeded {
			result.Summary.Completed++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeOutOfDiskResult2424 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		OutOfDisk  int `json:"outOfDisk"`
	} `json:"summary"`
}

func (s *Server) handleNodeOutOfDisk2424(w http.ResponseWriter, r *http.Request) {
	result := NodeOutOfDiskResult2424{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
				result.Summary.OutOfDisk++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.OutOfDisk*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ImageLatestResult2424 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages  int `json:"totalImages"`
		LatestImages int `json:"latestImages"`
	} `json:"summary"`
}

func (s *Server) handleImageLatest2424(w http.ResponseWriter, r *http.Request) {
	result := ImageLatestResult2424{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.TotalImages++
				if strings.HasSuffix(c.Image, ":latest") || !strings.Contains(c.Image, ":") {
					result.Summary.LatestImages++
				}
			}
		}
	}
	score := 100
	if result.Summary.TotalImages > 0 && result.Summary.LatestImages > 0 {
		score = 100 - (result.Summary.LatestImages*50)/result.Summary.TotalImages
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
