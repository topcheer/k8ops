package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.12 — Product Dimension (Round 55)
// 1. Pod Volume Mount SubPath Audit
// 2. Container Resources Requests Memory Distribution
// 3. Service Internal Traffic Policy Local Audit
// ============================================================

type SubPathResult2212 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalMounts int `json:"totalVolumeMounts"`
		WithSubPath int `json:"withSubPath"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSubPath2212(w http.ResponseWriter, r *http.Request) {
	result := SubPathResult2212{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			for _, vm := range c.VolumeMounts {
				result.Summary.TotalMounts++
				if vm.SubPath != "" {
					result.Summary.WithSubPath++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Memory Request Distribution
type MemReqDistResult2212 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalReqMB      float64 `json:"totalRequestedMB"`
		AvgReqMB        float64 `json:"avgReqMB"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleMemReqDist2212(w http.ResponseWriter, r *http.Request) {
	result := MemReqDistResult2212{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalReqMB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e6
		}
	}
	if result.Summary.TotalContainers > 0 {
		result.Summary.AvgReqMB = result.Summary.TotalReqMB / float64(result.Summary.TotalContainers)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Internal Traffic Policy Local
type IntTrafficLocalResult2212 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		LocalPolicy   int `json:"localPolicy"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleIntTrafficLocal2212(w http.ResponseWriter, r *http.Request) {
	result := IntTrafficLocalResult2212{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.InternalTrafficPolicy != nil && *svc.Spec.InternalTrafficPolicy == corev1.ServiceInternalTrafficPolicyLocal {
			result.Summary.LocalPolicy++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
