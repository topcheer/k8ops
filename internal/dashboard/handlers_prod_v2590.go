package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.90 Product: Pod Spec ReadinessGates, Container WorkingDir Dist, Service SessionAffinity
type ReadinessGatesResult2590 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByGate    map[string]int `json:"byReadinessGate"`
	}
}

func (s *Server) handleReadinessGates2590(w http.ResponseWriter, r *http.Request) {
	result := ReadinessGatesResult2590{ScannedAt: time.Now()}
	result.Summary.ByGate = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, gate := range pod.Spec.ReadinessGates {
			result.Summary.ByGate[string(gate.ConditionType)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type WorkingDirResult2590 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByDir           map[string]int `json:"byWorkingDir"`
	}
}

func (s *Server) handleWorkingDir2590(w http.ResponseWriter, r *http.Request) {
	result := WorkingDirResult2590{ScannedAt: time.Now()}
	result.Summary.ByDir = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			dir := c.WorkingDir
			if dir == "" {
				dir = "<default>"
			}
			result.Summary.ByDir[dir]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SessionAffinityResult2590 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int            `json:"totalServices"`
		ByAffinity map[string]int `json:"bySessionAffinity"`
	}
}

func (s *Server) handleSessionAffinity2590(w http.ResponseWriter, r *http.Request) {
	result := SessionAffinityResult2590{ScannedAt: time.Now()}
	result.Summary.ByAffinity = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.ByAffinity[string(svc.Spec.SessionAffinity)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
