package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.04 Product: Pod Overhead Resource, Container SecurityContext RunAsUser, Service ExternalTrafficPolicy
type OverheadResult2404 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithOverhead int `json:"withOverhead"`
	} `json:"summary"`
}

func (s *Server) handleOverhead2404(w http.ResponseWriter, r *http.Request) {
	result := OverheadResult2404{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Overhead != nil {
			result.Summary.WithOverhead++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type RunAsUserResult2404 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithRunAsUser   int `json:"withRunAsUser"`
	} `json:"summary"`
}

func (s *Server) handleRunAsUser2404(w http.ResponseWriter, r *http.Request) {
	result := RunAsUserResult2404{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				result.Summary.WithRunAsUser++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExtTrafficPolResult2404 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByPolicy      map[string]int `json:"byExternalTrafficPolicy"`
	} `json:"summary"`
}

func (s *Server) handleExtTrafficPol2404(w http.ResponseWriter, r *http.Request) {
	result := ExtTrafficPolResult2404{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.ByPolicy[string(svc.Spec.ExternalTrafficPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
