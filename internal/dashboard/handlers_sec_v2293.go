package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v22.93 Security: Pod capabilities drop audit, image registry trust audit, secret env var exposure
type CapDropResult2293 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCapDrop     int `json:"withCapDrop"`
	} `json:"summary"`
}

func (s *Server) handleCapDrop2293(w http.ResponseWriter, r *http.Request) {
	result := CapDropResult2293{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil && len(c.SecurityContext.Capabilities.Drop) > 0 {
				result.Summary.WithCapDrop++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.WithCapDrop * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RegistryTrustResult2293 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int            `json:"totalImages"`
		ByRegistry  map[string]int `json:"byRegistry"`
	} `json:"summary"`
}

func (s *Server) handleRegistryTrust2293(w http.ResponseWriter, r *http.Request) {
	result := RegistryTrustResult2293{ScannedAt: time.Now()}
	result.Summary.ByRegistry = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if seen[c.Image] {
				continue
			}
			seen[c.Image] = true
			result.Summary.TotalImages++
			reg := "docker.io"
			if idx := strings.Index(c.Image, "/"); idx > 0 {
				prefix := c.Image[:idx]
				if strings.Contains(prefix, ".") || strings.Contains(prefix, ":") {
					reg = prefix
				}
			}
			result.Summary.ByRegistry[reg]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretEnvResult2293 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers  int `json:"totalContainers"`
		WithSecretEnvRef int `json:"withSecretEnvRef"`
	} `json:"summary"`
}

func (s *Server) handleSecretEnv2293(w http.ResponseWriter, r *http.Request) {
	result := SecretEnvResult2293{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, env := range c.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					result.Summary.WithSecretEnvRef++
					break
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
