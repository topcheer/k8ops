package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.23 Security: Pod CapAdd Count, Secret OwnerRef Count, ClusterRole Wildcard Verbs
type CapAddCount2623Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithCapAdd      int `json:"withCapAdd"`
		TotalCapAdd     int `json:"totalCapAdd"`
	} `json:"summary"`
}

func (s *Server) handleCapAddCount2623(w http.ResponseWriter, r *http.Request) {
	result := CapAddCount2623Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				if len(c.SecurityContext.Capabilities.Add) > 0 {
					result.Summary.WithCapAdd++
					result.Summary.TotalCapAdd += len(c.SecurityContext.Capabilities.Add)
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretOwnerRefCount2623Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		WithOwnerRef int `json:"withOwnerReferences"`
	} `json:"summary"`
}

func (s *Server) handleSecretOwnerRefCount2623(w http.ResponseWriter, r *http.Request) {
	result := SecretOwnerRefCount2623Result{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if len(secret.OwnerReferences) > 0 {
			result.Summary.WithOwnerRef++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRWildcardVerbs2623Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs     int `json:"totalClusterRoles"`
		WithWildcard int `json:"withWildcardVerbs"`
	} `json:"summary"`
}

func (s *Server) handleCRWildcardVerbs2623(w http.ResponseWriter, r *http.Request) {
	result := CRWildcardVerbs2623Result{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					result.Summary.WithWildcard++
					break
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
