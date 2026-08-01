package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.26 Product: Pod Spec SetHostnameAsFQDN, Container Resource Summary v3, Service InternalTrafficPolicy
type FQDN2626Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithFQDN  int `json:"withSetHostnameAsFQDN"`
	} `json:"summary"`
}

func (s *Server) handleFQDN2626(w http.ResponseWriter, r *http.Request) {
	result := FQDN2626Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SetHostnameAsFQDN != nil && *pod.Spec.SetHostnameAsFQDN {
			result.Summary.WithFQDN++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResourceSummary3Result2626 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPUReq"`
		TotalCPULim     float64 `json:"totalCPULim"`
		TotalMemReqMB   float64 `json:"totalMemReqMB"`
		TotalMemLimMB   float64 `json:"totalMemLimMB"`
	} `json:"summary"`
}

func (s *Server) handleResourceSummary3Result2626(w http.ResponseWriter, r *http.Request) {
	result := ResourceSummary3Result2626{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalCPULim += c.Resources.Limits.Cpu().AsApproximateFloat64()
			result.Summary.TotalMemReqMB += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
			result.Summary.TotalMemLimMB += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcInternalTraffic2626Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs int            `json:"totalServices"`
		ByPolicy  map[string]int `json:"byInternalTrafficPolicy"`
	} `json:"summary"`
}

func (s *Server) handleSvcInternalTraffic2626(w http.ResponseWriter, r *http.Request) {
	result := SvcInternalTraffic2626Result{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.InternalTrafficPolicy != nil {
			result.Summary.ByPolicy[string(*svc.Spec.InternalTrafficPolicy)]++
		} else {
			result.Summary.ByPolicy["<default>"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
