package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.76 Operations: Pod QoS Guaranteed Ratio, Node Kernel Version, Event Reason Catalog
type QoSGuaranteedResult2376 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByQoS     map[string]int `json:"byQoSClass"`
	} `json:"summary"`
}

func (s *Server) handleQoSGuaranteed2376(w http.ResponseWriter, r *http.Request) {
	result := QoSGuaranteedResult2376{ScannedAt: time.Now()}
	result.Summary.ByQoS = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByQoS[string(pod.Status.QOSClass)]++
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		g := result.Summary.ByQoS["Guaranteed"]
		score = 50 + (g*50)/result.Summary.TotalPods
		if score > 100 {
			score = 100
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type NodeKernelResult2376 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKernel   map[string]int `json:"byKernelVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKernel2376(w http.ResponseWriter, r *http.Request) {
	result := NodeKernelResult2376{ScannedAt: time.Now()}
	result.Summary.ByKernel = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByKernel[node.Status.NodeInfo.KernelVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventReasonResult2376 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByReason    map[string]int `json:"byReason"`
	} `json:"summary"`
}

func (s *Server) handleEventReason2376(w http.ResponseWriter, r *http.Request) {
	result := EventReasonResult2376{ScannedAt: time.Now()}
	result.Summary.ByReason = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		result.Summary.ByReason[evt.Reason]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
