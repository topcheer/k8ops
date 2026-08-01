package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.22 Product: Pod ServiceAccount Missing, Container StartupProbe, Service ExternalName
type SAMissingResult2422 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		MissingSA int `json:"missingServiceAccount"`
	} `json:"summary"`
}

func (s *Server) handleSAMissing2422(w http.ResponseWriter, r *http.Request) {
	result := SAMissingResult2422{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ServiceAccountName == "" {
			result.Summary.MissingSA++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = 100 - (result.Summary.MissingSA*50)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type StartupProbeResult2422 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStartup     int `json:"withStartupProbe"`
	} `json:"summary"`
}

func (s *Server) handleStartupProbe2422(w http.ResponseWriter, r *http.Request) {
	result := StartupProbeResult2422{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.StartupProbe != nil {
				result.Summary.WithStartup++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExternalNameResult2422 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int `json:"totalServices"`
		ExternalName  int `json:"externalNameServices"`
	} `json:"summary"`
}

func (s *Server) handleExternalName2422(w http.ResponseWriter, r *http.Request) {
	result := ExternalNameResult2422{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Summary.ExternalName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
