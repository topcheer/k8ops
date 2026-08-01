package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.79 Security: Pod ReadOnlyRootFilesystem Ratio, Secret ServiceAccountToken Count, ClusterRole Rules Total
type RORootFSResult2479 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		ReadOnlyRootFS  int `json:"readOnlyRootFilesystem"`
	} `json:"summary"`
}

func (s *Server) handleRORootFS2479(w http.ResponseWriter, r *http.Request) {
	result := RORootFSResult2479{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.ReadOnlyRootFS++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.ReadOnlyRootFS * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretSATokenResult2479 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		SAToken      int `json:"serviceAccountTokenCount"`
	} `json:"summary"`
}

func (s *Server) handleSecretSAToken2479(w http.ResponseWriter, r *http.Request) {
	result := SecretSATokenResult2479{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeServiceAccountToken {
			result.Summary.SAToken++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRRulesTotalResult2479 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCRs   int `json:"totalClusterRoles"`
		TotalRules int `json:"totalRules"`
	} `json:"summary"`
}

func (s *Server) handleCRRulesTotal2479(w http.ResponseWriter, r *http.Request) {
	result := CRRulesTotalResult2479{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCRs++
		result.Summary.TotalRules += len(cr.Rules)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
