package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.10 Product: Pod PreemptionPolicy, Container WorkingDir, Service InternalTrafficPolicy
type PreemptionResult2410 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byPreemptionPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePreemption2410(w http.ResponseWriter, r *http.Request) {
	result := PreemptionResult2410{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		pp := "<none>"
		if pod.Spec.PreemptionPolicy != nil {
			pp = string(*pod.Spec.PreemptionPolicy)
		}
		result.Summary.ByPolicy[pp]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type WorkingDirResult2410 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithWorkingDir  int `json:"withWorkingDir"`
	} `json:"summary"`
}

func (s *Server) handleWorkingDir2410(w http.ResponseWriter, r *http.Request) {
	result := WorkingDirResult2410{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.WorkingDir != "" {
				result.Summary.WithWorkingDir++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IntTrafficPolResult2410 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByPolicy      map[string]int `json:"byInternalTrafficPolicy"`
	} `json:"summary"`
}

func (s *Server) handleIntTrafficPol2410(w http.ResponseWriter, r *http.Request) {
	result := IntTrafficPolResult2410{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		pol := "Cluster"
		if svc.Spec.InternalTrafficPolicy != nil {
			pol = string(*svc.Spec.InternalTrafficPolicy)
		}
		result.Summary.ByPolicy[pol]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
