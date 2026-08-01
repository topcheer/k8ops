package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.42 Product: Pod Spec SchedulerName Dist, Container Resource Memory Limit, Service Selector Labels
type SchedulerNameDistResult2542 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		BySched   map[string]int `json:"bySchedulerName"`
	} `json:"summary"`
}

func (s *Server) handleSchedulerNameDist2542(w http.ResponseWriter, r *http.Request) {
	result := SchedulerNameDistResult2542{ScannedAt: time.Now()}
	result.Summary.BySched = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sn := pod.Spec.SchedulerName
		if sn == "" {
			sn = "<default>"
		}
		result.Summary.BySched[sn]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MemLimitSummaryResult2542 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalMemLimitMB float64 `json:"totalMemLimitMB"`
	} `json:"summary"`
}

func (s *Server) handleMemLimitSummary2542(w http.ResponseWriter, r *http.Request) {
	result := MemLimitSummaryResult2542{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMemLimitMB += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ServiceSelectorResult2542 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs    int `json:"totalServices"`
		WithSelector int `json:"withSelectorLabels"`
	} `json:"summary"`
}

func (s *Server) handleServiceSelector2542(w http.ResponseWriter, r *http.Request) {
	result := ServiceSelectorResult2542{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if len(svc.Spec.Selector) > 0 {
			result.Summary.WithSelector++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
