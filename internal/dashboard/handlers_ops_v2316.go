package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.16 Operations: Pod Image Pull BackOff, Node Ready Transition, Event Warning Rate
type ImgPullBackOffResult2316 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		InBackOff int `json:"inImagePullBackOff"`
	} `json:"summary"`
}

func (s *Server) handleImgPullBackOff2316(w http.ResponseWriter, r *http.Request) {
	result := ImgPullBackOffResult2316{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodPending && pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "ImagePullBackOff" {
				result.Summary.InBackOff++
				break
			}
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = 100 - (result.Summary.InBackOff*100)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeReadyTransResult2316 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		Ready      int `json:"ready"`
		NotReady   int `json:"notReady"`
	} `json:"summary"`
}

func (s *Server) handleNodeReadyTrans2316(w http.ResponseWriter, r *http.Request) {
	result := NodeReadyTransResult2316{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if ready {
			result.Summary.Ready++
		} else {
			result.Summary.NotReady++
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = result.Summary.Ready * 100 / result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type EventWarnRateResult2316 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int `json:"totalEvents"`
		Warnings    int `json:"warnings"`
		WarningPct  int `json:"warningPct"`
	} `json:"summary"`
}

func (s *Server) handleEventWarnRate2316(w http.ResponseWriter, r *http.Request) {
	result := EventWarnRateResult2316{ScannedAt: time.Now()}
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		if evt.Type == "Warning" {
			result.Summary.Warnings++
		}
	}
	if result.Summary.TotalEvents > 0 {
		result.Summary.WarningPct = result.Summary.Warnings * 100 / result.Summary.TotalEvents
	}
	score := 100
	if result.Summary.WarningPct > 50 {
		score = 50
	} else if result.Summary.WarningPct > 30 {
		score = 70
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
