package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.32 — Operations Dimension (Round 58)
// 1. Pod Active Deadline Seconds Tracker
// 2. Node Kubelet Ready vs Network Status
// 3. Container Image Pull Backoff Detector
// ============================================================

type ActiveDeadlineResult2232 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithDeadline int `json:"withActiveDeadlineSeconds"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleActiveDeadline2232(w http.ResponseWriter, r *http.Request) {
	result := ActiveDeadlineResult2232{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ActiveDeadlineSeconds != nil {
			result.Summary.WithDeadline++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Kubelet Ready vs Network
type KubeletNetResult2232 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		Ready      int `json:"kubeletReady"`
		NetIssue   int `json:"withNetworkIssue"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleKubeletNet2232(w http.ResponseWriter, r *http.Request) {
	result := KubeletNetResult2232{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				result.Summary.Ready++
			}
			if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionTrue {
				result.Summary.NetIssue++
				score -= 10
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

// 3. Image Pull Backoff Detector
type ImgPullBackoffResult2232 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		InBackoff       int `json:"inImagePullBackoff"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleImgPullBackoff2232(w http.ResponseWriter, r *http.Request) {
	result := ImgPullBackoffResult2232{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Waiting != nil && containsStr2039(cs.State.Waiting.Reason, "ImagePullBackOff") {
				result.Summary.InBackoff++
				score -= 3
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
