package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.96 Product: Pod Spec Subdomain, Container Resource Summary Detail, Service ExternalTrafficPolicy
type SubdomainResult2596 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		BySubdomain map[string]int `json:"bySubdomain"`
	}
}

func (s *Server) handleSubdomain2596(w http.ResponseWriter, r *http.Request) {
	result := SubdomainResult2596{ScannedAt: time.Now()}
	result.Summary.BySubdomain = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sd := pod.Spec.Subdomain
		if sd == "" {
			sd = "<none>"
		}
		result.Summary.BySubdomain[sd]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResourceSummaryDetailResult2596 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPUReq"`
		TotalMemReqMB   float64 `json:"totalMemReqMB"`
	}
}

func (s *Server) handleResourceSummaryDetail2596(w http.ResponseWriter, r *http.Request) {
	result := ResourceSummaryDetailResult2596{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalMemReqMB += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExternalTrafficPolicyResult2596 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByPolicy  map[string]int `json:"byExternalTrafficPolicy"`
	}
}

func (s *Server) handleExternalTrafficPolicy2596(w http.ResponseWriter, r *http.Request) {
	result := ExternalTrafficPolicyResult2596{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type == corev1.ServiceTypeNodePort || svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			result.Summary.TotalSvcs++
			result.Summary.ByPolicy[string(svc.Spec.ExternalTrafficPolicy)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
