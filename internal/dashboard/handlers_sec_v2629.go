package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.29 Security: Pod Seccomp Type Dist, Secret Data Size, ClusterRole NonResourceURLs
type SeccompType2629Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByType          map[string]int `json:"bySeccompType"`
	} `json:"summary"`
}

func (s *Server) handleSeccompType2629(w http.ResponseWriter, r *http.Request) {
	result := SeccompType2629Result{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			t := "<none>"
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				t = string(c.SecurityContext.SeccompProfile.Type)
			}
			result.Summary.ByType[t]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretDataSize2629Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets   int `json:"totalSecrets"`
		TotalDataBytes int `json:"totalDataBytes"`
	} `json:"summary"`
}

func (s *Server) handleSecretDataSize2629(w http.ResponseWriter, r *http.Request) {
	result := SecretDataSize2629Result{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		for _, v := range secret.Data {
			result.Summary.TotalDataBytes += len(v)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRNonResourceURLs2629Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs int `json:"totalClusterRoles"`
		WithURLs int `json:"withNonResourceURLs"`
	} `json:"summary"`
}

func (s *Server) handleCRNonResourceURLs2629(w http.ResponseWriter, r *http.Request) {
	result := CRNonResourceURLs2629Result{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		for _, rule := range cr.Rules {
			if len(rule.NonResourceURLs) > 0 {
				result.Summary.WithURLs++
				break
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
