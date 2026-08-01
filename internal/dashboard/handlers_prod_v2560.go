package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.60 Product: Pod Spec TerminationGracePeriod Distribution, Container Resource Limit Memory, Service ClusterIP None Count
type TermGraceSummaryResult2560 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByGrace   map[string]int `json:"byGracePeriod"`
	}
}

func (s *Server) handleTermGraceSummary2560(w http.ResponseWriter, r *http.Request) {
	result := TermGraceSummaryResult2560{ScannedAt: time.Now()}
	result.Summary.ByGrace = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		g := "30"
		if pod.Spec.TerminationGracePeriodSeconds != nil {
			g = intToStr(int(*pod.Spec.TerminationGracePeriodSeconds))
		}
		result.Summary.ByGrace[g]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MemLimitContainerResult2560 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalMemLimit   float64 `json:"totalMemLimitMB"`
	}
}

func (s *Server) handleMemLimitContainer2560(w http.ResponseWriter, r *http.Request) {
	result := MemLimitContainerResult2560{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMemLimit += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ClusterIPNoneResult2560 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int `json:"totalServices"`
		Headless  int `json:"headlessServices"`
	}
}

func (s *Server) handleClusterIPNone2560(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPNoneResult2560{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.ClusterIP == "None" {
			result.Summary.Headless++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
