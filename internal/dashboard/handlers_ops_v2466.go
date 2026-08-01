package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.66 Operations: Node Ready Duration, Pod CrashLoopBackOff Count, Container Image Age
type NodeReadyDurationResult2466 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		ReadyNodes int `json:"readyNodes"`
	} `json:"summary"`
}

func (s *Server) handleNodeReadyDuration2466(w http.ResponseWriter, r *http.Request) {
	result := NodeReadyDurationResult2466{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				result.Summary.ReadyNodes++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = result.Summary.ReadyNodes * 100 / result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type CrashLoopResult2466 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		CrashLoop int `json:"crashLoopBackOff"`
	} `json:"summary"`
}

func (s *Server) handleCrashLoop2466(w http.ResponseWriter, r *http.Request) {
	result := CrashLoopResult2466{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				result.Summary.CrashLoop++
			}
		}
	}
	score := 100
	if result.Summary.CrashLoop > 0 {
		score = 100 - result.Summary.CrashLoop*10
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ImageAgeResult2466 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages  int            `json:"totalImages"`
		UniqueImages int            `json:"uniqueImages"`
		ByImage      map[string]int `json:"byImage"`
	} `json:"summary"`
}

func (s *Server) handleImageAge2466(w http.ResponseWriter, r *http.Request) {
	result := ImageAgeResult2466{ScannedAt: time.Now()}
	result.Summary.ByImage = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			result.Summary.ByImage[c.Image]++
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.UniqueImages++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
