package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.90 Product: Pod Preemption History, Container Stdin TTY Audit, Service LoadBalancer Health
type PreemptionResult2290 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Preempted int `json:"preempted"`
	} `json:"summary"`
}

func (s *Server) handlePreemption2290(w http.ResponseWriter, r *http.Request) {
	result := PreemptionResult2290{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Status.NominatedNodeName != "" {
			result.Summary.Preempted++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StdinTTYResult2290 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdin       int `json:"withStdin"`
		WithTTY         int `json:"withTTY"`
	} `json:"summary"`
}

func (s *Server) handleStdinTTY2290(w http.ResponseWriter, r *http.Request) {
	result := StdinTTYResult2290{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Stdin {
				result.Summary.WithStdin++
			}
			if c.TTY {
				result.Summary.WithTTY++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LBHealthResult2290 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLBSvc  int `json:"totalLoadBalancerServices"`
		WithIngress int `json:"withIngressIP"`
	} `json:"summary"`
}

func (s *Server) handleLBHealth2290(w http.ResponseWriter, r *http.Request) {
	result := LBHealthResult2290{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLBSvc++
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			result.Summary.WithIngress++
		}
	}
	score := 100
	if result.Summary.TotalLBSvc > 0 && result.Summary.WithIngress < result.Summary.TotalLBSvc {
		score = result.Summary.WithIngress * 100 / result.Summary.TotalLBSvc
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
