package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.08 Product: Pod Spec EphemeralContainers, Container Resource CPU Limit Summary, Service IPFamily Detail
type EphemeralContainersResult2608 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithEphem int `json:"withEphemeralContainers"`
	} `json:"summary"`
}

func (s *Server) handleEphemeralContainers2608(w http.ResponseWriter, r *http.Request) {
	result := EphemeralContainersResult2608{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.EphemeralContainers) > 0 {
			result.Summary.WithEphem++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPULimitSummary2608Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPULim     float64 `json:"totalCPULimit"`
	} `json:"summary"`
}

func (s *Server) handleCPULimitSummary2608(w http.ResponseWriter, r *http.Request) {
	result := CPULimitSummary2608Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPULim += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcIPFamily2608Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByFamily  map[string]int `json:"byIPFamily"`
	} `json:"summary"`
}

func (s *Server) handleSvcIPFamily2608(w http.ResponseWriter, r *http.Request) {
	result := SvcIPFamily2608Result{ScannedAt: time.Now()}
	result.Summary.ByFamily = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		for _, f := range svc.Spec.IPFamilies {
			result.Summary.ByFamily[string(f)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
