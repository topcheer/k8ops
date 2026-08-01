package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.09 Security: Pod CapAdd Summary, Secret Type Distribution, ClusterRole Resource Summary
type CapAddSummaryResult2509 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByCap           map[string]int `json:"byCapabilityAdded"`
	} `json:"summary"`
}

func (s *Server) handleCapAddSummary2509(w http.ResponseWriter, r *http.Request) {
	result := CapAddSummaryResult2509{ScannedAt: time.Now()}
	result.Summary.ByCap = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, cap := range c.SecurityContext.Capabilities.Add {
					result.Summary.ByCap[string(cap)]++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretTypeFullResult2509 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByType       map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handleSecretTypeFull2509(w http.ResponseWriter, r *http.Request) {
	result := SecretTypeFullResult2509{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByType[string(secret.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRResourceResult2509 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs   int            `json:"totalClusterRoles"`
		ByResource map[string]int `json:"byResource"`
	} `json:"summary"`
}

func (s *Server) handleCRResource2509(w http.ResponseWriter, r *http.Request) {
	result := CRResourceResult2509{ScannedAt: time.Now()}
	result.Summary.ByResource = make(map[string]int)
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		for _, rule := range cr.Rules {
			for _, res := range rule.Resources {
				result.Summary.ByResource[res]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
