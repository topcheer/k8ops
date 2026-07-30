package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.14 — Operations Dimension (Round 55)
// 1. Pod Node Selector Key Distribution
// 2. Node Network Unavailable Detector
// 3. Container State Running Ratio
// ============================================================

type NodeSelKeyResult2214 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int            `json:"totalPods"`
		WithSelector int            `json:"withNodeSelector"`
		ByKey        map[string]int `json:"bySelectorKey"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeSelKey2214(w http.ResponseWriter, r *http.Request) {
	result := NodeSelKeyResult2214{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByKey = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for k := range pod.Spec.NodeSelector {
			result.Summary.WithSelector++
			result.Summary.ByKey[k]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Network Unavailable
type NetUnavailableResult2214 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int `json:"totalNodes"`
		NetUnavailable int `json:"networkUnavailable"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNetUnavailable2214(w http.ResponseWriter, r *http.Request) {
	result := NetUnavailableResult2214{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionTrue {
				result.Summary.NetUnavailable++
				score -= 15
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

// 3. Container Running Ratio
type CtnrRunningRatioResult2214 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Running         int `json:"running"`
		Waiting         int `json:"waiting"`
		Terminated      int `json:"terminated"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCtnrRunningRatio2214(w http.ResponseWriter, r *http.Request) {
	result := CtnrRunningRatioResult2214{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.State.Running != nil {
				result.Summary.Running++
			}
			if cs.State.Waiting != nil {
				result.Summary.Waiting++
			}
			if cs.State.Terminated != nil {
				result.Summary.Terminated++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
