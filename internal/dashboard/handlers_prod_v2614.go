package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.14 Product: Pod Spec OS Name Detail, Container Resource Summary v2, Service SessionAffinityConfig
type PodOSName2614Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		WithOS    int `json:"withOSSpecified"`
	} `json:"summary"`
}

func (s *Server) handlePodOSName2614(w http.ResponseWriter, r *http.Request) {
	result := PodOSName2614Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil {
			result.Summary.WithOS++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ResourceSummary2Result2614 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPUReq"`
		TotalMemLim     float64 `json:"totalMemLimMB"`
	} `json:"summary"`
}

func (s *Server) handleResourceSummary2Result2614(w http.ResponseWriter, r *http.Request) {
	result := ResourceSummary2Result2614{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalMemLim += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SvcSessionConfigResult2614 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int `json:"totalServices"`
		WithConfig int `json:"withSessionAffinityConfig"`
	} `json:"summary"`
}

func (s *Server) handleSvcSessionConfig2614(w http.ResponseWriter, r *http.Request) {
	result := SvcSessionConfigResult2614{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if svc.Spec.SessionAffinityConfig.ClientIP != nil {
			result.Summary.WithConfig++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
