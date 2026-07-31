package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.89 Security: Pod Privileged Escalation, Secret Key Count, ClusterRole Rule Count
type PrivEscResult2389 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithPrivEsc     int `json:"withAllowPrivilegeEscalation"`
	} `json:"summary"`
}

func (s *Server) handlePrivEsc2389(w http.ResponseWriter, r *http.Request) {
	result := PrivEscResult2389{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil && *c.SecurityContext.AllowPrivilegeEscalation {
				result.Summary.WithPrivEsc++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 && result.Summary.WithPrivEsc > 0 {
		score = 100 - (result.Summary.WithPrivEsc*50)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type SecretKeyCountResult2389 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalKeys    int `json:"totalDataKeys"`
	} `json:"summary"`
}

func (s *Server) handleSecretKeyCount2389(w http.ResponseWriter, r *http.Request) {
	result := SecretKeyCountResult2389{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.TotalKeys += len(secret.Data)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CRRuleCountResult2389 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCR    int `json:"totalClusterRoles"`
		TotalRules int `json:"totalPolicyRules"`
	} `json:"summary"`
}

func (s *Server) handleCRRuleCount2389(w http.ResponseWriter, r *http.Request) {
	result := CRRuleCountResult2389{ScannedAt: time.Now()}
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})
	for _, cr := range crList.Items {
		result.Summary.TotalCR++
		result.Summary.TotalRules += len(cr.Rules)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
