package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.34 Operations: Node Status Conditions True Count, Pod Spec TopologySpreadConstraints Summary, Container EnvFrom ConfigMap
type NodeCondTrue2634Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalTrue  int `json:"totalConditionsTrue"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondTrue2634(w http.ResponseWriter, r *http.Request) {
	result := NodeCondTrue2634Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				result.Summary.TotalTrue++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TopologySpread2634Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByDomain  map[string]int `json:"byTopologyDomain"`
	} `json:"summary"`
}

func (s *Server) handleTopologySpread2634(w http.ResponseWriter, r *http.Request) {
	result := TopologySpread2634Result{ScannedAt: time.Now()}
	result.Summary.ByDomain = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, tsc := range pod.Spec.TopologySpreadConstraints {
			result.Summary.ByDomain[tsc.TopologyKey]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EnvFromCM2634Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithEnvFromCM   int `json:"withEnvFromConfigMap"`
	} `json:"summary"`
}

func (s *Server) handleEnvFromCM2634(w http.ResponseWriter, r *http.Request) {
	result := EnvFromCM2634Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, ef := range c.EnvFrom {
				if ef.ConfigMapRef != nil {
					result.Summary.WithEnvFromCM++
					break
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
