package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.24 Product: Pod Spec EnableServiceLinks, Container Resource Limit CPU Summary, Service ClusterIPs Count
type EnableServiceLinksResult2524 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Disabled  int `json:"serviceLinksDisabled"`
	} `json:"summary"`
}

func (s *Server) handleEnableServiceLinks2524(w http.ResponseWriter, r *http.Request) {
	result := EnableServiceLinksResult2524{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.EnableServiceLinks != nil && !*pod.Spec.EnableServiceLinks {
			result.Summary.Disabled++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPULimitSummaryResult2524 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPULimit   float64 `json:"totalCPULimitCores"`
	} `json:"summary"`
}

func (s *Server) handleCPULimitSummary2524(w http.ResponseWriter, r *http.Request) {
	result := CPULimitSummaryResult2524{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPULimit += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ClusterIPsCountResult2524 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int `json:"totalServices"`
		TotalCIPs int `json:"totalClusterIPs"`
	} `json:"summary"`
}

func (s *Server) handleClusterIPsCount2524(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPsCountResult2524{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		result.Summary.TotalCIPs += len(svc.Spec.ClusterIPs)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
