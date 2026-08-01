package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.34 Product: Pod EphemeralContainers, Container EnvFrom Count, Service SessionAffinityTimeout
type EphemeralCtnrResult2434 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithEphemeral int `json:"withEphemeralContainers"`
	} `json:"summary"`
}

func (s *Server) handleEphemeralCtnr2434(w http.ResponseWriter, r *http.Request) {
	result := EphemeralCtnrResult2434{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.EphemeralContainers) > 0 {
			result.Summary.WithEphemeral++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EnvFromCountResult2434 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalEnvFrom    int `json:"totalEnvFrom"`
	} `json:"summary"`
}

func (s *Server) handleEnvFromCount2434(w http.ResponseWriter, r *http.Request) {
	result := EnvFromCountResult2434{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalEnvFrom += len(c.EnvFrom)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SessionTimeoutResult2434 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		WithTimeout   int `json:"withSessionAffinityTimeout"`
	} `json:"summary"`
}

func (s *Server) handleSessionTimeout2434(w http.ResponseWriter, r *http.Request) {
	result := SessionTimeoutResult2434{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.SessionAffinityConfig != nil && svc.Spec.SessionAffinityConfig.ClientIP != nil && svc.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds != nil {
			result.Summary.WithTimeout++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
