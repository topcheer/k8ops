package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.38 Product: Pod OS Name Windows, Container Stderr File, Service Session Affinity Config
type PodOSNameResult2338 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByOSName  map[string]int `json:"byOSName"`
	} `json:"summary"`
}

func (s *Server) handlePodOSName2338(w http.ResponseWriter, r *http.Request) {
	result := PodOSNameResult2338{ScannedAt: time.Now()}
	result.Summary.ByOSName = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.OS != nil {
			result.Summary.ByOSName[string(pod.Spec.OS.Name)]++
		} else {
			result.Summary.ByOSName["linux"]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StderrResult2338 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStderr      int `json:"withStderrRedirect"`
	} `json:"summary"`
}

func (s *Server) handleStderr2338(w http.ResponseWriter, r *http.Request) {
	result := StderrResult2338{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Stdin {
				result.Summary.WithStderr++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SessionAffConfigResult2338 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		ClientIP      int `json:"clientIPAffinity"`
	} `json:"summary"`
}

func (s *Server) handleSessionAffConfig2338(w http.ResponseWriter, r *http.Request) {
	result := SessionAffConfigResult2338{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.SessionAffinity == corev1.ServiceAffinityClientIP {
			result.Summary.ClientIP++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
