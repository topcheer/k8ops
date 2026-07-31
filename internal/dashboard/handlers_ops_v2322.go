package dashboard

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.22 Operations: Pod Pending Duration Risk, Node CPU Throttling Risk, Container Exit Code Distribution
type PendingDurationResult2322 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		PendingPods int `json:"pendingPods"`
		LongPending int `json:"longPending"`
	} `json:"summary"`
}

func (s *Server) handlePendingDuration2322(w http.ResponseWriter, r *http.Request) {
	result := PendingDurationResult2322{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodPending {
			result.Summary.PendingPods++
			if pod.CreationTimestamp.Time.Before(now.Add(-5 * time.Minute)) {
				result.Summary.LongPending++
			}
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.PendingPods > 0 {
		score = 100 - (result.Summary.PendingPods*50)/result.Summary.TotalPods
		if result.Summary.LongPending > 0 {
			score -= 20
		}
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type CPUThrottleResult2322 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCPULimit    int `json:"withCPULimit"`
		WithoutLimit    int `json:"withoutCPULimit"`
	} `json:"summary"`
}

func (s *Server) handleCPUThrottle2322(w http.ResponseWriter, r *http.Request) {
	result := CPUThrottleResult2322{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if !c.Resources.Limits.Cpu().IsZero() {
				result.Summary.WithCPULimit++
			} else {
				result.Summary.WithoutLimit++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExitCodeResult2322 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalTerminated int            `json:"totalTerminated"`
		ByExitCode      map[string]int `json:"byExitCode"`
	} `json:"summary"`
}

func (s *Server) handleExitCode2322(w http.ResponseWriter, r *http.Request) {
	result := ExitCodeResult2322{ScannedAt: time.Now()}
	result.Summary.ByExitCode = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalTerminated++
				code := fmt.Sprintf("%d", cs.LastTerminationState.Terminated.ExitCode)
				result.Summary.ByExitCode[code]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
