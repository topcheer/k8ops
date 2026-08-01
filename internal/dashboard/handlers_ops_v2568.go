package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.68 Operations: Node Status Conditions Summary, Pod Spec Tolerations Count, Container Resource EphemeralStorage Request
type NodeCondSummaryResult2568 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByCond     map[string]int `json:"byCondition"`
	}
}

func (s *Server) handleNodeCondSummary2568(w http.ResponseWriter, r *http.Request) {
	result := NodeCondSummaryResult2568{ScannedAt: time.Now()}
	result.Summary.ByCond = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Status == corev1.ConditionFalse {
				result.Summary.ByCond[string(cond.Type)+":False"]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodTolerationsCountResult2568 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		TotalToler int `json:"totalTolerations"`
	}
}

func (s *Server) handlePodTolerationsCount2568(w http.ResponseWriter, r *http.Request) {
	result := PodTolerationsCountResult2568{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalToler += len(pod.Spec.Tolerations)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EphemeralReqResult2568 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithEphemeral   int `json:"withEphemeralReq"`
	}
}

func (s *Server) handleEphemeralReq2568(w http.ResponseWriter, r *http.Request) {
	result := EphemeralReqResult2568{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if _, ok := c.Resources.Requests[corev1.ResourceEphemeralStorage]; ok {
				result.Summary.WithEphemeral++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
