package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.62 Operations: Pod Restart Policy Distribution, Node Architecture Census, Container Privileged Escalation
type RestartPolicyResult2262 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int            `json:"totalPods"`
		ByRestartPolicy map[string]int `json:"byRestartPolicy"`
	} `json:"summary"`
}

func (s *Server) handleRestartPolicy2262(w http.ResponseWriter, r *http.Request) {
	result := RestartPolicyResult2262{ScannedAt: time.Now()}
	result.Summary.ByRestartPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByRestartPolicy[string(pod.Spec.RestartPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeArchResult2262 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByArch     map[string]int `json:"byArchitecture"`
	} `json:"summary"`
}

func (s *Server) handleNodeArch2262(w http.ResponseWriter, r *http.Request) {
	result := NodeArchResult2262{ScannedAt: time.Now()}
	result.Summary.ByArch = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByArch[node.Status.NodeInfo.Architecture]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PrivilegedEscResult2262 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		Privileged      int `json:"privileged"`
	} `json:"summary"`
}

func (s *Server) handlePrivilegedEsc2262(w http.ResponseWriter, r *http.Request) {
	result := PrivilegedEscResult2262{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.Privileged++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 && result.Summary.Privileged > 0 {
		score = 100 - (result.Summary.Privileged*100)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
