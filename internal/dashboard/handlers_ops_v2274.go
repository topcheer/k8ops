package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.74 Operations: Node Kernel Version Census, Pod Termination Grace Period, Event Reason Top
type KernelVersionResult2274 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKernel   map[string]int `json:"byKernelVersion"`
	} `json:"summary"`
}

func (s *Server) handleKernelVersion2274(w http.ResponseWriter, r *http.Request) {
	result := KernelVersionResult2274{ScannedAt: time.Now()}
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

type GracePeriodResult2274 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		DefaultSec int `json:"defaultSec"`
		CustomSec  int `json:"customSec"`
		ZeroSec    int `json:"zeroSec"`
	} `json:"summary"`
}

func (s *Server) handleGracePeriod2274(w http.ResponseWriter, r *http.Request) {
	result := GracePeriodResult2274{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		gp := int64(30) // default
		if pod.Spec.TerminationGracePeriodSeconds != nil {
			gp = *pod.Spec.TerminationGracePeriodSeconds
		}
		if gp == 0 {
			result.Summary.ZeroSec++
		} else if gp == 30 {
			result.Summary.DefaultSec++
		} else {
			result.Summary.CustomSec++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EventReasonTopResult2274 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByReason    map[string]int `json:"byReason"`
	} `json:"summary"`
}

func (s *Server) handleEventReasonTop2274(w http.ResponseWriter, r *http.Request) {
	result := EventReasonTopResult2274{ScannedAt: time.Now()}
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
