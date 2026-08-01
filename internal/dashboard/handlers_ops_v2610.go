package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.10 Operations: Node Taint Effect Distribution, Pod Spec Restart Policy Detail, Container Lifecycle Hook
type NodeTaintEffect2610Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		TotalTaints int `json:"totalTaints"`
	} `json:"summary"`
}

func (s *Server) handleNodeTaintEffect2610(w http.ResponseWriter, r *http.Request) {
	result := NodeTaintEffect2610Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalTaints += len(node.Spec.Taints)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RestartPolicy2610Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byRestartPolicy"`
	} `json:"summary"`
}

func (s *Server) handleRestartPolicy2610(w http.ResponseWriter, r *http.Request) {
	result := RestartPolicy2610Result{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByPolicy[string(pod.Spec.RestartPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LifecycleHook2610Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithPostStart   int `json:"withPostStart"`
		WithPreStop     int `json:"withPreStop"`
	} `json:"summary"`
}

func (s *Server) handleLifecycleHook2610(w http.ResponseWriter, r *http.Request) {
	result := LifecycleHook2610Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Lifecycle != nil {
				if c.Lifecycle.PostStart != nil {
					result.Summary.WithPostStart++
				}
				if c.Lifecycle.PreStop != nil {
					result.Summary.WithPreStop++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
