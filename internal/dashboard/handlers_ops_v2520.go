package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.20 Operations: Node StatusConditions, Pod Failed Count, Container Exit Code Summary
type NodeConditionsResult2520 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByCond     map[string]int `json:"byConditionType"`
	} `json:"summary"`
}

func (s *Server) handleNodeConditions2520(w http.ResponseWriter, r *http.Request) {
	result := NodeConditionsResult2520{ScannedAt: time.Now()}
	result.Summary.ByCond = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				result.Summary.ByCond[string(cond.Type)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodFailedCountResult2520 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Failed    int `json:"failedPods"`
	} `json:"summary"`
}

func (s *Server) handlePodFailedCount2520(w http.ResponseWriter, r *http.Request) {
	result := PodFailedCountResult2520{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodFailed {
			result.Summary.Failed++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.Failed > 0 {
		score = 100 - (result.Summary.Failed*100)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type ExitCodeResult2520 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByExitCode      map[string]int `json:"byExitCode"`
	} `json:"summary"`
}

func (s *Server) handleExitCode2520(w http.ResponseWriter, r *http.Request) {
	result := ExitCodeResult2520{ScannedAt: time.Now()}
	result.Summary.ByExitCode = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalContainers++
				code := cs.LastTerminationState.Terminated.ExitCode
				result.Summary.ByExitCode[intToStr(int(code))]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
