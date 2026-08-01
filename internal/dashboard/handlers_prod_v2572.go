package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.72 Product: Pod Spec GMSA, Container Resource Request vs Limit Memory, Service LoadBalancer Class
type PodGMSAResult2572 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithGMSA  int `json:"withGMSA"`
	}
}

func (s *Server) handlePodGMSA2572(w http.ResponseWriter, r *http.Request) {
	result := PodGMSAResult2572{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.WindowsOptions != nil && pod.Spec.SecurityContext.WindowsOptions.GMSACredentialSpecName != nil && *pod.Spec.SecurityContext.WindowsOptions.GMSACredentialSpecName != "" {
			result.Summary.WithGMSA++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type MemReqVsLimResult2572 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalMemReqMB   float64 `json:"totalMemReqMB"`
		TotalMemLimMB   float64 `json:"totalMemLimMB"`
	}
}

func (s *Server) handleMemReqVsLim2572(w http.ResponseWriter, r *http.Request) {
	result := MemReqVsLimResult2572{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMemReqMB += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
			result.Summary.TotalMemLimMB += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LBClassResult2572 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalLB int            `json:"totalLoadBalancer"`
		ByClass map[string]int `json:"byLoadBalancerClass"`
	}
}

func (s *Server) handleLBClass2572(w http.ResponseWriter, r *http.Request) {
	result := LBClassResult2572{ScannedAt: time.Now()}
	result.Summary.ByClass = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		cls := "<default>"
		if svc.Spec.LoadBalancerClass != nil {
			cls = *svc.Spec.LoadBalancerClass
		}
		result.Summary.ByClass[cls]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
