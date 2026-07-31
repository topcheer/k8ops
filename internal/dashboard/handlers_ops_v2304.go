package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.04 Operations: Pod Scheduling Gate Audit, Node KubeProxy Version, Container Started State Census
type SchedGateResult2304 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithGates int `json:"withSchedulingGates"`
	} `json:"summary"`
}

func (s *Server) handleSchedGate2304(w http.ResponseWriter, r *http.Request) {
	result := SchedGateResult2304{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodPending {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.SchedulingGates) > 0 {
			result.Summary.WithGates++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type KubeProxyResult2304 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeProxyVersion"`
	} `json:"summary"`
}

func (s *Server) handleKubeProxy2304(w http.ResponseWriter, r *http.Request) {
	result := KubeProxyResult2304{ScannedAt: time.Now()}
	result.Summary.ByVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByVersion[node.Status.NodeInfo.KubeProxyVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StartedStateResult2304 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Started         int `json:"started"`
		NotStarted      int `json:"notStarted"`
	} `json:"summary"`
}

func (s *Server) handleStartedState2304(w http.ResponseWriter, r *http.Request) {
	result := StartedStateResult2304{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.Started != nil && *cs.Started {
				result.Summary.Started++
			} else {
				result.Summary.NotStarted++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.Started * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
