package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.86 Operations: Pod CrashLoop Detection, Node Disk Pressure, Container Restart Distribution
type CrashLoopResult2286 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		InCrashLoop int `json:"inCrashLoop"`
	} `json:"summary"`
}

func (s *Server) handleCrashLoop2286(w http.ResponseWriter, r *http.Request) {
	result := CrashLoopResult2286{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		result.Summary.TotalPods++
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				result.Summary.InCrashLoop++
				break
			}
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.InCrashLoop > 0 {
		score = 100 - (result.Summary.InCrashLoop*100)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type DiskPressureResult2286 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int `json:"totalNodes"`
		WithPressure int `json:"withDiskPressure"`
	} `json:"summary"`
}

func (s *Server) handleDiskPressure2286(w http.ResponseWriter, r *http.Request) {
	result := DiskPressureResult2286{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeDiskPressure && cond.Status == corev1.ConditionTrue {
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

type RestartDistResult2286 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByRestartBucket map[string]int `json:"byRestartBucket"`
	} `json:"summary"`
}

func (s *Server) handleRestartDist2286(w http.ResponseWriter, r *http.Request) {
	result := RestartDistResult2286{ScannedAt: time.Now()}
	result.Summary.ByRestartBucket = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			rc := int(cs.RestartCount)
			var bucket string
			switch {
			case rc == 0:
				bucket = "0"
			case rc <= 3:
				bucket = "1-3"
			case rc <= 10:
				bucket = "4-10"
			default:
				bucket = "10+"
			}
			result.Summary.ByRestartBucket[bucket]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
