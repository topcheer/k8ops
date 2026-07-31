package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.98 Operations: Node Network Unavailable, Pod Ready Transition, Event InvolvedObject Census
type NetUnavailableResult2298 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		WithNetIssue int `json:"withNetworkUnavailable"`
	} `json:"summary"`
}

func (s *Server) handleNetUnavailable2298(w http.ResponseWriter, r *http.Request) {
	result := NetUnavailableResult2298{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeNetworkUnavailable && cond.Status == corev1.ConditionTrue {
				result.Summary.WithNetIssue++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.WithNetIssue*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ReadyTransitionResult2298 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Ready     int `json:"ready"`
		NotReady  int `json:"notReady"`
	} `json:"summary"`
}

func (s *Server) handleReadyTransition2298(w http.ResponseWriter, r *http.Request) {
	result := ReadyTransitionResult2298{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		isReady := true
		for _, cs := range pod.Status.Conditions {
			if cs.Type == corev1.PodReady && cs.Status != corev1.ConditionTrue {
				isReady = false
			}
		}
		if isReady {
			result.Summary.Ready++
		} else {
			result.Summary.NotReady++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = result.Summary.Ready * 100 / result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type EventInvObjResult2298 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEvents int            `json:"totalEvents"`
		ByKind      map[string]int `json:"byInvolvedObjectKind"`
	} `json:"summary"`
}

func (s *Server) handleEventInvObj2298(w http.ResponseWriter, r *http.Request) {
	result := EventInvObjResult2298{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		result.Summary.ByKind[evt.InvolvedObject.Kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
