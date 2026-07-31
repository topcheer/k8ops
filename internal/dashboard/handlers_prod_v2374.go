package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.74 Product: Pod Priority Audit, Container Readiness Probe Exist, Service TargetPort Custom
type PriorityResult2374 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int            `json:"totalPods"`
		ByPriority map[string]int `json:"byPriorityClass"`
	} `json:"summary"`
}

func (s *Server) handlePriority2374(w http.ResponseWriter, r *http.Request) {
	result := PriorityResult2374{ScannedAt: time.Now()}
	result.Summary.ByPriority = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		pc := pod.Spec.PriorityClassName
		if pc == "" {
			pc = "<default>"
		}
		result.Summary.ByPriority[pc]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ReadinessExistResult2374 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithReadiness   int `json:"withReadinessProbe"`
	} `json:"summary"`
}

func (s *Server) handleReadinessExist2374(w http.ResponseWriter, r *http.Request) {
	result := ReadinessExistResult2374{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.ReadinessProbe != nil {
				result.Summary.WithReadiness++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.WithReadiness * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type TargetPortCustomResult2374 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		TotalPorts    int `json:"totalPorts"`
	} `json:"summary"`
}

func (s *Server) handleTargetPortCustom2374(w http.ResponseWriter, r *http.Request) {
	result := TargetPortCustomResult2374{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.TotalPorts += len(svc.Spec.Ports)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
