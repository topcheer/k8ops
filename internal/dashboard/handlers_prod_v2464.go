package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.64 Product: Pod NodeSelector Count, Container Resource Limit Distribution, Service ExternalName Count
type NodeSelectorCountResult2464 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		TotalSel  int `json:"totalNodeSelectors"`
	} `json:"summary"`
}

func (s *Server) handleNodeSelectorCount2464(w http.ResponseWriter, r *http.Request) {
	result := NodeSelectorCountResult2464{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalSel += len(pod.Spec.NodeSelector)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResourceLimitDistResult2464 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCPULimit    int `json:"withCPULimit"`
		WithMemLimit    int `json:"withMemLimit"`
	} `json:"summary"`
}

func (s *Server) handleResourceLimitDist2464(w http.ResponseWriter, r *http.Request) {
	result := ResourceLimitDistResult2464{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
				result.Summary.WithCPULimit++
			}
			if _, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
				result.Summary.WithMemLimit++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		cpuRatio := result.Summary.WithCPULimit * 100 / result.Summary.TotalContainers
		memRatio := result.Summary.WithMemLimit * 100 / result.Summary.TotalContainers
		if cpuRatio < memRatio {
			score = cpuRatio
		} else {
			score = memRatio
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ExternalNameResult2464 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs    int `json:"totalServices"`
		ExternalName int `json:"externalNameCount"`
	} `json:"summary"`
}

func (s *Server) handleExternalName2464(w http.ResponseWriter, r *http.Request) {
	result := ExternalNameResult2464{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Summary.ExternalName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
