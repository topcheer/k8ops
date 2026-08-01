package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.36 Operations: Pod QoS Guaranteed Ratio, Node Cond Network, Container Termination Message
type QoSRatioResult2436 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByQoS     map[string]int `json:"byQoSClass"`
	} `json:"summary"`
}

func (s *Server) handleQoSRatio2436(w http.ResponseWriter, r *http.Request) {
	result := QoSRatioResult2436{ScannedAt: time.Now()}
	result.Summary.ByQoS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByQoS[string(pod.Status.QOSClass)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCondNetResult2436 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int `json:"totalNodes"`
		NetUnavailable int `json:"networkUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondNet2436(w http.ResponseWriter, r *http.Request) {
	result := NodeCondNetResult2436{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionTrue {
				result.Summary.NetUnavailable++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.NetUnavailable*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type TermMsgResult2436 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByPolicy        map[string]int `json:"byTerminationMessagePolicy"`
	} `json:"summary"`
}

func (s *Server) handleTermMsg2436(w http.ResponseWriter, r *http.Request) {
	result := TermMsgResult2436{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.ByPolicy[string(c.TerminationMessagePolicy)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
