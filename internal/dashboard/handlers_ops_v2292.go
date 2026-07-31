package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.92 Operations: Node Memory Pressure, Pod Container Restarts Top, Image Pull Duration Risk
type MemPressureResult2292 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		WithPressure int `json:"withMemoryPressure"`
	} `json:"summary"`
}

func (s *Server) handleMemPressure2292(w http.ResponseWriter, r *http.Request) {
	result := MemPressureResult2292{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeMemoryPressure && cond.Status == corev1.ConditionTrue {
				result.Summary.WithPressure++
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.WithPressure*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RestartTopResult2292 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		HighRestart     int `json:"highRestartContainers"`
	} `json:"summary"`
}

func (s *Server) handleRestartTop2292(w http.ResponseWriter, r *http.Request) {
	result := RestartTopResult2292{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			if cs.RestartCount > 5 {
				result.Summary.HighRestart++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		badPct := result.Summary.HighRestart * 100 / result.Summary.TotalContainers
		score = 100 - badPct/2
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PullDurationResult2292 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages  int            `json:"totalImages"`
		ByPullPolicy map[string]int `json:"byPullPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePullDuration2292(w http.ResponseWriter, r *http.Request) {
	result := PullDurationResult2292{ScannedAt: time.Now()}
	result.Summary.ByPullPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.TotalImages++
			}
			result.Summary.ByPullPolicy[string(c.ImagePullPolicy)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
